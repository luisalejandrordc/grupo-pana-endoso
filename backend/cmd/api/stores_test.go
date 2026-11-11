package main

import (
	"context"
	"endosoflow/internal/platform/config"
	"io"
	"log/slog"
	"net/url"
	"strings"
	"testing"
	"time"
)

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func r2Config() config.Config {
	return config.Config{
		Env:                        "production",
		R2AccountID:                "account",
		R2CatalogBucket:            "endosoflow-catalog-images",
		R2CatalogAccessKeyID:       "catalog-key",
		R2CatalogSecretAccessKey:   "catalog-secret-access-key",
		R2CatalogPublicBaseURL:     "https://img.example.pe/",
		R2DocumentsBucket:          "endosoflow-sale-documents",
		R2DocumentsAccessKeyID:     "docs-key",
		R2DocumentsSecretAccessKey: "docs-secret",
		R2AvatarsBucket:            "endosoflow-employee-avatars",
		R2AvatarsAccessKeyID:       "avatars-key",
		R2AvatarsSecretAccessKey:   "avatars-secret",
	}
}

// signed returns the host/path and the access key a store puts in a signed address.
func signed(t *testing.T, s interface {
	SignedURL(context.Context, string, string, time.Duration) (string, error)
},
) (string, string) {
	t.Helper()
	raw, err := s.SignedURL(context.Background(), "k/file.pdf", "file.pdf", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u.Host + u.Path, strings.SplitN(u.Query().Get("X-Amz-Credential"), "/", 2)[0]
}

func TestEachBucketUsesItsOwnNameAndToken(t *testing.T) {
	cfg := r2Config()
	st := newStores(cfg, quiet())

	where, key := signed(t, st.Documents)
	if !strings.Contains(where, "endosoflow-sale-documents") || key != "docs-key" {
		t.Fatalf("documents: %s with %s", where, key)
	}
	where, key = signed(t, st.Avatars)
	if !strings.Contains(where, "endosoflow-employee-avatars") || key != "avatars-key" {
		t.Fatalf("avatars: %s with %s", where, key)
	}
	where, key = signed(t, st.Catalog)
	if !strings.Contains(where, "endosoflow-catalog-images") || key != "catalog-key" {
		t.Fatalf("catalog: %s with %s", where, key)
	}
}

func TestAPrivateBucketThatIsNotConfiguredFailsInsteadOfFallingBackToAPublicOne(t *testing.T) {
	cfg := r2Config()
	cfg.R2DocumentsBucket, cfg.R2AvatarsBucket = "", ""
	st := newStores(cfg, quiet())
	if err := st.Documents.Put(context.Background(), "k", "application/pdf", []byte("x")); err == nil {
		t.Fatal("documents must not be written anywhere when their bucket is missing")
	}
	if err := st.Avatars.Put(context.Background(), "k", "image/png", []byte("x")); err == nil {
		t.Fatal("avatars must not be written anywhere when their bucket is missing")
	}
}

func TestWithoutR2OnlyLocalAndTestKeepFilesOnDisk(t *testing.T) {
	cfg := config.Config{Env: "local", LocalMediaDir: t.TempDir(), MediaBaseURL: "http://localhost:8080/media"}
	if st := newStores(cfg, quiet()); st.Local == nil {
		t.Fatal("local development keeps files on disk")
	}
	cfg.Env = "production"
	st := newStores(cfg, quiet())
	if st.Local != nil {
		t.Fatal("production never serves files from disk")
	}
	if err := st.Documents.Put(context.Background(), "k", "application/pdf", []byte("x")); err == nil {
		t.Fatal("production without R2 must refuse to store documents")
	}
}

func TestDeletionsAreRoutedToTheirOwnBucket(t *testing.T) {
	st := newStores(r2Config(), quiet())
	for reason, want := range map[string]any{
		"catalog_image_replaced":  st.Catalog,
		"catalog_image_removed":   st.Catalog,
		"document_replaced":       st.Documents,
		"document_removed":        st.Documents,
		"sale_deleted":            st.Documents,
		"employee_photo_replaced": st.Avatars,
		"employee_photo_removed":  st.Avatars,
	} {
		if st.forReason(reason) != want {
			t.Errorf("%s is routed to the wrong bucket", reason)
		}
	}
}
