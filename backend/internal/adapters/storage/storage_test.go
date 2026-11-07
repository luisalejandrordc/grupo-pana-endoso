package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The expected signatures were produced by an independent implementation (botocore, S3 signer) for the same inputs;
// the first one is also the GET example printed in the AWS Signature Version 4 documentation.
func TestSignV4MatchesTheAWSDocumentationExample(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	req.Header.Set("Range", "bytes=0-9")
	signV4(req, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		"AKIAIOSFODNN7EXAMPLE", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "us-east-1", "s3",
		time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC))

	want := "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, " +
		"SignedHeaders=host;range;x-amz-content-sha256;x-amz-date, " +
		"Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"
	if got := req.Header.Get("Authorization"); got != want {
		t.Fatalf("authorization =\n%s\nwant\n%s", got, want)
	}
}

func TestSignV4MatchesTheReferenceForAPutWithAnEscapedKey(t *testing.T) {
	body := []byte("hello")
	sum := sha256.Sum256(body)
	req, _ := http.NewRequest(http.MethodPut, "https://acct.r2.cloudflarestorage.com/bucket/catalog/colors/a%20b/main.png", nil)
	req.Header.Set("Content-Type", "image/png")
	signV4(req, hex.EncodeToString(sum[:]), "AKIAIOSFODNN7EXAMPLE", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		"auto", "s3", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	if got := req.Header.Get("Authorization"); !strings.HasSuffix(got, "Signature=555996ccdfc4367b3913ae42f93a7db9efbe63f5829a635dfb0d0487d2152666") {
		t.Fatalf("authorization = %s", got)
	}
}

func TestR2PutAndDeleteSendSignedRequests(t *testing.T) {
	type seen struct{ method, path, auth, contentType, body, hash string }
	var calls []seen
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		calls = append(calls, seen{
			r.Method, r.URL.EscapedPath(), r.Header.Get("Authorization"), r.Header.Get("Content-Type"),
			string(body), r.Header.Get("x-amz-content-sha256"),
		})
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := NewR2(R2Config{
		AccountID: "acct", AccessKeyID: "AK", SecretAccessKey: "SK", Bucket: "bucket",
		PublicBaseURL: "https://img.example.pe/", Endpoint: server.URL,
		Now: func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) },
	})

	if err := store.Put(context.Background(), "catalog/colors/x y/main.png", "image/png", []byte("hello")); err != nil {
		t.Fatalf("put: %v", err)
	}
	if err := store.Delete(context.Background(), "catalog/colors/x y/main.png"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %d", len(calls))
	}
	put := calls[0]
	sum := sha256.Sum256([]byte("hello"))
	if put.method != http.MethodPut || put.path != "/bucket/catalog/colors/x%20y/main.png" || put.body != "hello" ||
		put.contentType != "image/png" || put.hash != hex.EncodeToString(sum[:]) {
		t.Fatalf("put = %+v", put)
	}
	if !strings.HasPrefix(put.auth, "AWS4-HMAC-SHA256 Credential=AK/20260102/auto/s3/aws4_request") {
		t.Fatalf("authorization = %s", put.auth)
	}
	if calls[1].method != http.MethodDelete || calls[1].auth == "" {
		t.Fatalf("delete = %+v", calls[1])
	}
	if got := store.PublicURL("catalog/a.png"); got != "https://img.example.pe/catalog/a.png" {
		t.Fatalf("public url = %s", got)
	}
}

func TestR2ErrorsNeverLeakCredentialsAndMissingObjectsAreFine(t *testing.T) {
	status := http.StatusForbidden
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte("<Error>denied</Error>"))
	}))
	defer server.Close()
	store := NewR2(R2Config{AccountID: "acct", AccessKeyID: "AK-SECRET-ID", SecretAccessKey: "SK-SECRET", Bucket: "b", Endpoint: server.URL})

	err := store.Put(context.Background(), "k", "image/png", []byte("x"))
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("a 403 must fail: %v", err)
	}
	status = http.StatusNotFound
	if err := store.Delete(context.Background(), "gone"); err != nil {
		t.Fatalf("deleting a missing object must succeed: %v", err)
	}

	down := NewR2(R2Config{AccountID: "acct", AccessKeyID: "AK-SECRET-ID", SecretAccessKey: "SK-SECRET", Bucket: "b", Endpoint: "http://127.0.0.1:1"})
	err = down.Put(context.Background(), "k", "image/png", []byte("x"))
	if err == nil || strings.Contains(err.Error(), "AK-SECRET-ID") || strings.Contains(err.Error(), "SK-SECRET") {
		t.Fatalf("a connection error must not include credentials: %v", err)
	}
}

