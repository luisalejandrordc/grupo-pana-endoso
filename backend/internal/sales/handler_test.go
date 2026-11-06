package sales_test

import (
	"bytes"
	"encoding/json"
	"endosoflow/internal/auth"
	"endosoflow/internal/sales"
	"endosoflow/internal/tasks"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (f *fx) router() http.Handler {
	authHandler := auth.NewHandler(f.Auth, f.Logger, auth.HandlerConfig{AppBaseURL: "http://app.test"})
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		authHandler.Mount(r)
		sales.NewHandler(f.sales, authHandler, f.Logger).Mount(r)
		tasks.NewHandler(f.tasks, authHandler, f.Logger).Mount(r)
	})
	return r
}

func call(h http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("not JSON: %s", rec.Body.String())
	}
	return m
}

func TestEverySalesRouteNeedsSignInAndAStaffRole(t *testing.T) {
	f := newFx(t)
	router := f.router()
	s := f.newSale(f.advisor)
	base := "/api/v1/sales/" + s.ID.String()
	routes := []struct{ method, path string }{
		{"GET", "/api/v1/sales"},
		{"POST", "/api/v1/sales"},
		{"GET", base},
		{"PATCH", base},
		{"DELETE", base + "?reason=x&confirm=x"},
		{"GET", base + "/history"},
		{"POST", base + "/cancel"},
		{"POST", base + "/reopen"},
		{"POST", base + "/reassign"},
		{"GET", base + "/tasks"},
		{"GET", base + "/tasks/1"},
		{"POST", base + "/tasks/1/start"},
		{"POST", base + "/tasks/1/complete"},
		{"POST", base + "/tasks/1/restart"},
		{"POST", base + "/tasks/1/reopen"},
		{"PATCH", base + "/tasks/1/data"},
		{"PATCH", base + "/tasks/1/notes"},
	}
	for _, r := range routes {
		if rec := call(router, r.method, r.path, "{}", ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without a token: %d", r.method, r.path, rec.Code)
		}
	}
}

