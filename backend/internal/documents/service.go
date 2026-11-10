package documents

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"endosoflow/internal/adapters/storage"
	"endosoflow/internal/audit"
	"endosoflow/internal/auth"
	"endosoflow/internal/platform/apperr"
	"endosoflow/internal/platform/database"
	"endosoflow/internal/platform/ids"
	"endosoflow/internal/storagecleanup"
	"endosoflow/internal/tasks"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// DefaultDownloadTTL is how long a signed download address works.
const DefaultDownloadTTL = 5 * time.Minute

var (
	errSaleNotFound     = apperr.NotFound("SALE_NOT_FOUND", "La venta no existe.")
	errTaskNotFound     = apperr.NotFound("TASK_NOT_FOUND", "La tarea no existe.")
	errSlotNotFound     = apperr.NotFound("DOCUMENT_SLOT_NOT_FOUND", "Ese documento no existe en esta tarea.")
	errDocumentNotFound = apperr.NotFound("DOCUMENT_NOT_FOUND", "El documento no existe.")
	errForbidden        = apperr.Forbidden("FORBIDDEN", "No tiene permiso para realizar esta acción.")
	errSaleNotActive    = apperr.Conflict("SALE_NOT_ACTIVE", "La venta no está activa: sus documentos no se pueden modificar.")
	errNotInProgress    = apperr.Conflict("TASK_NOT_IN_PROGRESS", "La tarea no está en curso: inicie la tarea para subir o quitar documentos.")
	errTooLarge         = apperr.New(http.StatusRequestEntityTooLarge, "DOCUMENT_TOO_LARGE", "El archivo supera el máximo de 10 MB.")
	errStorage          = apperr.New(http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "El almacenamiento de archivos no está disponible. Intente nuevamente.")
	errFileRequired     = apperr.Unprocessable("DOCUMENT_REQUIRED", "Falta el archivo.").
				WithFields(apperr.FieldError{Field: "file", Code: "REQUIRED", Message: "El archivo es obligatorio."})
)

// Deps are the service's dependencies.
type Deps struct {
	DB          auth.Database
	Store       storage.Store
	Sales       tasks.SaleGate
	Logger      *slog.Logger
	Now         func() time.Time // nil means time.Now
	DownloadTTL time.Duration    // zero means DefaultDownloadTTL
}

// Service implements the document use cases. It also answers the task engine's question about missing files.
type Service struct {
	db     auth.Database
	store  storage.Store
	sales  tasks.SaleGate
	logger *slog.Logger
	now    func() time.Time
	ttl    time.Duration
}

var _ tasks.DocumentSource = (*Service)(nil)

// NewService builds the service.
func NewService(d Deps) *Service {
	s := &Service{db: d.DB, store: d.Store, sales: d.Sales, logger: d.Logger, now: d.Now, ttl: d.DownloadTTL}
	if s.now == nil {
		s.now = time.Now
	}
	if s.ttl <= 0 {
		s.ttl = DefaultDownloadTTL
	}
	return s
}

func isStaff(actor auth.Actor) bool {
	switch actor.Role {
	case auth.RoleSystemAdmin, auth.RoleSalesManager, auth.RoleSalesAdvisor:
		return true
	}
	return false
}

func isManagement(actor auth.Actor) bool {
	return actor.Role == auth.RoleSystemAdmin || actor.Role == auth.RoleSalesManager
}

// scoped finds the sale and applies the actor's scope: an Advisor only reaches their own sales, and any other
// sale answers as if it did not exist.
func (s *Service) scoped(ctx context.Context, q querier, actor auth.Actor, saleID uuid.UUID, forUpdate bool) (*tasks.SaleRef, error) {
	if !isStaff(actor) {
		return nil, errForbidden
	}
	sale, err := s.sales.Find(ctx, q, saleID, forUpdate)
	if err != nil {
		return nil, err
	}
	if sale == nil || (actor.Role == auth.RoleSalesAdvisor && sale.AdvisorID != actor.ID) {
		return nil, errSaleNotFound
	}
	return sale, nil
}

// writable applies the rules for changing a task's files. The sale must be active. While the task is in
// progress its actors (own Advisor, Manager, Admin) may change files. A completed task can only have a file
// replaced, by a Manager or Admin, with a reason (allowCompleted). Anything else is refused.
func (s *Service) writable(ctx context.Context, q querier, actor auth.Actor, saleID uuid.UUID, number int,
	forUpdate, allowCompleted bool, reason string,
) (*tasks.SaleRef, *tasks.TaskInfo, error) {
	sale, err := s.scoped(ctx, q, actor, saleID, forUpdate)
	if err != nil {
		return nil, nil, err
	}
	if sale.State == "cancelled" {
		return nil, nil, errSaleNotActive
	}
	task, err := tasks.FindTask(ctx, q, saleID, number, forUpdate)
	if err != nil {
		return nil, nil, err
	}
	if task == nil {
		return nil, nil, errTaskNotFound
	}
	switch {
	case task.Status == "in_progress":
		if sale.State != "active" {
			return nil, nil, errSaleNotActive
		}
	case task.Status == "completed" && allowCompleted:
		if !isManagement(actor) {
			return nil, nil, errForbidden
		}
		if reason == "" {
			return nil, nil, apperr.Unprocessable("REASON_REQUIRED", "Debe escribir el motivo del reemplazo.").
				WithFields(apperr.FieldError{Field: "reason", Code: "REQUIRED", Message: "El motivo es obligatorio en una tarea completada."})
		}
	default:
		return nil, nil, errNotInProgress
	}
	return sale, task, nil
}

