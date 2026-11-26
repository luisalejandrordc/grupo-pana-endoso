package documents

import (
	"context"
	"endosoflow/internal/tasks"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// What a customer may download. A file is visible to a customer only when ALL of these hold: the sale is theirs,
// its slot is marked customer-visible, and its task is completed. A reopened or restarted task takes its files
// out of the portal until it is completed again. Everything else (the Excel quotation, the settlement, Toyota's
// payment, the endorsed and customs documents, service orders) is never offered.

// CustomerDocument is a file a customer may download.
type CustomerDocument struct {
	ID         uuid.UUID `json:"id"`
	TaskNumber int       `json:"taskNumber"`
	Label      string    `json:"label"`
	FileName   string    `json:"fileName"`
	SizeBytes  int       `json:"sizeBytes"`
	UploadedAt time.Time `json:"uploadedAt"`
}

// CustomerDocuments lists the files of a sale a customer may download, by task. The caller has already checked
// that the sale is the customer's.
func CustomerDocuments(ctx context.Context, q tasks.Querier, saleID uuid.UUID) ([]CustomerDocument, error) {
	rows, err := q.Query(ctx, `SELECT d.id, t.task_number, s.label_es, d.original_file_name, d.size_bytes, d.uploaded_at
		FROM documents d
		JOIN document_slots s ON s.slot_code = d.slot_code
		JOIN task_instances t ON t.id = d.task_instance_id
		WHERE d.sale_id = $1 AND s.is_customer_visible AND t.status = 'completed'
		ORDER BY t.task_number, s.slot_code`, saleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CustomerDocument{}
	for rows.Next() {
		var d CustomerDocument
		if err := rows.Scan(&d.ID, &d.TaskNumber, &d.Label, &d.FileName, &d.SizeBytes, &d.UploadedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// CustomerDownloadURL issues a short-lived signed address for a file of the customer's. A single query decides
// whether the customer may have it, so a file that is someone else's, hidden from customers, or whose task is not
// completed answers exactly like one that does not exist: nothing reveals that it is there.
func (s *Service) CustomerDownloadURL(ctx context.Context, customerID, documentID uuid.UUID) (*DownloadURL, error) {
	var saleID uuid.UUID
	var key, fileName, mime string
	err := s.db.QueryRow(ctx, `SELECT d.sale_id, d.storage_key, d.original_file_name, d.mime_type
		FROM documents d
		JOIN sale_registers r ON r.id = d.sale_id
		JOIN document_slots sl ON sl.slot_code = d.slot_code
		JOIN task_instances t ON t.id = d.task_instance_id
		WHERE d.id = $1 AND r.customer_id = $2 AND sl.is_customer_visible AND t.status = 'completed'`,
		documentID, customerID).Scan(&saleID, &key, &fileName, &mime)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errDocumentNotFound
	}
	if err != nil {
		return nil, err
	}
	signed, err := s.store.SignedURL(ctx, key, fileName, s.ttl)
	if err != nil {
		s.logger.Error("could not sign a customer's document download", "document", documentID, "error", err)
		return nil, errStorage.WithCause(err)
	}
	s.logger.Info("customer document download issued", "document", documentID, "sale", saleID, "customer", customerID)
	return &DownloadURL{URL: signed, ExpiresAt: s.now().Add(s.ttl), FileName: fileName, MimeType: mime}, nil
}
