package staff

import (
	"context"
	"endosoflow/internal/audit"
	"endosoflow/internal/auth"
	"endosoflow/internal/platform/apperr"
	"endosoflow/internal/platform/database"
	"endosoflow/internal/platform/ids"
	"endosoflow/internal/storagecleanup"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// MaxPhotoBytes is the largest accepted profile picture.
const MaxPhotoBytes = 2 << 20

// photoLinkTTL is how long the address of a photo works. Photos are shown on every screen, so the address lives
// longer than a document's, but it is still short: a copied address stops working soon.
const photoLinkTTL = 10 * time.Minute

var photoTypes = map[string]string{"image/jpeg": "jpg", "image/png": "png", "image/webp": "webp"}

var (
	errPhotoRequired = apperr.Unprocessable("PHOTO_REQUIRED", "Falta la foto.").
				WithFields(apperr.FieldError{Field: "file", Code: "REQUIRED", Message: "La foto es obligatoria."})
	errPhotoTooLarge = apperr.New(http.StatusRequestEntityTooLarge, "PHOTO_TOO_LARGE", "La foto supera el máximo de 2 MB.")
	errPhotoType     = apperr.Unprocessable("PHOTO_TYPE_NOT_ALLOWED", "La foto debe ser JPEG, PNG o WebP.")
	errPhotoNotFound = apperr.NotFound("PHOTO_NOT_FOUND", "El empleado no tiene foto.")
	errStorage       = apperr.New(http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "No se pudo guardar la foto. Intente nuevamente más tarde.")
)

// PhotoLink is a short-lived address from which a photo can be loaded.
type PhotoLink struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expiresAt"`
	MimeType  string    `json:"mimeType"`
}

// SetPhoto stores the caller's own profile picture, replacing the previous one. The type comes from the file's
// content, never from what the client says. The old file is deleted from storage once the change has committed.
func (s *Service) SetPhoto(ctx context.Context, actor auth.Actor, data []byte) (*PhotoLink, error) {
	switch {
	case len(data) == 0:
		return nil, errPhotoRequired
	case len(data) > MaxPhotoBytes:
		return nil, errPhotoTooLarge
	}
	mime := http.DetectContentType(data)
	ext, ok := photoTypes[mime]
	if !ok {
		return nil, errPhotoType
	}

	key := fmt.Sprintf("employees/%s/%s.%s", actor.ID, ids.New(), ext)
	if err := s.avatars.Put(ctx, key, mime, data); err != nil {
		s.logger.Error("could not store a profile picture", "error", err)
		return nil, errStorage.WithCause(err)
	}

	now := s.now()
	var old []storagecleanup.Pending
	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		var previous *string
		if err := tx.QueryRow(ctx, `SELECT profile_picture_key FROM employees WHERE id = $1 AND NOT is_system FOR UPDATE`, actor.ID).Scan(&previous); err != nil {
			if err == pgx.ErrNoRows {
				return errNotFound
			}
			return err
		}
		// The version is left alone: a new picture must not make an Admin's open edit form stale.
		if _, err := tx.Exec(ctx, `UPDATE employees SET profile_picture_key = $2, profile_picture_mime = $3, profile_picture_size = $4,
			updated_by = $1, updated_at = $5 WHERE id = $1`, actor.ID, key, mime, len(data), now); err != nil {
			return err
		}
		action := "set"
		if previous != nil {
			action = "replaced"
			pending, err := storagecleanup.Enqueue(ctx, tx, *previous, storagecleanup.EmployeePhotoReplaced)
			if err != nil {
				return err
			}
			old = append(old, pending)
		}
		return audit.Record(ctx, tx, audit.Event{
			ActorID: &actor.ID, EntityType: audit.EntityEmployee, EntityID: actor.ID, Action: audit.ActionUpdated,
			Changes: map[string]any{"photo": action, "mimeType": mime, "sizeBytes": len(data)},
		}, now)
	})
	if err != nil {
		// The new object has no row pointing at it: queue its deletion so it cannot linger.
		s.discard(ctx, key)
		return nil, apperr.FromError(err)
	}
	storagecleanup.Process(ctx, s.db, s.avatars, s.logger, old...)
	return s.link(ctx, key, mime, now)
}

// RemovePhoto removes the caller's own profile picture. Removing a picture that does not exist is not an error.
func (s *Service) RemovePhoto(ctx context.Context, actor auth.Actor) error {
	now := s.now()
	var old []storagecleanup.Pending
	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		var previous *string
		if err := tx.QueryRow(ctx, `SELECT profile_picture_key FROM employees WHERE id = $1 AND NOT is_system FOR UPDATE`, actor.ID).Scan(&previous); err != nil {
			if err == pgx.ErrNoRows {
				return errNotFound
			}
			return err
		}
		if previous == nil {
			return nil
		}
		if _, err := tx.Exec(ctx, `UPDATE employees SET profile_picture_key = NULL, profile_picture_mime = NULL, profile_picture_size = NULL,
			updated_by = $1, updated_at = $2 WHERE id = $1`, actor.ID, now); err != nil {
			return err
		}
		pending, err := storagecleanup.Enqueue(ctx, tx, *previous, storagecleanup.EmployeePhotoRemoved)
		if err != nil {
			return err
		}
		old = append(old, pending)
		return audit.Record(ctx, tx, audit.Event{
			ActorID: &actor.ID, EntityType: audit.EntityEmployee, EntityID: actor.ID, Action: audit.ActionUpdated,
			Changes: map[string]any{"photo": "removed"},
		}, now)
	})
	if err != nil {
		return apperr.FromError(err)
	}
	storagecleanup.Process(ctx, s.db, s.avatars, s.logger, old...)
	return nil
}

// PhotoURL returns a short-lived address for an employee's photo. Any signed-in staff member may ask for any
// employee's photo (FR-019); the picture itself is never reachable without one of these addresses.
func (s *Service) PhotoURL(ctx context.Context, id uuid.UUID) (*PhotoLink, error) {
	var key, mime *string
	err := s.db.QueryRow(ctx, `SELECT profile_picture_key, profile_picture_mime FROM employees WHERE id = $1 AND NOT is_system`, id).Scan(&key, &mime)
	if err == pgx.ErrNoRows {
		return nil, errNotFound
	}
	if err != nil {
		return nil, apperr.FromError(err)
	}
	if key == nil || mime == nil {
		return nil, errPhotoNotFound
	}
	return s.link(ctx, *key, *mime, s.now())
}

func (s *Service) link(ctx context.Context, key, mime string, now time.Time) (*PhotoLink, error) {
	url, err := s.avatars.SignedURL(ctx, key, "", photoLinkTTL)
	if err != nil {
		s.logger.Error("could not sign a photo address", "error", err)
		return nil, errStorage.WithCause(err)
	}
	return &PhotoLink{URL: url, ExpiresAt: now.Add(photoLinkTTL), MimeType: mime}, nil
}

// discard queues the deletion of an object that was stored but never recorded, and tries it once.
func (s *Service) discard(ctx context.Context, key string) {
	pending, err := storagecleanup.Enqueue(ctx, s.db, key, storagecleanup.EmployeePhotoRemoved)
	if err != nil {
		s.logger.Error("could not queue the deletion of an unrecorded photo", "key", key, "error", err)
		return
	}
	storagecleanup.Process(ctx, s.db, s.avatars, s.logger, pending)
}