// slotFor returns the slot, which must belong to the task in the path.
func (s *Service) slotFor(ctx context.Context, code string, number int) (*slotRow, error) {
	if number < 1 || number > tasks.TaskCount {
		return nil, errTaskNotFound
	}
	slot, err := findSlot(ctx, s.db, code)
	if err != nil {
		return nil, err
	}
	if slot == nil || slot.TaskNumber != number {
		return nil, errSlotNotFound
	}
	return slot, nil
}

// List returns every slot of a sale with its current file.
func (s *Service) List(ctx context.Context, actor auth.Actor, saleID uuid.UUID) (*SlotList, error) {
	sale, err := s.scoped(ctx, s.db, actor, saleID, false)
	if err != nil {
		return nil, err
	}
	items, err := listSlots(ctx, s.db, saleID, sale.CustomerType)
	if err != nil {
		return nil, err
	}
	return &SlotList{Items: items}, nil
}

// Upload stores a file in a slot, replacing the current one. The file is checked by content (not by name),
// written to private storage, and then recorded in one transaction; a replaced file is deleted from storage
// only after that transaction commits. The bool is true when a file was replaced.
func (s *Service) Upload(ctx context.Context, actor auth.Actor, in UploadInput) (*File, bool, error) {
	slot, err := s.slotFor(ctx, in.Slot, in.Number)
	if err != nil {
		return nil, false, err
	}
	switch {
	case len(in.Data) == 0:
		return nil, false, errFileRequired
	case len(in.Data) > MaxBytes:
		return nil, false, errTooLarge
	}
	kind, ok := sniff(in.Data)
	if !ok || kind.Format != slot.Format {
		message := "El archivo debe ser un PDF."
		if slot.Format == FormatExcel {
			message = "El archivo debe ser una hoja de Excel (.xlsx o .xls)."
		}
		return nil, false, apperr.Unprocessable("DOCUMENT_TYPE_NOT_ALLOWED", message).
			WithFields(apperr.FieldError{Field: "file", Code: "TYPE_NOT_ALLOWED", Message: message})
	}
	reason := strings.TrimSpace(in.Reason)
	if utf8.RuneCountInString(reason) > 500 {
		return nil, false, apperr.Unprocessable("REASON_TOO_LONG", "El motivo es demasiado largo.").
			WithFields(apperr.FieldError{Field: "reason", Code: "TOO_LONG", Message: "El motivo no puede superar los 500 caracteres."})
	}
	// Refuse early, before a 10 MB file is written to storage. The transaction below checks again under lock.
	if _, _, err := s.writable(ctx, s.db, actor, in.SaleID, in.Number, false, true, reason); err != nil {
		return nil, false, apperr.FromError(err)
	}

	fileName := cleanFileName(in.FileName)
	sum := sha256.Sum256(in.Data)
	checksum := hex.EncodeToString(sum[:])
	id := ids.New()
	key := fmt.Sprintf("documents/%s/%s.%s", in.SaleID, id, kind.Extension)
	if err := s.store.Put(ctx, key, kind.Mime, in.Data); err != nil {
		s.logger.Error("could not store a document", "error", err)
		return nil, false, errStorage.WithCause(err)
	}

	now := s.now()
	var file *File
	var replaced bool
	var old []storagecleanup.Pending
	err = database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		_, task, err := s.writable(ctx, tx, actor, in.SaleID, in.Number, true, true, reason)
		if err != nil {
			return err
		}
		existing, err := findDocument(ctx, tx, task.ID, in.Slot, true)
		if err != nil {
			return err
		}
		documentID := id
		event := audit.Event{ActorID: &actor.ID, SaleID: &in.SaleID, EntityType: audit.EntityDocument, Reason: reason}
		after := map[string]any{"fileName": fileName, "sizeBytes": len(in.Data), "checksumSha256": checksum}
		if existing == nil {
			if err := insertDocument(ctx, tx, id, in.SaleID, task.ID, in.Slot, fileName, kind.Mime, len(in.Data), key, checksum, actor.ID, now); err != nil {
				return err
			}
			event.Action, event.EntityID = audit.ActionUploaded, id
			event.Changes = map[string]any{"slot": in.Slot, "after": after}
		} else {
			replaced, documentID = true, existing.ID
			if err := replaceDocument(ctx, tx, existing.ID, fileName, kind.Mime, len(in.Data), key, checksum, actor.ID, now); err != nil {
				return err
			}
			pending, err := storagecleanup.Enqueue(ctx, tx, existing.Key, storagecleanup.DocumentReplaced)
			if err != nil {
				return err
			}
			old = append(old, pending)
			event.Action, event.EntityID = audit.ActionReplaced, existing.ID
			event.Changes = map[string]any{
				"slot":   in.Slot,
				"before": map[string]any{"fileName": existing.FileName, "sizeBytes": existing.SizeBytes, "checksumSha256": existing.ChecksumSHA},
				"after":  after,
			}
		}
		if err := audit.Record(ctx, tx, event, now); err != nil {
			return err
		}
		file, err = readFile(ctx, tx, documentID)
		return err
	})
	if err != nil {
		// The new object has no row pointing at it: queue its deletion so it cannot linger.
		s.discard(ctx, key)
		return nil, false, apperr.FromError(err)
	}
	storagecleanup.Process(ctx, s.db, s.store, s.logger, old...)
	return file, replaced, nil
}

