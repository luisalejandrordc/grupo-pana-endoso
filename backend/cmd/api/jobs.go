package main

import (
	"context"
	"endosoflow/internal/adapters/mail"
	"endosoflow/internal/adapters/storage"
	"endosoflow/internal/notifications"
	"endosoflow/internal/platform/config"
	"endosoflow/internal/storagecleanup"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// mailer is the email sender together with whether a real delivery service stands behind it.
type mailer struct {
	mail.Sender
	delivers bool
}

// forReason says which bucket a queued file deletion belongs to: a deletion records why it exists, not where
// the file is.
func (s stores) forReason(reason string) storage.Store {
	switch {
	case strings.HasPrefix(reason, "catalog_"):
		return s.Catalog
	case strings.HasPrefix(reason, "employee_photo_"):
		return s.Avatars
	default: // document_* and sale_deleted
		return s.Documents
	}
}

// storageRetryEvery is how often file deletions that failed the first time are tried again.
const storageRetryEvery = 10 * time.Minute

// startJobs starts the background work of the process and returns a function that waits for it to stop. The
// jobs end when ctx is cancelled; each finishes the item it is working on first.
//
//   - The notification dispatcher sends queued emails (only when a delivery service is configured).
//   - The storage retry removes files whose deletion failed the first time.
func startJobs(ctx context.Context, cfg config.Config, pool *pgxpool.Pool, st stores, sender mailer, logger *slog.Logger) (wait func()) {
	var wg sync.WaitGroup
	if sender.delivers {
		dispatcher := notifications.NewDispatcher(notifications.DispatcherDeps{
			DB: pool, Sender: sender.Sender, AppBaseURL: cfg.AppBaseURL, Logger: logger,
		})
		wg.Add(1)
		go func() {
			defer wg.Done()
			dispatcher.Run(ctx, cfg.MailDispatchInterval)
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(storageRetryEvery)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if n, err := storagecleanup.RetryOpenRouted(ctx, pool, st.forReason, logger, 100); err != nil && ctx.Err() == nil {
					logger.Error("could not retry the pending file deletions", "error", err)
				} else if n > 0 {
					logger.Info("retried pending file deletions", "count", n)
				}
			}
		}
	}()
	return wg.Wait
}
