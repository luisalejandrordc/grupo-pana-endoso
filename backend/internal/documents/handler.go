package documents

import (
	"endosoflow/internal/auth"
	"endosoflow/internal/platform/apperr"
	"endosoflow/internal/platform/httpx"
	"endosoflow/internal/tasks"
	"errors"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Handler serves the document routes.
type Handler struct {
	svc    *Service
	auth   *auth.Handler
	logger *slog.Logger
}

// NewHandler builds the handler. The auth handler provides the Authenticate middleware.
func NewHandler(svc *Service, authHandler *auth.Handler, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, auth: authHandler, logger: logger}
}

// Mount registers the routes under the given router (already under /api/v1). The routes are written out in
// full because the sales and tasks modules share the /sales prefix.
func (h *Handler) Mount(r chi.Router) {
	staff := auth.RequireRoles(h.logger, auth.RoleSystemAdmin, auth.RoleSalesManager, auth.RoleSalesAdvisor)

	r.Group(func(r chi.Router) {
		r.Use(h.auth.Authenticate)
		r.With(staff).Get("/sales/{id}/documents", h.list)
		r.With(staff).Put("/sales/{id}/tasks/{n}/documents/{slot}", h.upload)
		r.With(staff).Delete("/sales/{id}/tasks/{n}/documents/{slot}", h.remove)
		r.With(staff).Get("/documents/{id}/download-url", h.downloadURL)
	})
}

func (h *Handler) actor(r *http.Request) auth.Actor {
	actor, _ := auth.ActorFrom(r.Context())
	return actor
}

func (h *Handler) uuidParam(w http.ResponseWriter, r *http.Request, name string, notFound error) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		httpx.WriteError(w, r, h.logger, notFound)
		return uuid.Nil, false
	}
	return id, true
}

// target reads the sale, the task number and the slot code from the path.
func (h *Handler) target(w http.ResponseWriter, r *http.Request) (uuid.UUID, int, string, bool) {
	saleID, ok := h.uuidParam(w, r, "id", errSaleNotFound)
	if !ok {
		return uuid.Nil, 0, "", false
	}
	number, err := strconv.Atoi(chi.URLParam(r, "n"))
	if err != nil || number < 1 || number > tasks.LastTask {
		httpx.WriteError(w, r, h.logger, errTaskNotFound)
		return uuid.Nil, 0, "", false
	}
	return saleID, number, chi.URLParam(r, "slot"), true
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	saleID, ok := h.uuidParam(w, r, "id", errSaleNotFound)
	if !ok {
		return
	}
	result, err := h.svc.List(r.Context(), h.actor(r), saleID)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) upload(w http.ResponseWriter, r *http.Request) {
	saleID, number, slot, ok := h.target(w, r)
	if !ok {
		return
	}
	fileName, data, reason, err := readUpload(w, r)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	file, replaced, err := h.svc.Upload(r.Context(), h.actor(r), UploadInput{
		SaleID: saleID, Number: number, Slot: slot, FileName: fileName, Data: data, Reason: reason,
	})
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	status := http.StatusCreated
	if replaced {
		status = http.StatusOK
	}
	httpx.WriteJSON(w, status, file)
}

func (h *Handler) remove(w http.ResponseWriter, r *http.Request) {
	saleID, number, slot, ok := h.target(w, r)
	if !ok {
		return
	}
	if err := h.svc.Remove(r.Context(), h.actor(r), saleID, number, slot); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) downloadURL(w http.ResponseWriter, r *http.Request) {
	id, ok := h.uuidParam(w, r, "id", errDocumentNotFound)
	if !ok {
		return
	}
	result, err := h.svc.DownloadURL(r.Context(), h.actor(r), id)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

// maxUploadBody allows the largest file plus the small reason part and the multipart framing.
const maxUploadBody = MaxBytes + 64<<10

const maxReasonPartBytes = 4 << 10

// readUpload reads a multipart request made of one file part called "file" and an optional text part called
// "reason". It streams the body and never keeps more than the file limit in memory.
func readUpload(w http.ResponseWriter, r *http.Request) (fileName string, data []byte, reason string, err error) {
	mediaType, params, perr := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if perr != nil || mediaType != "multipart/form-data" || params["boundary"] == "" {
		return "", nil, "", apperr.BadRequest("INVALID_MULTIPART", "La solicitud debe enviarse como multipart/form-data.")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBody)
	reader := multipart.NewReader(r.Body, params["boundary"])

	sawFile, sawReason := false, false
	for {
		part, nerr := reader.NextPart()
		if errors.Is(nerr, io.EOF) {
			break
		}
		if nerr != nil {
			return "", nil, "", multipartError(nerr)
		}
		switch part.FormName() {
		case "file":
			if sawFile {
				return "", nil, "", apperr.BadRequest("INVALID_MULTIPART", "Envíe un solo archivo.")
			}
			sawFile = true
			fileName = part.FileName()
			data, err = io.ReadAll(io.LimitReader(part, MaxBytes+1))
			if err != nil {
				return "", nil, "", multipartError(err)
			}
			if len(data) > MaxBytes {
				return "", nil, "", errTooLarge
			}
		case "reason":
			if sawReason {
				return "", nil, "", apperr.BadRequest("INVALID_MULTIPART", "La solicitud no es válida.")
			}
			sawReason = true
			text, rerr := io.ReadAll(io.LimitReader(part, maxReasonPartBytes+1))
			if rerr != nil {
				return "", nil, "", multipartError(rerr)
			}
			if len(text) > maxReasonPartBytes {
				return "", nil, "", apperr.BadRequest("INVALID_MULTIPART", "El motivo es demasiado largo.")
			}
			reason = string(text)
		default:
			return "", nil, "", apperr.BadRequest("INVALID_MULTIPART", "La solicitud contiene una parte desconocida.")
		}
	}
	return fileName, data, reason, nil
}

func multipartError(err error) error {
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		return errTooLarge
	}
	return apperr.BadRequest("INVALID_MULTIPART", "No se pudo leer la solicitud.").WithCause(err)
}
