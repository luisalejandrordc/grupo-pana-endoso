package customers

import (
	"encoding/json"
	"endosoflow/internal/auth"
	"endosoflow/internal/platform/ids"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func (e *testEnv) router() http.Handler {
	authHandler := auth.NewHandler(e.auth, e.logger, auth.HandlerConfig{AppBaseURL: "http://app.test"})
	handler := NewHandler(e.svc, authHandler, e.logger)
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		authHandler.Mount(r)
		handler.Mount(r)
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

func (e *testEnv) tokenFor(emp testEmployee) string {
	e.t.Helper()
	session, err := e.auth.Login(e.ctx, auth.LoginRequest{
		DocumentType: "DNI", DocumentNumber: emp.DocNumber, Password: goodPassword,
	}, auth.ClientInfo{})
	if err != nil {
		e.t.Fatalf("login: %v", err)
	}
	return session.AccessToken
}

func TestCustomerRoutesRequireSignInAndTheRightRole(t *testing.T) {
	e := newTestEnv(t)
	router := e.router()
	admin := e.tokenFor(e.insertEmployee(auth.RoleSystemAdmin, goodPassword))
	manager := e.tokenFor(e.insertEmployee(auth.RoleSalesManager, goodPassword))
	advisorEmp := e.insertEmployee(auth.RoleSalesAdvisor, goodPassword)
	advisor := e.tokenFor(advisorEmp)
	c := e.createCustomer(advisorEmp.actor(), person())
	path := "/api/v1/customers/" + c.ID.String()

	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/customers"},
		{http.MethodPost, "/api/v1/customers"},
		{http.MethodGet, "/api/v1/customers/lookup?documentType=DNI&documentNumber=12345678"},
		{http.MethodGet, path},
		{http.MethodPatch, path},
		{http.MethodDelete, path + "?reason=x"},
		{http.MethodPost, path + "/deactivate"},
		{http.MethodPost, path + "/reactivate"},
		{http.MethodGet, "/api/v1/lookups/identity?documentType=DNI&documentNumber=12345678"},
		{http.MethodGet, "/api/v1/lookups/identity/quota"},
	} {
		if rec := send(router, route.method, route.path, "{}", ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without a token: %d", route.method, route.path, rec.Code)
		}
	}

	// Advisors cannot deactivate, reactivate, delete or read the quota; managers cannot delete.
	forbidden := []struct{ who, token, method, path string }{
		{"advisor", advisor, http.MethodPost, path + "/deactivate"},
		{"advisor", advisor, http.MethodPost, path + "/reactivate"},
		{"advisor", advisor, http.MethodDelete, path + "?reason=x"},
		{"advisor", advisor, http.MethodGet, "/api/v1/lookups/identity/quota"},
		{"manager", manager, http.MethodDelete, path + "?reason=x"},
		{"manager", manager, http.MethodGet, "/api/v1/lookups/identity/quota"},
	}
	for _, f := range forbidden {
		if rec := send(router, f.method, f.path, "", f.token); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s %s: %d, want 403", f.who, f.method, f.path, rec.Code)
		}
	}

	if rec := send(router, http.MethodGet, "/api/v1/lookups/identity/quota", "", admin); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"limit"`) {
		t.Fatalf("admin quota: %d %s", rec.Code, rec.Body.String())
	}
	if rec := send(router, http.MethodGet, "/api/v1/customers?pageSize=5", "", advisor); rec.Code != http.StatusOK {
		t.Fatalf("advisor list: %d %s", rec.Code, rec.Body.String())
	}
}

func TestCustomerLifecycleOverHTTP(t *testing.T) {
	e := newTestEnv(t)
	router := e.router()
	admin := e.tokenFor(e.insertEmployee(auth.RoleSystemAdmin, goodPassword))
	manager := e.tokenFor(e.insertEmployee(auth.RoleSalesManager, goodPassword))

	body, _ := json.Marshal(person())
	rec := send(router, http.MethodPost, "/api/v1/customers", string(body), manager)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var created Customer
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.Version != 1 {
		t.Fatalf("create body: %s (%v)", rec.Body.String(), err)
	}
	e.trackCustomer(created.ID)
	path := "/api/v1/customers/" + created.ID.String()

	if rec := send(router, http.MethodPost, "/api/v1/customers", string(body), manager); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "CUSTOMER_ALREADY_EXISTS") {
		t.Fatalf("duplicate: %d %s", rec.Code, rec.Body.String())
	}
	if rec := send(router, http.MethodPost, "/api/v1/customers", `{"customerType":"diplomatico"}`, manager); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"fields"`) {
		t.Fatalf("invalid: %d %s", rec.Code, rec.Body.String())
	}
	if rec := send(router, http.MethodGet, path, "", manager); rec.Code != http.StatusOK {
		t.Fatalf("get: %d", rec.Code)
	}
	if rec := send(router, http.MethodGet, "/api/v1/customers/not-a-uuid", "", manager); rec.Code != http.StatusNotFound {
		t.Fatalf("bad id: %d", rec.Code)
	}
	if rec := send(router, http.MethodGet, "/api/v1/customers?status=bogus", "", manager); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad status: %d", rec.Code)
	}
	if rec := send(router, http.MethodGet, "/api/v1/customers?customerType=bogus", "", manager); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad type: %d", rec.Code)
	}

	patch := `{"version":1,"phone":"999000111"}`
	if rec := send(router, http.MethodPatch, path, patch, manager); rec.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body.String())
	}
	if rec := send(router, http.MethodPatch, path, patch, manager); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "VERSION_CONFLICT") {
		t.Fatalf("stale patch: %d %s", rec.Code, rec.Body.String())
	}
	if rec := send(router, http.MethodPatch, path, `{"version":2,"unknownField":1}`, manager); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown field: %d", rec.Code)
	}

	if rec := send(router, http.MethodPost, path+"/deactivate", "", manager); rec.Code != http.StatusOK {
		t.Fatalf("deactivate: %d %s", rec.Code, rec.Body.String())
	}
	if rec := send(router, http.MethodPost, path+"/reactivate", "", manager); rec.Code != http.StatusOK {
		t.Fatalf("reactivate: %d %s", rec.Code, rec.Body.String())
	}
	if rec := send(router, http.MethodDelete, path+"?reason=", "", admin); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("delete without a reason: %d", rec.Code)
	}
	if rec := send(router, http.MethodDelete, path+"?reason=Creado%20por%20error", "", admin); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
}

