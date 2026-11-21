package staff

import (
	"encoding/json"
	"endosoflow/internal/auth"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func newTestRouter(e *testEnv) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	authHandler := auth.NewHandler(e.auth, logger, auth.HandlerConfig{AppBaseURL: "http://app.test"})
	staffHandler := NewHandler(e.staff, authHandler, logger)

	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		authHandler.Mount(r)
		staffHandler.Mount(r)
	})
	return r
}

func send(router http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func (e *testEnv) tokenFor(t *testing.T, emp testEmployee) string {
	t.Helper()
	session, err := e.auth.Login(e.ctx, auth.LoginRequest{
		DocumentType: "DNI", DocumentNumber: emp.DocNumber,
		Password: goodPassword,
	}, auth.ClientInfo{})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	return session.AccessToken
}

func TestEmployeeRoutesRequireAnAdmin(t *testing.T) {
	e := newTestEnv(t)
	router := newTestRouter(e)
	admin := e.tokenFor(t, e.insertEmployee(t, "system_admin", goodPassword, "Admin"))
	manager := e.tokenFor(t, e.insertEmployee(t, "sales_manager", goodPassword, "Gerente"))
	advisor := e.tokenFor(t, e.insertEmployee(t, "sales_advisor", goodPassword, "Asesor"))

	if rec := send(router, http.MethodGet, "/api/v1/employees", "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token: %d", rec.Code)
	}
	for name, token := range map[string]string{"advisor": advisor, "manager": manager} {
		if rec := send(router, http.MethodGet, "/api/v1/employees", "", token); rec.Code != http.StatusForbidden {
			t.Fatalf("%s: %d", name, rec.Code)
		}
	}
	rec := send(router, http.MethodGet, "/api/v1/employees?pageSize=5", "", admin)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"items"`) {
		t.Fatalf("admin: %d %s", rec.Code, rec.Body.String())
	}
}

func TestAdvisorPickerAllowsManagersButNotAdvisors(t *testing.T) {
	e := newTestEnv(t)
	router := newTestRouter(e)
	manager := e.tokenFor(t, e.insertEmployee(t, "sales_manager", goodPassword, "Gerente"))
	advisor := e.tokenFor(t, e.insertEmployee(t, "sales_advisor", goodPassword, "Asesor"))

	if rec := send(router, http.MethodGet, "/api/v1/advisors", "", manager); rec.Code != http.StatusOK {
		t.Fatalf("manager: %d %s", rec.Code, rec.Body.String())
	}
	if rec := send(router, http.MethodGet, "/api/v1/advisors", "", advisor); rec.Code != http.StatusForbidden {
		t.Fatalf("advisor: %d", rec.Code)
	}
}

func TestEmployeeLifecycleOverHTTP(t *testing.T) {
	e := newTestEnv(t)
	router := newTestRouter(e)
	admin := e.tokenFor(t, e.insertEmployee(t, "system_admin", goodPassword, "Admin"))

	body, _ := json.Marshal(validInput())
	rec := send(router, http.MethodPost, "/api/v1/employees", string(body), admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var created CreateResult
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.Employee.ID.String() == "" || !created.ActivationLinkIssued {
		t.Fatalf("unexpected create body: %s (%v)", rec.Body.String(), err)
	}
	id := created.Employee.ID
	t.Cleanup(func() { e.cleanup(id) })
	e.dispatch(t)

	path := "/api/v1/employees/" + id.String()
	if rec := send(router, http.MethodGet, path, "", admin); rec.Code != http.StatusOK {
		t.Fatalf("get: %d", rec.Code)
	}
	if rec := send(router, http.MethodGet, "/api/v1/employees/not-a-uuid", "", admin); rec.Code != http.StatusNotFound {
		t.Fatalf("bad id: %d", rec.Code)
	}

	patch := `{"version":1,"firstName":"Cambiada"}`
	if rec := send(router, http.MethodPatch, path, patch, admin); rec.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body.String())
	}
	rec = send(router, http.MethodPatch, path, patch, admin) // the same stale version again
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "VERSION_CONFLICT") {
		t.Fatalf("stale patch: %d %s", rec.Code, rec.Body.String())
	}
	if rec := send(router, http.MethodPatch, path, `{"version":2,"unknownField":1}`, admin); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown field: %d", rec.Code)
	}

	if rec := send(router, http.MethodPost, path+"/deactivate", "", admin); rec.Code != http.StatusOK {
		t.Fatalf("deactivate: %d %s", rec.Code, rec.Body.String())
	}
	if rec := send(router, http.MethodPost, path+"/reactivate", "", admin); rec.Code != http.StatusOK {
		t.Fatalf("reactivate: %d %s", rec.Code, rec.Body.String())
	}
	if rec := send(router, http.MethodPost, path+"/resend-activation", "", admin); rec.Code != http.StatusAccepted {
		t.Fatalf("resend: %d %s", rec.Code, rec.Body.String())
	}
	e.dispatch(t)

	if rec := send(router, http.MethodDelete, path+"?reason=", "", admin); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("delete without a reason: %d", rec.Code)
	}
	if rec := send(router, http.MethodDelete, path+"?reason=Creado%20por%20error", "", admin); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
}