func TestLocalStoreWritesDeletesAndRefusesPathTricks(t *testing.T) {
	dir := t.TempDir()
	store := NewLocal(dir, "http://localhost:8080/media/")
	ctx := context.Background()

	if err := store.Put(ctx, "catalog/a/main.png", "image/png", []byte("png")); err != nil {
		t.Fatalf("put: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "catalog", "a", "main.png")); err != nil || string(data) != "png" {
		t.Fatalf("file = %q, %v", data, err)
	}
	if got := store.PublicURL("catalog/a/main.png"); got != "http://localhost:8080/media/catalog/a/main.png" {
		t.Fatalf("public url = %s", got)
	}
	rec := httptest.NewRecorder()
	store.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/catalog/a/main.png", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "png" {
		t.Fatalf("serve: %d %q", rec.Code, rec.Body.String())
	}
	if err := store.Delete(ctx, "catalog/a/main.png"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := store.Delete(ctx, "catalog/a/main.png"); err != nil {
		t.Fatalf("deleting twice must succeed: %v", err)
	}
	for _, bad := range []string{"", "../escape.png", "a/../../escape.png", "/etc/passwd"} {
		if err := store.Put(ctx, bad, "image/png", []byte("x")); err == nil {
			t.Errorf("key %q must be refused", bad)
		}
	}
}

func TestMemoryStoreAndUnconfigured(t *testing.T) {
	m := NewMemory("https://m.test")
	_ = m.Put(context.Background(), "a", "image/png", []byte("x"))
	if o, ok := m.Get("a"); !ok || string(o.Data) != "x" || m.PublicURL("a") != "https://m.test/a" {
		t.Fatal("memory store broken")
	}
	_ = m.Delete(context.Background(), "a")
	if len(m.Keys()) != 0 {
		t.Fatal("delete did not remove the object")
	}
	if err := Unconfigured().Put(context.Background(), "a", "x", nil); err != ErrNotConfigured {
		t.Fatalf("unconfigured: %v", err)
	}
}

// The example of a presigned GET from the AWS documentation ("Authenticating Requests: Using Query
// Parameters"). The same signature proves the canonical request and key derivation are right.
func TestPresignV4MatchesTheAWSDocumentationExample(t *testing.T) {
	u, err := url.Parse("https://examplebucket.s3.amazonaws.com/test.txt")
	if err != nil {
		t.Fatal(err)
	}
	got := presignV4(u, "AKIAIOSFODNN7EXAMPLE", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "us-east-1", "s3",
		time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC), 86400*time.Second)
	want := "https://examplebucket.s3.amazonaws.com/test.txt?X-Amz-Algorithm=AWS4-HMAC-SHA256" +
		"&X-Amz-Credential=AKIAIOSFODNN7EXAMPLE%2F20130524%2Fus-east-1%2Fs3%2Faws4_request" +
		"&X-Amz-Date=20130524T000000Z&X-Amz-Expires=86400&X-Amz-SignedHeaders=host" +
		"&X-Amz-Signature=aeeed9bbccd4d02ee5c0109b86d86835f995330da4c265957d157751f604d404"
	if got != want {
		t.Fatalf("presigned URL\n got: %s\nwant: %s", got, want)
	}
}

func TestR2SignedURLCarriesTheFileNameAndTheLifetime(t *testing.T) {
	store := NewR2(R2Config{
		AccountID: "acct", AccessKeyID: "AKID", SecretAccessKey: "secret", Bucket: "docs",
		Now: func() time.Time { return time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC) },
	})
	raw, err := store.SignedURL(context.Background(), "documents/s1/d1.pdf", `Constancia "final"; ñ.pdf`, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if u.Host != "acct.r2.cloudflarestorage.com" || u.Path != "/docs/documents/s1/d1.pdf" {
		t.Fatalf("address = %s", raw)
	}
	q := u.Query()
	if q.Get("X-Amz-Expires") != "300" || q.Get("X-Amz-Signature") == "" || q.Get("X-Amz-SignedHeaders") != "host" {
		t.Fatalf("query = %v", q)
	}
	disposition := q.Get("response-content-disposition")
	if !strings.HasPrefix(disposition, "attachment; filename=") || strings.Contains(disposition, `"final"`) ||
		!strings.Contains(disposition, "filename*=UTF-8''") || !strings.Contains(disposition, "%C3%B1") {
		t.Fatalf("content disposition = %q", disposition)
	}
	if strings.Contains(raw, "secret") {
		t.Fatal("the secret key must never appear in a URL")
	}
	for _, ttl := range []time.Duration{0, -time.Second, 8 * 24 * time.Hour} {
		if _, err := store.SignedURL(context.Background(), "k", "a.pdf", ttl); err == nil {
			t.Errorf("a lifetime of %s must be refused", ttl)
		}
	}
}

func TestLocalMemoryAndUnconfiguredSignedURLs(t *testing.T) {
	local := NewLocal(t.TempDir(), "http://localhost:8080/media/")
	if got, err := local.SignedURL(context.Background(), "documents/a.pdf", "a.pdf", time.Minute); err != nil || got != "http://localhost:8080/media/documents/a.pdf" {
		t.Fatalf("local: %q, %v", got, err)
	}
	if _, err := local.SignedURL(context.Background(), "../secret", "a.pdf", time.Minute); err == nil {
		t.Fatal("local must refuse a key that escapes the folder")
	}
	mem := NewMemory("https://m.test")
	if got, _ := mem.SignedURL(context.Background(), "k", "a b.pdf", 5*time.Minute); got != "https://m.test/k?name=a+b.pdf&ttl=5m0s" {
		t.Fatalf("memory: %q", got)
	}
	if _, err := Unconfigured().SignedURL(context.Background(), "k", "a.pdf", time.Minute); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("unconfigured: %v", err)
	}
}