func TestLookupRoutesOverHTTP(t *testing.T) {
	e := newTestEnv(t)
	router := e.router()
	manager := e.tokenFor(e.insertEmployee(auth.RoleSalesManager, goodPassword))
	c := e.createCustomer(e.insertEmployee(auth.RoleSalesManager, "").actor(), person())

	rec := send(router, http.MethodGet, "/api/v1/customers/lookup?documentType=DNI&documentNumber="+c.DocumentNumber, "", manager)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"access":"full"`) {
		t.Fatalf("lookup: %d %s", rec.Code, rec.Body.String())
	}
	if rec := send(router, http.MethodGet, "/api/v1/customers/lookup?documentType=DNI&documentNumber=1", "", manager); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad document: %d", rec.Code)
	}
	rec = send(router, http.MethodGet, "/api/v1/lookups/identity?documentType=DNI&documentNumber=7"+randomDigits(7), "", manager)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"source":"provider"`) {
		t.Fatalf("identity: %d %s", rec.Code, rec.Body.String())
	}
	if rec := send(router, http.MethodGet, "/api/v1/lookups/identity?documentType=CE&documentNumber=AB1234567", "", manager); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "LOOKUP_NOT_SUPPORTED") {
		t.Fatalf("unsupported type: %d %s", rec.Code, rec.Body.String())
	}
}

func TestResendActivationRouteOverHTTP(t *testing.T) {
	e := newTestEnv(t)
	router := e.router()
	manager := e.tokenFor(e.insertEmployee(auth.RoleSalesManager, goodPassword))
	advisor := e.tokenFor(e.insertEmployee(auth.RoleSalesAdvisor, goodPassword))
	c := e.createCustomer(e.insertEmployee(auth.RoleSalesManager, "").actor(), person())
	path := "/api/v1/customers/" + c.ID.String() + "/resend-activation"

	if rec := send(router, http.MethodPost, path, "", advisor); rec.Code != http.StatusForbidden {
		t.Fatalf("an advisor must not resend: %d", rec.Code)
	}
	if rec := send(router, http.MethodPost, path, "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", rec.Code)
	}
	before := e.queue.count()
	if rec := send(router, http.MethodPost, path, "", manager); rec.Code != http.StatusAccepted {
		t.Fatalf("a manager resends: %d %s", rec.Code, rec.Body.String())
	}
	if e.queue.count() != before+1 {
		t.Fatal("the resend must queue an email")
	}
	if rec := send(router, http.MethodPost, "/api/v1/customers/"+ids.NewString()+"/resend-activation", "", manager); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown customer: %d", rec.Code)
	}
}
