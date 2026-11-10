package storage

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Local keeps objects in a folder and serves them through Handler. It is for local development only.
type Local struct {
	dir     string
	baseURL string
}

// NewLocal stores files under dir. baseURL is where Handler is mounted, for example
// http://localhost:8080/media.
func NewLocal(dir, baseURL string) *Local {
	return &Local{dir: dir, baseURL: strings.TrimRight(baseURL, "/")}
}

// path maps a key to a file and refuses keys that would escape the folder.
func (l *Local) path(key string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(key))
	if key == "" || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("storage: invalid key %q", key)
	}
	return filepath.Join(l.dir, clean), nil
}

func (l *Local) Put(_ context.Context, key, _ string, data []byte) error {
	target, err := l.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, data, 0o644)
}

func (l *Local) Delete(_ context.Context, key string) error {
	target, err := l.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(target); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func (l *Local) PublicURL(key string) string { return l.baseURL + "/" + key }

// SignedURL returns the plain address of the file: the local folder has no signing, so anyone who knows the
// key can read it. That is acceptable for development only; production uses R2.
func (l *Local) SignedURL(_ context.Context, key, _ string, _ time.Duration) (string, error) {
	if _, err := l.path(key); err != nil {
		return "", err
	}
	return l.baseURL + "/" + key, nil
}

// Handler serves the stored files. Mount it at the path of the base URL, with the prefix stripped.
func (l *Local) Handler() http.Handler { return http.FileServer(http.Dir(l.dir)) }
