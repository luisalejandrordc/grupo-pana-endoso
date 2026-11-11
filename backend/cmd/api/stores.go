package main

import (
	"endosoflow/internal/adapters/storage"
	"endosoflow/internal/platform/config"
	"log/slog"
)

// stores are the three object-storage buckets of the application. Each has its own bucket and, when the
// configuration gives one, its own access token, so a leaked catalog token cannot read private files.
type stores struct {
	// Catalog holds the vehicle images. It is public: the browser loads them from R2_PUBLIC_BASE_URL.
	Catalog storage.Store
	// Documents holds the files of the sale tasks. It is private: files are reachable only through short-lived
	// signed addresses issued after the caller's role and scope were checked.
	Documents storage.Store
	// Avatars holds the employees' profile pictures. It is private, like Documents.
	Avatars storage.Store
	// Local serves the development folder at /media; nil when R2 is used or outside local and test.
	Local *storage.Local
}

// newStores builds the stores from the configuration. With R2 configured, a private bucket that has no name
// answers STORAGE_UNAVAILABLE instead of falling back to a public place. Local and test environments keep every
// file in one folder served at /media, which is public and for development only. Anywhere else without R2,
// uploads and downloads fail loudly.
func newStores(cfg config.Config, logger *slog.Logger) stores {
	st := stores{Catalog: storage.Unconfigured(), Documents: storage.Unconfigured(), Avatars: storage.Unconfigured()}
	switch {
	case cfg.R2Configured():
		st.Catalog = storage.NewR2(storage.R2Config{
			AccountID:       cfg.R2AccountID,
			Bucket:          cfg.R2CatalogBucket,
			AccessKeyID:     cfg.R2CatalogAccessKeyID,
			SecretAccessKey: cfg.R2CatalogSecretAccessKey,
			PublicBaseURL:   cfg.R2CatalogPublicBaseURL,
		})
		st.Documents = storage.NewR2(storage.R2Config{
			AccountID:       cfg.R2AccountID,
			Bucket:          cfg.R2DocumentsBucket,
			AccessKeyID:     cfg.R2DocumentsAccessKeyID,
			SecretAccessKey: cfg.R2DocumentsSecretAccessKey,
		})
		st.Avatars = storage.NewR2(storage.R2Config{
			AccountID:       cfg.R2AccountID,
			Bucket:          cfg.R2AvatarsBucket,
			AccessKeyID:     cfg.R2AvatarsAccessKeyID,
			SecretAccessKey: cfg.R2AvatarsSecretAccessKey,
		})
	case cfg.Env == "local" || cfg.Env == "test":
		st.Local = storage.NewLocal(cfg.LocalMediaDir, cfg.MediaBaseURL)
		st.Catalog, st.Documents, st.Avatars = st.Local, st.Local, st.Local
		logger.Warn("R2 is not configured: images, documents and photos are kept on local disk, which is public: development only",
			"dir", cfg.LocalMediaDir)
	default:
		logger.Warn("R2 is not configured: catalog images, documents and photos cannot be stored")
	}
	return st
}
