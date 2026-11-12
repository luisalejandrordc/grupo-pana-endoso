package storagecleanup_test

import (
	"context"
	"endosoflow/internal/adapters/storage"
	"endosoflow/internal/storagecleanup"
	"endosoflow/internal/testkit"
	"io"
	"log/slog"
	"strings"
	"testing"
)

// Each queued deletion goes to the bucket its reason belongs to, and only to that one. The test works inside a
// transaction that it rolls back, so it neither sees nor changes the deletions of tests running elsewhere.
func TestRetryOpenRoutedDeletesFromTheRightBucket(t *testing.T) {
	env := testkit.New(t)
	tx, err := env.Pool.Begin(env.Ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())

	catalog, documents, avatars := storage.NewMemory("https://img.test"), storage.NewMemory(""), storage.NewMemory("")
	all := []*storage.Memory{catalog, documents, avatars}
	for _, key := range []string{"routed/catalog.png", "routed/doc.pdf", "routed/photo.png", "routed/sale.pdf"} {
		for _, s := range all {
			if err := s.Put(env.Ctx, key, "application/octet-stream", []byte("x")); err != nil {
				t.Fatal(err)
			}
		}
	}
	for key, reason := range map[string]string{
		"routed/catalog.png": storagecleanup.CatalogImageRemoved,
		"routed/doc.pdf":     storagecleanup.DocumentReplaced,
		"routed/photo.png":   storagecleanup.EmployeePhotoReplaced,
		"routed/sale.pdf":    storagecleanup.SaleDeleted,
	} {
		if _, err := storagecleanup.Enqueue(env.Ctx, tx, key, reason); err != nil {
			t.Fatal(err)
		}
	}

	route := func(reason string) storage.Store {
		switch {
		case strings.HasPrefix(reason, "catalog_"):
			return catalog
		case strings.HasPrefix(reason, "employee_photo_"):
			return avatars
		}
		return documents
	}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	if n, err := storagecleanup.RetryOpenRouted(env.Ctx, tx, route, quiet, 1000); err != nil || n < 4 {
		t.Fatalf("retry: %d %v", n, err)
	}

	has := func(s *storage.Memory, key string) bool { _, ok := s.Get(key); return ok }
	if has(catalog, "routed/catalog.png") || !has(documents, "routed/catalog.png") || !has(avatars, "routed/catalog.png") {
		t.Fatal("a catalog image deletion touches the catalog bucket only")
	}
	if has(avatars, "routed/photo.png") || !has(catalog, "routed/photo.png") || !has(documents, "routed/photo.png") {
		t.Fatal("a photo deletion touches the avatars bucket only")
	}
	if has(documents, "routed/doc.pdf") || has(documents, "routed/sale.pdf") || !has(catalog, "routed/doc.pdf") || !has(avatars, "routed/sale.pdf") {
		t.Fatal("document and sale deletions touch the documents bucket only")
	}
}
