package documents

import (
	"context"
	"endosoflow/internal/tasks"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type querier = tasks.Querier

type slotRow struct {
	Code       string
	TaskNumber int
	Format     string
}

// findSlot returns (nil, nil) when the slot code does not exist.
func findSlot(ctx context.Context, q querier, code string) (*slotRow, error) {
	var s slotRow
	err := q.QueryRow(ctx, `SELECT slot_code, task_number, allowed_format::text FROM document_slots WHERE slot_code = $1`, code).
		Scan(&s.Code, &s.TaskNumber, &s.Format)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &s, err
}

// stored is the part of a document row the service needs to replace or remove it.
type stored struct {
	ID          uuid.UUID
	Key         string
	FileName    string
	SizeBytes   int
	ChecksumSHA string
}

// findDocument returns the current file of a slot, or (nil, nil) when the slot is empty.
func findDocument(ctx context.Context, q querier, taskID uuid.UUID, slot string, forUpdate bool) (*stored, error) {
	sql := `SELECT id, storage_key, original_file_name, size_bytes, checksum_sha256 FROM documents
		WHERE task_instance_id = $1 AND slot_code = $2`
	if forUpdate {
		sql += ` FOR UPDATE`
	}
	var d stored
	err := q.QueryRow(ctx, sql, taskID, slot).Scan(&d.ID, &d.Key, &d.FileName, &d.SizeBytes, &d.ChecksumSHA)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &d, err
}

// reference is what a download needs to know about a document.
type reference struct {
	SaleID   uuid.UUID
	Key      string
	FileName string
	Mime     string
}

func findReference(ctx context.Context, q querier, id uuid.UUID) (*reference, error) {
	var r reference
	err := q.QueryRow(ctx, `SELECT sale_id, storage_key, original_file_name, mime_type FROM documents WHERE id = $1`, id).
		Scan(&r.SaleID, &r.Key, &r.FileName, &r.Mime)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &r, err
}

func insertDocument(ctx context.Context, q querier, id, saleID, taskID uuid.UUID, slot, fileName, mime string,
	size int, key, checksum string, actor uuid.UUID, now time.Time,
) error {
	_, err := q.Exec(ctx, `INSERT INTO documents
		(id, sale_id, task_instance_id, slot_code, original_file_name, mime_type, size_bytes, storage_key,
		 checksum_sha256, uploaded_by, uploaded_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		id, saleID, taskID, slot, fileName, mime, size, key, checksum, actor, now)
	return err
}

func replaceDocument(ctx context.Context, q querier, id uuid.UUID, fileName, mime string, size int, key, checksum string,
	actor uuid.UUID, now time.Time,
) error {
	_, err := q.Exec(ctx, `UPDATE documents SET original_file_name = $2, mime_type = $3, size_bytes = $4, storage_key = $5,
		checksum_sha256 = $6, uploaded_by = $7, uploaded_at = $8, row_version = row_version + 1 WHERE id = $1`,
		id, fileName, mime, size, key, checksum, actor, now)
	return err
}

func deleteDocument(ctx context.Context, q querier, id uuid.UUID) error {
	_, err := q.Exec(ctx, `DELETE FROM documents WHERE id = $1`, id)
	return err
}

// readFile returns the public description of a stored document.
func readFile(ctx context.Context, q querier, id uuid.UUID) (*File, error) {
	var f File
	err := q.QueryRow(ctx, `SELECT d.id, d.original_file_name, d.mime_type, d.size_bytes, d.checksum_sha256, d.uploaded_by,
			e.first_name || ' ' || e.last_name, d.uploaded_at
		FROM documents d JOIN employees e ON e.id = d.uploaded_by WHERE d.id = $1`, id).
		Scan(&f.ID, &f.FileName, &f.MimeType, &f.SizeBytes, &f.ChecksumSHA, &f.UploadedByID, &f.UploadedBy, &f.UploadedAt)
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// listSlots returns every slot with the current file of the sale, in task order. required says which slots the
// task engine needs for this customer type.
func listSlots(ctx context.Context, q querier, saleID uuid.UUID, customerType string) ([]Slot, error) {
	rows, err := q.Query(ctx, `SELECT sl.slot_code, sl.task_number, sl.label_es, sl.allowed_format::text, sl.is_customer_visible,
			d.id, d.original_file_name, d.mime_type, d.size_bytes, d.checksum_sha256, d.uploaded_by,
			e.first_name || ' ' || e.last_name, d.uploaded_at
		FROM document_slots sl
		LEFT JOIN documents d ON d.slot_code = sl.slot_code AND d.sale_id = $1
		LEFT JOIN employees e ON e.id = d.uploaded_by
		ORDER BY sl.task_number, sl.slot_code`, saleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Slot{}
	for rows.Next() {
		var s Slot
		var (
			id         *uuid.UUID
			name, mime *string
			size       *int
			sum        *string
			byID       *uuid.UUID
			by         *string
			at         *time.Time
		)
		if err := rows.Scan(&s.SlotCode, &s.TaskNumber, &s.Label, &s.AllowedFormat, &s.CustomerVisible,
			&id, &name, &mime, &size, &sum, &byID, &by, &at); err != nil {
			return nil, err
		}
		if id != nil {
			s.File = &File{
				ID: *id, FileName: *name, MimeType: *mime, SizeBytes: *size, ChecksumSHA: *sum,
				UploadedByID: *byID, UploadedBy: *by, UploadedAt: *at,
			}
		}
		for _, code := range tasks.RequiredSlots(s.TaskNumber, customerType) {
			if code == s.SlotCode {
				s.Required = true
			}
		}
		items = append(items, s)
	}
	return items, rows.Err()
}

// presentSlots returns the slot codes that already have a file for the task.
func presentSlots(ctx context.Context, q querier, taskID uuid.UUID, wanted []string) (map[string]bool, error) {
	rows, err := q.Query(ctx, `SELECT slot_code FROM documents WHERE task_instance_id = $1 AND slot_code = ANY($2)`, taskID, wanted)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	present := map[string]bool{}
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		present[code] = true
	}
	return present, rows.Err()
}