// discard queues the deletion of an object that was stored but never recorded, and tries it once.
func (s *Service) discard(ctx context.Context, key string) {
	pending, err := storagecleanup.Enqueue(ctx, s.db, key, storagecleanup.DocumentRemoved)
	if err != nil {
		s.logger.Error("could not queue the deletion of an unrecorded file", "key", key, "error", err)
		return
	}
	storagecleanup.Process(ctx, s.db, s.store, s.logger, pending)
}

// Remove deletes the file of a slot. It is allowed only while the task is in progress: a completed task can
// have its file replaced but never left without one.
func (s *Service) Remove(ctx context.Context, actor auth.Actor, saleID uuid.UUID, number int, slotCode string) error {
	if _, err := s.slotFor(ctx, slotCode, number); err != nil {
		return err
	}
	now := s.now()
	var pending []storagecleanup.Pending
	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		_, task, err := s.writable(ctx, tx, actor, saleID, number, true, false, "")
		if err != nil {
			return err
		}
		existing, err := findDocument(ctx, tx, task.ID, slotCode, true)
		if err != nil {
			return err
		}
		if existing == nil {
			return errDocumentNotFound
		}
		if err := deleteDocument(ctx, tx, existing.ID); err != nil {
			return err
		}
		item, err := storagecleanup.Enqueue(ctx, tx, existing.Key, storagecleanup.DocumentRemoved)
		if err != nil {
			return err
		}
		pending = append(pending, item)
		return audit.Record(ctx, tx, audit.Event{
			ActorID: &actor.ID, SaleID: &saleID, EntityType: audit.EntityDocument, EntityID: existing.ID, Action: audit.ActionRemoved,
			Changes: map[string]any{"slot": slotCode, "fileName": existing.FileName, "sizeBytes": existing.SizeBytes, "checksumSha256": existing.ChecksumSHA},
		}, now)
	})
	if err != nil {
		return apperr.FromError(err)
	}
	storagecleanup.Process(ctx, s.db, s.store, s.logger, pending...)
	return nil
}

// DownloadURL issues a short-lived signed address for a file, after checking the caller's role and scope. The
// storage address itself is never exposed.
func (s *Service) DownloadURL(ctx context.Context, actor auth.Actor, id uuid.UUID) (*DownloadURL, error) {
	if !isStaff(actor) {
		return nil, errForbidden
	}
	ref, err := findReference(ctx, s.db, id)
	if err != nil {
		return nil, err
	}
	if ref == nil {
		return nil, errDocumentNotFound
	}
	if _, err := s.scoped(ctx, s.db, actor, ref.SaleID, false); err != nil {
		if errors.Is(err, errSaleNotFound) {
			return nil, errDocumentNotFound // do not reveal that the document exists
		}
		return nil, err
	}
	signed, err := s.store.SignedURL(ctx, ref.Key, ref.FileName, s.ttl)
	if err != nil {
		s.logger.Error("could not sign a document download", "document", id, "error", err)
		return nil, errStorage.WithCause(err)
	}
	s.logger.Info("document download issued", "document", id, "sale", ref.SaleID, "actor", actor.ID)
	return &DownloadURL{URL: signed, ExpiresAt: s.now().Add(s.ttl), FileName: ref.FileName, MimeType: ref.Mime}, nil
}

// TaskSlots gives the task engine the slots of a task with their files. The engine uses it to show them and to
// refuse completing a task while a required slot is empty.
func (s *Service) TaskSlots(ctx context.Context, q tasks.Querier, taskInstanceID uuid.UUID, taskNumber int, customerType string) ([]tasks.DocumentSlot, error) {
	return taskSlots(ctx, q, taskInstanceID, taskNumber, customerType)
}

// StoredKeys lists the storage keys of every file of a sale. Deleting a sale queues them for removal from
// storage before the rows disappear.
func (s *Service) StoredKeys(ctx context.Context, q tasks.Querier, saleID uuid.UUID) ([]string, error) {
	return storedKeys(ctx, q, saleID)
}
