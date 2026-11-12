package staff

import (
	"bytes"
	"context"
	"encoding/json"
	"endosoflow/internal/adapters/storage"
	"endosoflow/internal/auth"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func pngPhoto(t *testing.T, shade uint8) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for x := 0; x < 8; x++ {
		for y := 0; y < 8; y++ {
			img.Set(x, y, color.RGBA{shade, 20, 30, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func jpegPhoto(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 8, 8)), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func (e *testEnv) employee(t *testing.T, role string) (testEmployee, auth.Actor) {
	t.Helper()
	emp := e.insertEmployee(t, role, goodPassword, "Foto")
	return emp, actorOf(emp.ID, auth.Role(role))
}

func (e *testEnv) rowVersion(t *testing.T, id uuid.UUID) int {
	t.Helper()
	var v int
	if err := e.pool.QueryRow(e.ctx, `SELECT row_version FROM employees WHERE id = $1`, id).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func (e *testEnv) photoAudit(t *testing.T, id uuid.UUID) []string {
	t.Helper()
	rows, err := e.pool.Query(e.ctx, `SELECT changes_summary->>'photo' FROM audit_logs WHERE entity_id = $1 AND changes_summary ? 'photo' ORDER BY created_at, id`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

func TestSettingReplacingAndRemovingAPhoto(t *testing.T) {
	e := newTestEnv(t)
	emp, actor := e.employee(t, "sales_advisor")
	t.Cleanup(func() { e.cleanup(emp.ID) })
	before := e.rowVersion(t, emp.ID)

	link, err := e.staff.SetPhoto(e.ctx, actor, pngPhoto(t, 10))
	if err != nil || link.MimeType != "image/png" || link.URL == "" || !link.ExpiresAt.After(time.Now()) {
		t.Fatalf("first photo: %v %+v", err, link)
	}
	if got := e.avatars.Keys(); len(got) != 1 || !strings.HasPrefix(got[0], "employees/"+emp.ID.String()+"/") || !strings.HasSuffix(got[0], ".png") {
		t.Fatalf("the photo is stored under the employee's folder, named by its content type: %v", got)
	}
	first := e.avatars.Keys()[0]

	link, err = e.staff.SetPhoto(e.ctx, actor, jpegPhoto(t))
	if err != nil || link.MimeType != "image/jpeg" {
		t.Fatalf("replacement: %v %+v", err, link)
	}
	if _, ok := e.avatars.Get(first); ok || len(e.avatars.Keys()) != 1 {
		t.Fatalf("the old picture must leave storage once the change committed: %v", e.avatars.Keys())
	}
	if v := e.rowVersion(t, emp.ID); v != before {
		t.Fatalf("a photo must not make an open edit form stale: version %d -> %d", before, v)
	}

	if err := e.staff.RemovePhoto(e.ctx, actor); err != nil {
		t.Fatal(err)
	}
	if err := e.staff.RemovePhoto(e.ctx, actor); err != nil {
		t.Fatalf("removing a picture that is gone is not an error: %v", err)
	}
	if len(e.avatars.Keys()) != 0 {
		t.Fatalf("nothing may remain in storage: %v", e.avatars.Keys())
	}
	if _, err := e.staff.PhotoURL(e.ctx, emp.ID); code(err) != "PHOTO_NOT_FOUND" {
		t.Fatalf("an employee without a picture: %v", err)
	}
	if got := e.photoAudit(t, emp.ID); strings.Join(got, ",") != "set,replaced,removed" {
		t.Fatalf("every change is audited, once: %v", got)
	}
}

func TestPhotosAreCheckedByContentAndSize(t *testing.T) {
	e := newTestEnv(t)
	emp, actor := e.employee(t, "sales_advisor")
	t.Cleanup(func() { e.cleanup(emp.ID) })

	for name, c := range map[string]struct {
		data []byte
		want string
	}{
		"empty":                  {nil, "PHOTO_REQUIRED"},
		"a PDF named like image": {[]byte("%PDF-1.7 not a picture"), "PHOTO_TYPE_NOT_ALLOWED"},
		"a GIF":                  {[]byte("GIF89a\x01\x00\x01\x00\x80\x00\x00\x00\x00\x00\xff\xff\xff!\xf9\x04\x01\x00\x00\x00\x00,\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x02D\x01\x00;"), "PHOTO_TYPE_NOT_ALLOWED"},
		"text":                   {[]byte("hello"), "PHOTO_TYPE_NOT_ALLOWED"},
		"too large":              {append(pngPhoto(t, 1), make([]byte, MaxPhotoBytes)...), "PHOTO_TOO_LARGE"},
	} {
		if _, err := e.staff.SetPhoto(e.ctx, actor, c.data); code(err) != c.want {
			t.Errorf("%s: %v, want %s", name, err, c.want)
		}
	}
	if len(e.avatars.Keys()) != 0 {
		t.Fatalf("a refused photo must never reach storage: %v", e.avatars.Keys())
	}
}

type brokenStore struct{ storage.Store }

func (brokenStore) Put(context.Context, string, string, []byte) error {
	return errors.New("storage is down")
}

func TestAStorageOutageLeavesTheEmployeeUnchanged(t *testing.T) {
	e := newTestEnv(t)
	emp, actor := e.employee(t, "sales_advisor")
	t.Cleanup(func() { e.cleanup(emp.ID) })
	svc := NewService(Deps{DB: e.pool, Activation: e.auth, Access: e.auth, Avatars: brokenStore{e.avatars}, Logger: e.staff.logger})
	if _, err := svc.SetPhoto(e.ctx, actor, pngPhoto(t, 3)); code(err) != "STORAGE_UNAVAILABLE" {
		t.Fatalf("%v", err)
	}
	if _, err := e.staff.PhotoURL(e.ctx, emp.ID); code(err) != "PHOTO_NOT_FOUND" {
		t.Fatalf("no picture may be recorded when the file could not be stored: %v", err)
	}
}

// --- over HTTP ---

func multipartBody(t *testing.T, field, name string, data []byte) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile(field, name)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(data)
	_ = w.Close()
	return &body, w.FormDataContentType()
}

func upload(router http.Handler, token string, body *bytes.Buffer, contentType string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPut, "/api/v1/me/photo", body)
	req.Header.Set("Content-Type", contentType)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestPhotoEndpoints(t *testing.T) {
	e := newTestEnv(t)
	router := newTestRouter(e)
	advisor, _ := e.employee(t, "sales_advisor")
	colleague, _ := e.employee(t, "sales_advisor")
	t.Cleanup(func() { e.cleanup(advisor.ID); e.cleanup(colleague.ID) })
	advisorToken, colleagueToken := e.tokenFor(t, advisor), e.tokenFor(t, colleague)
	photoPath := "/api/v1/employees/" + advisor.ID.String() + "/photo"

	body, ct := multipartBody(t, "file", "yo.png", pngPhoto(t, 50))
	if rec := upload(router, "", body, ct); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous upload: %d", rec.Code)
	}
	body, ct = multipartBody(t, "file", "yo.png", pngPhoto(t, 50))
	rec := upload(router, advisorToken, body, ct)
	var link PhotoLink
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &link) != nil || link.URL == "" {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body.String())
	}

	// Anyone signed in may look at anyone's picture, but anonymous visitors may not.
	if rec := send(router, http.MethodGet, photoPath, "", colleagueToken); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"url"`) {
		t.Fatalf("a colleague asking for the address: %d %s", rec.Code, rec.Body.String())
	}
	if rec := send(router, http.MethodGet, photoPath, "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", rec.Code)
	}
	// The photo route being open must not open the rest of /employees.
	if rec := send(router, http.MethodGet, "/api/v1/employees/"+advisor.ID.String(), "", colleagueToken); rec.Code != http.StatusForbidden {
		t.Fatalf("the employee record stays admin-only: %d", rec.Code)
	}
	if rec := send(router, http.MethodGet, "/api/v1/employees", "", colleagueToken); rec.Code != http.StatusForbidden {
		t.Fatalf("the employee list stays admin-only: %d", rec.Code)
	}
	if rec := send(router, http.MethodGet, "/api/v1/employees/"+colleague.ID.String()+"/photo", "", advisorToken); rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "PHOTO_NOT_FOUND") {
		t.Fatalf("an employee without a picture: %d %s", rec.Code, rec.Body.String())
	}
	if rec := send(router, http.MethodGet, "/api/v1/employees/"+uuid.NewString()+"/photo", "", advisorToken); rec.Code != http.StatusNotFound {
		t.Fatalf("an unknown employee: %d", rec.Code)
	}
	if rec := send(router, http.MethodGet, "/api/v1/employees/not-a-uuid/photo", "", advisorToken); rec.Code != http.StatusNotFound {
		t.Fatalf("a malformed id: %d", rec.Code)
	}

	// /me tells the screen whether to ask for the picture at all.
	if rec := send(router, http.MethodGet, "/api/v1/me", "", advisorToken); !strings.Contains(rec.Body.String(), `"hasPhoto":true`) {
		t.Fatalf("/me: %s", rec.Body.String())
	}
	if rec := send(router, http.MethodGet, "/api/v1/me", "", colleagueToken); !strings.Contains(rec.Body.String(), `"hasPhoto":false`) {
		t.Fatalf("/me without a picture: %s", rec.Body.String())
	}

	if rec := send(router, http.MethodDelete, "/api/v1/me/photo", "", advisorToken); rec.Code != http.StatusNoContent {
		t.Fatalf("remove: %d", rec.Code)
	}
	if rec := send(router, http.MethodGet, photoPath, "", colleagueToken); rec.Code != http.StatusNotFound {
		t.Fatalf("after removal: %d", rec.Code)
	}
}

func TestPhotoUploadRejectsBadRequests(t *testing.T) {
	e := newTestEnv(t)
	router := newTestRouter(e)
	emp, _ := e.employee(t, "sales_advisor")
	t.Cleanup(func() { e.cleanup(emp.ID) })
	token := e.tokenFor(t, emp)

	if rec := send(router, http.MethodPut, "/api/v1/me/photo", `{"file":"x"}`, token); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "INVALID_MULTIPART") {
		t.Fatalf("JSON instead of multipart: %d %s", rec.Code, rec.Body.String())
	}
	body, ct := multipartBody(t, "avatar", "yo.png", pngPhoto(t, 5))
	if rec := upload(router, token, body, ct); rec.Code != http.StatusBadRequest {
		t.Fatalf("a part with the wrong name: %d %s", rec.Code, rec.Body.String())
	}
	body, ct = multipartBody(t, "file", "yo.txt", []byte("not an image"))
	if rec := upload(router, token, body, ct); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "PHOTO_TYPE_NOT_ALLOWED") {
		t.Fatalf("not an image: %d %s", rec.Code, rec.Body.String())
	}
	body, ct = multipartBody(t, "file", "big.png", append(pngPhoto(t, 5), make([]byte, MaxPhotoBytes)...))
	if rec := upload(router, token, body, ct); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("a photo over 2 MB: %d %s", rec.Code, rec.Body.String())
	}
	body, ct = multipartBody(t, "file", "huge.png", make([]byte, MaxPhotoBytes+200<<10))
	if rec := upload(router, token, body, ct); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("a body far over the limit is cut off: %d", rec.Code)
	}
	if len(e.avatars.Keys()) != 0 {
		t.Fatalf("nothing may be stored: %v", e.avatars.Keys())
	}
}