func TestTheSalesAndTaskFlowOverHTTP(t *testing.T) {
	f := newFx(t)
	router := f.router()
	advisorEmp := f.Employee(auth.RoleSalesAdvisor, true)
	manager := f.Token(f.Employee(auth.RoleSalesManager, true))
	admin := f.Token(f.Employee(auth.RoleSystemAdmin, true))
	advisor := f.Token(advisorEmp)
	other := f.Token(f.Employee(auth.RoleSalesAdvisor, true))
	customer := f.Customer(advisorEmp.ID, false)

	// ---- create ----
	body := fmt.Sprintf(`{"customerId":"%s","versionId":"%s","colorId":"%s","temperature":"hot","notes":"Primera llamada"}`, customer, f.vehicle.VersionID, f.vehicle.ColorID)
	rec := call(router, "POST", "/api/v1/sales", body, advisor)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	sale := decode(t, rec)
	id := sale["id"].(string)
	base := "/api/v1/sales/" + id
	for _, key := range []string{
		"id", "code", "customer", "vehicle", "advisor", "temperature", "state", "notes", "quotedFinalPrice", "createdAt", "updatedAt",
		"completedAt", "cancellation", "currentTask", "milestones", "tasks", "permissions", "version",
	} {
		if _, ok := sale[key]; !ok {
			t.Errorf("the sale JSON lacks %q", key)
		}
	}
	if len(sale["tasks"].([]any)) != 13 || len(sale["milestones"].([]any)) != 6 {
		t.Fatal("13 tasks and 6 milestones expected")
	}
	if rec := call(router, "POST", "/api/v1/sales", `{"customerId":"`+customer.String()+`","bogus":1}`, advisor); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown field: %d", rec.Code)
	}
	if rec := call(router, "POST", "/api/v1/sales", strings.Replace(body, `"hot"`, `"tibia"`, 1), advisor); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"temperature"`) {
		t.Fatalf("bad temperature: %d %s", rec.Code, rec.Body.String())
	}

	// ---- scope ----
	if rec := call(router, "GET", base, "", other); rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "SALE_NOT_FOUND") {
		t.Fatalf("another advisor: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(router, "GET", base+"/tasks/1", "", other); rec.Code != http.StatusNotFound {
		t.Fatalf("another advisor's task: %d", rec.Code)
	}
	if rec := call(router, "GET", "/api/v1/sales/not-a-uuid", "", advisor); rec.Code != http.StatusNotFound {
		t.Fatalf("bad id: %d", rec.Code)
	}

	// ---- list query validation ----
	for _, q := range []string{"state=bogus", "customerType=x", "temperature=x", "currentTask=14", "currentTask=0", "milestone=7", "advisorId=nope", "modelId=nope", "page=0", "pageSize=101", "pageSize=abc"} {
		if rec := call(router, "GET", "/api/v1/sales?"+q, "", manager); rec.Code != http.StatusBadRequest {
			t.Errorf("?%s: %d, want 400", q, rec.Code)
		}
	}
	rec = call(router, "GET", "/api/v1/sales?pageSize=5&q="+url.QueryEscape(sale["code"].(string)), "", advisor)
	if rec.Code != http.StatusOK || decode(t, rec)["total"].(float64) != 1 {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}

	// ---- task 1 over HTTP ----
	if rec := call(router, "GET", base+"/tasks", "", advisor); rec.Code != http.StatusOK || len(decode(t, rec)["items"].([]any)) != 13 {
		t.Fatalf("task list: %d", rec.Code)
	}
	for _, n := range []string{"0", "14", "x"} {
		if rec := call(router, "GET", base+"/tasks/"+n, "", advisor); rec.Code != http.StatusNotFound {
			t.Errorf("task %s: %d", n, rec.Code)
		}
	}
	if rec := call(router, "POST", base+"/tasks/2/start", "", advisor); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "TASK_NOT_ELIGIBLE") {
		t.Fatalf("start task 2: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(router, "POST", base+"/tasks/1/start", "", advisor); rec.Code != http.StatusOK {
		t.Fatalf("start: %d %s", rec.Code, rec.Body.String())
	}
	task := decode(t, call(router, "GET", base+"/tasks/1", "", advisor))
	for _, key := range []string{"number", "name", "status", "data", "calculated", "documents", "warnings", "permissions", "version", "notes", "saleId"} {
		if _, ok := task[key]; !ok {
			t.Errorf("the task JSON lacks %q", key)
		}
	}
	version := int(task["version"].(float64))
	save := func(version int, data string) *httptest.ResponseRecorder {
		return call(router, "PATCH", base+"/tasks/1/data", fmt.Sprintf(`{"version":%d,"data":%s}`, version, data), advisor)
	}
	if rec := call(router, "PATCH", base+"/tasks/1/data", `{"data":{}}`, advisor); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "VERSION_REQUIRED") {
		t.Fatalf("no version: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(router, "PATCH", base+"/tasks/1/data", `{"version":1}`, advisor); rec.Code != http.StatusBadRequest {
		t.Fatalf("no data: %d", rec.Code)
	}
	if rec := save(version, `{"vin":"x"}`); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "INVALID_JSON") {
		t.Fatalf("a field of another task: %d %s", rec.Code, rec.Body.String())
	}
	if rec := save(version, `{"initialToyotaAmount":"-1"}`); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"initialToyotaAmount"`) {
		t.Fatalf("bad amount: %d %s", rec.Code, rec.Body.String())
	}
	rec = save(version, `{"initialToyotaAmount":"30000","initialCommission":"2500.5","accessoryAmount":"1200.50","releaseProcessingAmount":"800"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("save: %d %s", rec.Code, rec.Body.String())
	}
	saved := decode(t, rec)
	if saved["calculated"].(map[string]any)["quotedFinalPrice"] != "34501.00" {
		t.Fatalf("calculated = %v", saved["calculated"])
	}
	if rec := save(version, `{"accessoryAmount":"1"}`); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "VERSION_CONFLICT") {
		t.Fatalf("stale version: %d", rec.Code)
	}
	if rec := call(router, "POST", base+"/tasks/1/complete", "", advisor); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "documents.quotation_pdf") {
		t.Fatalf("complete without documents: %d %s", rec.Code, rec.Body.String())
	}
	saleID := uuid.MustParse(id)
	f.FillRequiredDocuments(saleID, 1, advisorEmp.ID)
	if rec := call(router, "POST", base+"/tasks/1/complete", "", advisor); rec.Code != http.StatusOK || decode(t, rec)["status"] != "completed" {
		t.Fatalf("complete: %d %s", rec.Code, rec.Body.String())
	}

	// ---- restart and reopen need a reason and the right role ----
	if rec := call(router, "POST", base+"/tasks/1/reopen", `{"reason":"x"}`, advisor); rec.Code != http.StatusForbidden {
		t.Fatalf("an advisor cannot reopen: %d", rec.Code)
	}
	if rec := call(router, "POST", base+"/tasks/1/reopen", `{}`, manager); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "REASON_REQUIRED") {
		t.Fatalf("reopen without a reason: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(router, "POST", base+"/tasks/1/reopen", `{"reason":"Corregir cotización"}`, manager); rec.Code != http.StatusOK || decode(t, rec)["status"] != "in_progress" {
		t.Fatalf("reopen: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(router, "POST", base+"/tasks/1/restart", `{"reason":"Reiniciar"}`, advisor); rec.Code != http.StatusForbidden {
		t.Fatalf("an advisor cannot restart: %d", rec.Code)
	}
	if rec := call(router, "POST", base+"/tasks/1/restart", `{"reason":"Reiniciar"}`, manager); rec.Code != http.StatusOK || decode(t, rec)["status"] != "pending" {
		t.Fatalf("restart: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(router, "POST", base+"/tasks/1/start", "", advisor); rec.Code != http.StatusOK {
		t.Fatalf("start again: %d", rec.Code)
	}

	// ---- task notes and sale notes ----
	v := int(decode(t, call(router, "GET", base+"/tasks/1", "", advisor))["version"].(float64))
	if rec := call(router, "PATCH", base+"/tasks/1/notes", fmt.Sprintf(`{"version":%d,"notes":"Cliente confirmó"}`, v), advisor); rec.Code != http.StatusOK || decode(t, rec)["notes"] != "Cliente confirmó" {
		t.Fatalf("task notes: %d %s", rec.Code, rec.Body.String())
	}
	sv := int(decode(t, call(router, "GET", base, "", advisor))["version"].(float64))
	if rec := call(router, "PATCH", base, fmt.Sprintf(`{"version":%d,"notes":"Nota de la venta"}`, sv), advisor); rec.Code != http.StatusOK || decode(t, rec)["notes"] != "Nota de la venta" {
		t.Fatalf("sale notes: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(router, "PATCH", base, `{"notes":"x"}`, advisor); rec.Code != http.StatusBadRequest {
		t.Fatalf("sale notes without a version: %d", rec.Code)
	}

	// ---- reassign, history ----
	if rec := call(router, "POST", base+"/reassign", `{}`, manager); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("reassign without an advisor: %d", rec.Code)
	}
	if rec := call(router, "GET", base+"/history?page=1&pageSize=5", "", advisor); rec.Code != http.StatusOK || len(decode(t, rec)["items"].([]any)) != 5 {
		t.Fatalf("history: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(router, "GET", base+"/history?pageSize=0", "", advisor); rec.Code != http.StatusBadRequest {
		t.Fatalf("history page size: %d", rec.Code)
	}

	// ---- cancel, reopen, delete ----
	if rec := call(router, "POST", base+"/cancel", `{"category":"other"}`, advisor); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("cancel without a reason: %d", rec.Code)
	}
	if rec := call(router, "POST", base+"/cancel", `{"category":"customer_withdrew","reason":"Desistió"}`, advisor); rec.Code != http.StatusOK || decode(t, rec)["state"] != "cancelled" {
		t.Fatalf("cancel: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(router, "POST", base+"/reopen", `{"reason":"x"}`, advisor); rec.Code != http.StatusForbidden {
		t.Fatalf("an advisor cannot reopen a sale: %d", rec.Code)
	}
	if rec := call(router, "POST", base+"/reopen", `{"reason":"Volvió"}`, manager); rec.Code != http.StatusOK || decode(t, rec)["state"] != "active" {
		t.Fatalf("reopen sale: %d %s", rec.Code, rec.Body.String())
	}
	code := sale["code"].(string)
	if rec := call(router, "DELETE", base+"?reason=Prueba&confirm="+code, "", manager); rec.Code != http.StatusForbidden {
		t.Fatalf("a manager cannot delete: %d", rec.Code)
	}
	if rec := call(router, "DELETE", base+"?reason=Prueba&confirm=EF-0", "", admin); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("wrong confirmation: %d", rec.Code)
	}
	if rec := call(router, "DELETE", base+"?reason=Prueba&confirm="+code, "", admin); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(router, "GET", base, "", admin); rec.Code != http.StatusNotFound {
		t.Fatalf("deleted sale: %d", rec.Code)
	}
}
