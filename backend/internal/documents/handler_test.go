package documents

import (
	"bytes"
	"endosoflow/internal/platform/apperr"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type part struct {
	name, fileName, content string
}

func upload(t *testing.T, parts ...part) *http.Request {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for _, p := range parts {
		var err error
		var field interface{ Write([]byte) (int, error) }
		if p.fileName != "" {
			field, err = w.CreateFormFile(p.name, p.fileName)
		} else {
			field, err = w.CreateFormField(p.name)
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, err := field.Write([]byte(p.content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPut, "/upload", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func codeOf(err error) string {
	if e, ok := err.(*apperr.Error); ok {
		return e.Code
	}
	return ""
}

func TestReadUploadTakesTheFileAndTheReason(t *testing.T) {
	req := upload(t, part{"reason", "", " error de digitación "}, part{"file", "a.pdf", "%PDF-1.7 hola"})
	name, data, reason, err := readUpload(httptest.NewRecorder(), req)
	if err != nil {
		t.Fatal(err)
	}
	if name != "a.pdf" || string(data) != "%PDF-1.7 hola" || reason != " error de digitación " {
		t.Fatalf("name=%q data=%q reason=%q", name, data, reason)
	}
}

func TestReadUploadWithoutAFileReturnsNoData(t *testing.T) {
	_, data, _, err := readUpload(httptest.NewRecorder(), upload(t, part{"reason", "", "x"}))
	if err != nil || len(data) != 0 {
		t.Fatalf("data=%d err=%v: the service answers DOCUMENT_REQUIRED for an empty upload", len(data), err)
	}
}

func TestReadUploadRefusesBadRequests(t *testing.T) {
	cases := map[string]*http.Request{
		"two files":        upload(t, part{"file", "a.pdf", "x"}, part{"file", "b.pdf", "y"}),
		"two reasons":      upload(t, part{"reason", "", "x"}, part{"reason", "", "y"}),
		"an unknown part":  upload(t, part{"other", "", "x"}),
		"a reason too big": upload(t, part{"reason", "", strings.Repeat("a", maxReasonPartBytes+1)}),
	}
	for name, req := range cases {
		if _, _, _, err := readUpload(httptest.NewRecorder(), req); codeOf(err) != "INVALID_MULTIPART" {
			t.Errorf("%s: got %v", name, err)
		}
	}
	notMultipart := httptest.NewRequest(http.MethodPut, "/upload", strings.NewReader("hola"))
	notMultipart.Header.Set("Content-Type", "application/json")
	if _, _, _, err := readUpload(httptest.NewRecorder(), notMultipart); codeOf(err) != "INVALID_MULTIPART" {
		t.Errorf("not multipart: got %v", err)
	}
}

func TestReadUploadRefusesAFileOverTenMegabytes(t *testing.T) {
	big := strings.Repeat("a", MaxBytes+1)
	_, _, _, err := readUpload(httptest.NewRecorder(), upload(t, part{"file", "big.pdf", big}))
	if codeOf(err) != "DOCUMENT_TOO_LARGE" {
		t.Fatalf("got %v", err)
	}
	exact := strings.Repeat("a", MaxBytes)
	if _, data, _, err := readUpload(httptest.NewRecorder(), upload(t, part{"file", "ok.pdf", exact})); err != nil || len(data) != MaxBytes {
		t.Fatalf("a file of exactly 10 MB must be accepted: %d, %v", len(data), err)
	}
}
