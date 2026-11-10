// Package storage keeps uploaded files (catalog images today; documents and photos later) behind one
// small interface. Cloudflare R2 is the real store; a folder on disk serves local development; a map in
// memory serves tests.
package storage

import (
	"context"
	"errors"
	"net/url"
	"sync"
	"time"
)

// Store saves and removes objects and says where the public ones can be read.
type Store interface {
	// Put stores data under key, replacing any object already there.
	Put(ctx context.Context, key, contentType string, data []byte) error
	// Delete removes the object. Removing an object that does not exist is not an error.
	Delete(ctx context.Context, key string) error
	// PublicURL is the address where a public object can be read.
	PublicURL(key string) string
	// SignedURL is a short-lived address where a private object can be downloaded, saved under fileName.
	// The caller must have checked who may read the object before asking for one.
	SignedURL(ctx context.Context, key, fileName string, ttl time.Duration) (string, error)
}

// ErrNotConfigured is returned by the store used where no real storage exists, so a missing store is
// never silent.
var ErrNotConfigured = errors.New("storage: no file store is configured for this environment")

type unconfigured struct{}

func (unconfigured) Put(context.Context, string, string, []byte) error { return ErrNotConfigured }
func (unconfigured) Delete(context.Context, string) error              { return ErrNotConfigured }
func (unconfigured) PublicURL(string) string                           { return "" }
func (unconfigured) SignedURL(context.Context, string, string, time.Duration) (string, error) {
	return "", ErrNotConfigured
}

// Unconfigured returns a store whose writes always fail.
func Unconfigured() Store { return unconfigured{} }

// Memory keeps objects in a map. It is for tests.
type Memory struct {
	mu      sync.Mutex
	objects map[string]Object
	base    string
	// FailPut and FailDelete make the next call of that kind fail, once, with the given error.
	FailPut    error
	FailDelete error
}

// Object is a stored file.
type Object struct {
	ContentType string
	Data        []byte
}

// NewMemory returns an empty in-memory store whose public URLs start with base.
func NewMemory(base string) *Memory {
	return &Memory{objects: map[string]Object{}, base: base}
}

func (m *Memory) Put(_ context.Context, key, contentType string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.FailPut != nil {
		err := m.FailPut
		m.FailPut = nil
		return err
	}
	m.objects[key] = Object{ContentType: contentType, Data: append([]byte(nil), data...)}
	return nil
}

func (m *Memory) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.FailDelete != nil {
		err := m.FailDelete
		m.FailDelete = nil
		return err
	}
	delete(m.objects, key)
	return nil
}

func (m *Memory) PublicURL(key string) string { return m.base + "/" + key }

// SignedURL returns an address that carries the file name and lifetime, so tests can check both.
func (m *Memory) SignedURL(_ context.Context, key, fileName string, ttl time.Duration) (string, error) {
	return m.base + "/" + key + "?name=" + url.QueryEscape(fileName) + "&ttl=" + ttl.String(), nil
}

// Get returns a stored object, for assertions.
func (m *Memory) Get(key string) (Object, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.objects[key]
	return o, ok
}

// Keys lists the stored keys, for assertions.
func (m *Memory) Keys() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	keys := make([]string, 0, len(m.objects))
	for k := range m.objects {
		keys = append(keys, k)
	}
	return keys
}
