package sales_test

import (
	"bytes"
	"encoding/json"
	"endosoflow/internal/auth"
	"endosoflow/internal/catalog"
	"endosoflow/internal/portal"
	"endosoflow/internal/sales"
	"endosoflow/internal/tasks"
	"endosoflow/internal/testkit"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// The customer portal, end to end over real sales: who can call it, whose data it returns, and above all what a
// customer can and cannot read. The visibility rules come from the specification: only fields and documents marked
// customer-visible, only from completed tasks, never anything internal.

type portalFx struct {
	*fx
	router http.Handler
}

func newPortalFx(t *testing.T) *portalFx {
	t.Helper()
	f := newFx(t)
	catalogService := catalog.NewService(catalog.Deps{DB: f.Pool, Store: f.store, Logger: f.Logger})
	authHandler := auth.NewHandler(f.Auth, f.Logger, auth.HandlerConfig{AppBaseURL: "http://app.test", PortalBaseURL: "http://portal.test"})
	portalService := portal.NewService(portal.Deps{DB: f.Pool, Vehicles: catalogService, Documents: f.docs, Logger: f.Logger})
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		authHandler.Mount(r)
		portal.NewHandler(portalService, authHandler, f.Logger).Mount(r)
		sales.NewHandler(f.sales, authHandler, f.Logger).Mount(r)
		tasks.NewHandler(f.tasks, authHandler, f.Logger).Mount(r)
	})
	return &portalFx{fx: f, router: r}
}

// customerOf is the customer of a sale.
func (f *portalFx) customerOf(sale uuid.UUID) uuid.UUID {
	var id uuid.UUID
	if err := f.Pool.QueryRow(f.Ctx, `SELECT customer_id FROM sale_registers WHERE id = $1`, sale).Scan(&id); err != nil {
		f.T.Fatal(err)
	}
	return id
}

// signIn gives the customer a password and signs them in, returning their access token.
func (f *portalFx) signIn(customer uuid.UUID) string {
	f.T.Helper()
	hash, err := auth.HashPassword(testkit.Password)
	if err != nil {
		f.T.Fatal(err)
	}
	var docType, docNumber string
	if err := f.Pool.QueryRow(f.Ctx, `UPDATE customers SET password_hash = $2, activated_at = COALESCE(activated_at, now())
		WHERE id = $1 RETURNING document_type::text, document_number`, customer, hash).Scan(&docType, &docNumber); err != nil {
		f.T.Fatal(err)
	}
	session, err := f.Auth.CustomerLogin(f.Ctx, auth.LoginRequest{DocumentType: docType, DocumentNumber: docNumber, Password: testkit.Password}, auth.ClientInfo{})
	if err != nil {
		f.T.Fatalf("customer login (%s %q): %v", docType, docNumber, err)
	}
	return session.AccessToken
}

func (f *portalFx) get(path, token string) *httptest.ResponseRecorder {
	return call(f.router, "GET", "/api/v1"+path, "", token)
}

// body is the response with the request id removed, so two answers can be compared.
func body(rec *httptest.ResponseRecorder) string {
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		return rec.Body.String()
	}
	if e, ok := m["error"].(map[string]any); ok {
		delete(e, "requestId")
	}
	out, _ := json.Marshal(m)
	return string(out)
}

func (f *portalFx) detail(token string, sale uuid.UUID) (map[string]any, string) {
	f.T.Helper()
	rec := f.get("/portal/sales/"+sale.String(), token)
	if rec.Code != http.StatusOK {
		f.T.Fatalf("sale detail: %d %s", rec.Code, rec.Body.String())
	}
	return decode(f.T, rec), rec.Body.String()
}

// sectionsOf indexes the sections of a detail by task number.
func sectionsOf(detail map[string]any) map[int]map[string]any {
	out := map[int]map[string]any{}
	for _, raw := range detail["sections"].([]any) {
		s := raw.(map[string]any)
		out[int(s["taskNumber"].(float64))] = s
	}
	return out
}

func fieldKeys(section map[string]any) map[string]string {
	out := map[string]string{}
	for _, raw := range section["fields"].([]any) {
		fl := raw.(map[string]any)
		out[fl["key"].(string)] = fl["value"].(string)
	}
	return out
}

func documentLabels(section map[string]any) []string {
	var out []string
	for _, raw := range section["documents"].([]any) {
		out = append(out, raw.(map[string]any)["label"].(string))
	}
	return out
}

func milestoneStates(detail map[string]any) []string {
	var out []string
	for _, raw := range detail["milestones"].([]any) {
		out = append(out, raw.(map[string]any)["state"].(string))
	}
	return out
}

// ---- who can call it ------------------------------------------------------------------------

func TestEveryPortalRouteNeedsACustomerAndNothingElseWillDo(t *testing.T) {
	f := newPortalFx(t)
	s := f.newSale(f.advisor)
	customerToken := f.signIn(f.customerOf(s.ID))
	employeeToken := f.Token(f.Employee(auth.RoleSystemAdmin, true)) // the fixture's own admin has no password
	someDocument := uuid.NewString()

	routes := []struct{ method, path string }{
		{"GET", "/api/v1/portal/me"},
		{"PUT", "/api/v1/portal/me/password"},
		{"GET", "/api/v1/portal/sales"},
		{"GET", "/api/v1/portal/sales/" + s.ID.String()},
		{"GET", "/api/v1/portal/documents/" + someDocument + "/download-url"},
	}
	for _, r := range routes {
		for name, token := range map[string]string{"no token": "", "an employee's token": employeeToken, "garbage": "not.a.token"} {
			rec := call(f.router, r.method, r.path, "{}", token)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("%s %s with %s: status %d, want 401", r.method, r.path, name, rec.Code)
			}
		}
		if rec := call(f.router, r.method, r.path, "{}", customerToken); rec.Code == http.StatusUnauthorized {
			t.Errorf("%s %s must accept a customer's token", r.method, r.path)
		}
	}

	// And a customer's token opens nothing of the employees' application.
	for _, path := range []string{"/api/v1/me", "/api/v1/sales", "/api/v1/sales/" + s.ID.String(), "/api/v1/sales/" + s.ID.String() + "/tasks/1"} {
		if rec := call(f.router, "GET", path, "", customerToken); rec.Code != http.StatusUnauthorized {
			t.Errorf("GET %s with a customer's token: status %d, want 401", path, rec.Code)
		}
	}
}

func TestPortalAnswersAreNeverCached(t *testing.T) {
	f := newPortalFx(t)
	s := f.newSale(f.advisor)
	rec := f.get("/portal/sales", f.signIn(f.customerOf(s.ID)))
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
}

// ---- whose data ----------------------------------------------------------------------------

func TestACustomerReachesOnlyTheirOwnPurchases(t *testing.T) {
	f := newPortalFx(t)
	mine, theirs := f.newSale(f.advisor), f.newSale(f.advisor)
	f.advance(theirs.ID, f.advisor, 3) // theirs has completed tasks and documents
	token := f.signIn(f.customerOf(mine.ID))

	list := decode(f.T, f.get("/portal/sales", token))["items"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["id"] != mine.ID.String() {
		t.Fatalf("the list must hold only the customer's own purchase: %v", list)
	}

	// Someone else's purchase answers exactly like one that does not exist.
	other := f.get("/portal/sales/"+theirs.ID.String(), token)
	missing := f.get("/portal/sales/"+uuid.NewString(), token)
	malformed := f.get("/portal/sales/not-a-uuid", token)
	for name, rec := range map[string]*httptest.ResponseRecorder{"someone else's": other, "unknown": missing, "malformed": malformed} {
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", name, rec.Code)
		}
		if body(rec) != body(other) {
			t.Errorf("%s answers differently from someone else's purchase:\n%s\n%s", name, body(rec), body(other))
		}
	}

	// The same for their files.
	var theirFile uuid.UUID
	if err := f.Pool.QueryRow(f.Ctx, `SELECT d.id FROM documents d JOIN document_slots s ON s.slot_code = d.slot_code
		WHERE d.sale_id = $1 AND s.is_customer_visible LIMIT 1`, theirs.ID).Scan(&theirFile); err != nil {
		t.Fatalf("the other sale must have a visible file: %v", err)
	}
	stolen := f.get("/portal/documents/"+theirFile.String()+"/download-url", token)
	absent := f.get("/portal/documents/"+uuid.NewString()+"/download-url", token)
	if stolen.Code != http.StatusNotFound || body(stolen) != body(absent) {
		t.Fatalf("someone else's file must answer like a missing one: %d %s vs %d %s", stolen.Code, body(stolen), absent.Code, body(absent))
	}
	// Their own owner can get it.
	if rec := f.get("/portal/documents/"+theirFile.String()+"/download-url", f.signIn(f.customerOf(theirs.ID))); rec.Code != http.StatusOK {
		t.Fatalf("the owner must get their file: %d %s", rec.Code, rec.Body.String())
	}
}

// ---- what they see -------------------------------------------------------------------------

func TestACustomerSeesTheirPurchaseWithItsVehicleAdvisorAndMilestones(t *testing.T) {
	f := newPortalFx(t)
	s := f.newSale(f.advisor)
	detail, _ := f.detail(f.signIn(f.customerOf(s.ID)), s.ID)

	if detail["code"] != s.Code || detail["state"] != "active" {
		t.Fatalf("code and state: %v %v", detail["code"], detail["state"])
	}
	vehicle := detail["vehicle"].(map[string]any)
	if vehicle["model"] == "" || vehicle["version"] == "" || vehicle["color"] == "" || vehicle["referencePrice"] == "" {
		t.Fatalf("the vehicle is described: %v", vehicle)
	}
	advisor := detail["advisor"].(map[string]any)
	if advisor["name"] != f.nameOfEmployee(f.advisor.ID) || advisor["email"] != f.emailOfEmployee(f.advisor.ID) {
		t.Fatalf("the advisor's business contact: %v", advisor)
	}
	if states := milestoneStates(detail); len(states) != 6 || states[0] != "in_progress" || states[5] != "pending" {
		t.Fatalf("six milestones, the first one under way: %v", states)
	}
	if len(detail["sections"].([]any)) != 0 {
		t.Fatal("nothing is completed yet, so there is nothing to read")
	}
	list := decode(f.T, f.get("/portal/sales", f.signIn(f.customerOf(s.ID))))["items"].([]any)
	if card := list[0].(map[string]any); card["code"] != s.Code || len(card["milestones"].([]any)) != 6 {
		t.Fatalf("the card: %v", card)
	}
}

// internalMarkers are values that exist in the sale but must never reach a customer: each one is recorded in a field
// or a document that the specification keeps for the staff.
var internalMarkers = []string{
	"no_pep", "general", // task 2: the PEP and DDC classification
	"Juan Pérez",                   // task 9: the person in charge
	"callao_maritima", "118-2026-", // task 8: customs
	"OS-",                      // task 12: the service order number
	"bbva",                     // task 5: Toyota's payment (task 4 is the customer's own, "bcp")
	"(Excel)",                  // task 1: the Excel quotation
	"liquidación de la unidad", // task 3: the settlement document
	"Confirmación de pago a Toyota", "Voucher de pago a Toyota",
	"Documentos endosados", "Certificado aduanero", "Pedido de Depósito", "Orden de servicio",
	"signerNameSource", "mismatchReason", "pepStatus", "ddcRegime",
}

func TestACustomerSeesOnlyWhatTheSpecificationMarksVisibleOfTheCompletedTasks(t *testing.T) {
	f := newPortalFx(t)
	s := f.newSale(f.advisor)
	token := f.signIn(f.customerOf(s.ID))

	f.advance(s.ID, f.advisor, 3)
	detail, raw := f.detail(token, s.ID)
	sections := sectionsOf(detail)
	if len(sections) != 3 {
		t.Fatalf("three completed tasks, three sections: %v", len(sections))
	}
	// Task 1: the quotation amounts, the calculated price and only the PDF.
	one := fieldKeys(sections[1])
	for _, key := range []string{"initialToyotaAmount", "initialCommission", "accessoryAmount", "releaseProcessingAmount", "quotedFinalPrice"} {
		if one[key] == "" {
			t.Errorf("task 1 must show %s: %v", key, one)
		}
	}
	if labels := documentLabels(sections[1]); len(labels) != 1 || labels[0] != "Cotización oficial (PDF)" {
		t.Errorf("task 1 offers only the PDF quotation, got %v", labels)
	}
	// Task 2: the signer and its three documents, but not the classification.
	two := fieldKeys(sections[2])
	if two["signerFirstName"] != "Ana" || two["signerLastName"] != "Rojas" || len(two) != 4 {
		t.Errorf("task 2 shows the signer only: %v", two)
	}
	if len(documentLabels(sections[2])) != 3 {
		t.Errorf("task 2 offers its three documents: %v", documentLabels(sections[2]))
	}
	// Task 3: the series, the VIN, the amounts, and no settlement document.
	three := fieldKeys(sections[3])
	for _, key := range []string{"vehicleSeries", "vin", "finalToyotaAmount", "finalCommission"} {
		if three[key] == "" {
			t.Errorf("task 3 must show %s: %v", key, three)
		}
	}
	if len(documentLabels(sections[3])) != 0 {
		t.Errorf("the settlement document is internal: %v", documentLabels(sections[3]))
	}
	for _, marker := range internalMarkers {
		if strings.Contains(raw, marker) {
			t.Errorf("an internal value reached the customer: %q", marker)
		}
	}

	// A whole purchase: tasks 5, 7, 8 and 9 have nothing for the customer and never appear.
	f.advance2(s.ID, 13)
	detail, raw = f.detail(token, s.ID)
	sections = sectionsOf(detail)
	for _, n := range []int{1, 2, 3, 4, 6, 10, 11, 12, 13} {
		if _, ok := sections[n]; !ok {
			t.Errorf("task %d has something for the customer and must appear", n)
		}
	}
	for _, n := range []int{5, 7, 8, 9} {
		if _, ok := sections[n]; ok {
			t.Errorf("task %d is internal and must not appear", n)
		}
	}
	if f4 := fieldKeys(sections[4]); f4["bank"] != "bcp" || f4["transactionNumber"] == "" || f4["depositedAmount"] == "" {
		t.Errorf("task 4 (the customer's own payment): %v", f4)
	}
	if f6 := fieldKeys(sections[6]); len(f6) != 12 || f6["toyota.number"] == "" {
		t.Errorf("task 6 shows the four invoices: %v", f6)
	}
	if fieldKeys(sections[10])["licensePlate"] == "" || fieldKeys(sections[13])["deliveryDate"] == "" {
		t.Error("the plate and the delivery date are shown")
	}
	if labels := documentLabels(sections[10]); len(labels) != 1 || !strings.Contains(labels[0], "TIVe") {
		t.Errorf("task 10 offers the TIVe: %v", labels)
	}
	if labels := documentLabels(sections[13]); len(labels) != 1 || labels[0] != "Acta de entrega firmada" {
		t.Errorf("task 13 offers the signed delivery record: %v", labels)
	}
	if len(documentLabels(sections[6])) != 4 {
		t.Errorf("task 6 offers its four invoices: %v", documentLabels(sections[6]))
	}
	for _, marker := range internalMarkers {
		if strings.Contains(raw, marker) {
			t.Errorf("an internal value reached the customer after the whole purchase: %q", marker)
		}
	}
	if states := milestoneStates(detail); strings.Join(states, ",") != strings.Repeat("completed,", 5)+"completed" || detail["state"] != "completed" {
		t.Errorf("a delivered purchase has six completed milestones: %v (state %v)", states, detail["state"])
	}
}

// advance2 completes the tasks after the ones already completed, up to n (advance always starts from task 1).
func (f *portalFx) advance2(sale uuid.UUID, n int) {
	f.T.Helper()
	var done int
	if err := f.Pool.QueryRow(f.Ctx, `SELECT count(*) FROM task_instances WHERE sale_id = $1 AND status = 'completed'`, sale).Scan(&done); err != nil {
		f.T.Fatal(err)
	}
	for i := done + 1; i <= n; i++ {
		if _, err := f.tasks.Start(f.Ctx, f.advisor.Actor(), sale, i); err != nil {
			f.T.Fatalf("start %d: %v", i, err)
		}
		if i != 7 {
			v := f.taskVersion(f.advisor, sale, i)
			if _, err := f.tasks.SaveData(f.Ctx, f.advisor.Actor(), sale, i, tasks.SaveInput{Version: &v, Data: []byte(f.validData(i))}); err != nil {
				f.T.Fatalf("save %d: %v", i, err)
			}
		}
		f.FillRequiredDocuments(sale, i, f.advisor.ID)
		if _, err := f.tasks.Complete(f.Ctx, f.advisor.Actor(), sale, i); err != nil {
			f.T.Fatalf("complete %d: %v", i, err)
		}
	}
}

func TestAReopenedTaskDisappearsFromThePortalAndItsMilestoneMovesBack(t *testing.T) {
	f := newPortalFx(t)
	s := f.newSale(f.advisor)
	token := f.signIn(f.customerOf(s.ID))
	f.advance(s.ID, f.advisor, 3)

	detail, raw := f.detail(token, s.ID)
	vin := fieldKeys(sectionsOf(detail)[3])["vin"]
	if vin == "" || !strings.Contains(raw, vin) {
		t.Fatal("the VIN is shown while task 3 is completed")
	}
	if states := milestoneStates(detail); states[0] != "completed" || states[1] != "in_progress" {
		t.Fatalf("milestone 1 is done and milestone 2 under way: %v", states)
	}

	if _, err := f.tasks.Reopen(f.Ctx, f.admin.Actor(), s.ID, 3, "Se corrige el VIN"); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	detail, raw = f.detail(token, s.ID)
	if _, ok := sectionsOf(detail)[3]; ok || strings.Contains(raw, vin) {
		t.Fatal("a reopened task and its VIN must disappear from the portal at once, although its data is still stored")
	}
	if len(sectionsOf(detail)) != 2 {
		t.Fatal("tasks 1 and 2 stay")
	}
	var stored string
	if err := f.Pool.QueryRow(f.Ctx, `SELECT vin FROM sale_registers WHERE id = $1`, s.ID).Scan(&stored); err != nil || stored != vin {
		t.Fatalf("the data itself is kept for the staff: %q (%v)", stored, err)
	}

	// Completed again, it comes back with the data that was kept.
	f.FillRequiredDocuments(s.ID, 3, f.advisor.ID)
	if _, err := f.tasks.Complete(f.Ctx, f.advisor.Actor(), s.ID, 3); err != nil {
		t.Fatalf("complete again: %v", err)
	}
	if again, _ := f.detail(token, s.ID); fieldKeys(sectionsOf(again)[3])["vin"] != vin {
		t.Fatal("completed again, the task is back in the portal")
	}
}

func TestAReopenedTasksFilesStopBeingOffered(t *testing.T) {
	f := newPortalFx(t)
	s := f.newSale(f.advisor)
	token := f.signIn(f.customerOf(s.ID))
	f.advance(s.ID, f.advisor, 2)

	var file uuid.UUID
	if err := f.Pool.QueryRow(f.Ctx, `SELECT d.id FROM documents d JOIN document_slots sl ON sl.slot_code = d.slot_code
		JOIN task_instances t ON t.id = d.task_instance_id WHERE d.sale_id = $1 AND t.task_number = 2 AND sl.is_customer_visible LIMIT 1`, s.ID).Scan(&file); err != nil {
		t.Fatal(err)
	}
	path := "/portal/documents/" + file.String() + "/download-url"
	if rec := f.get(path, token); rec.Code != http.StatusOK {
		t.Fatalf("task 2 is completed, its file is available: %d", rec.Code)
	}
	if _, err := f.tasks.Reopen(f.Ctx, f.admin.Actor(), s.ID, 2, "Se corrige el firmante"); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if rec := f.get(path, token); rec.Code != http.StatusNotFound {
		t.Fatalf("a reopened task's file must stop being offered: %d", rec.Code)
	}
	detail, _ := f.detail(token, s.ID)
	if states := milestoneStates(detail); states[0] != "in_progress" {
		t.Fatalf("milestone 1 moves back to in progress: %v", states)
	}
	if _, ok := sectionsOf(detail)[2]; ok {
		t.Fatal("and so does its section")
	}
}

func mustDetail(f *portalFx, token string, sale uuid.UUID) map[string]any {
	d, _ := f.detail(token, sale)
	return d
}

func TestACancelledPurchaseIsShownCancelledWithoutWhy(t *testing.T) {
	f := newPortalFx(t)
	s := f.newSale(f.advisor)
	token := f.signIn(f.customerOf(s.ID))
	f.advance(s.ID, f.advisor, 2)
	if _, err := f.sales.Cancel(f.Ctx, f.manager.Actor(), s.ID, "due_diligence_finding", "MOTIVO-CONFIDENCIAL-731"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	detail, raw := f.detail(token, s.ID)
	if detail["state"] != "cancelled" {
		t.Fatalf("state = %v", detail["state"])
	}
	for _, leak := range []string{"MOTIVO-CONFIDENCIAL-731", "due_diligence_finding", "cancel", "reason", "notes", "temperature"} {
		if strings.Contains(strings.ToLower(raw), strings.ToLower(leak)) && leak != "cancel" {
			t.Errorf("a cancelled purchase must not reveal %q", leak)
		}
	}
	// What was completed before stays readable.
	if len(sectionsOf(detail)) != 2 {
		t.Fatalf("the completed tasks stay readable: %d sections", len(sectionsOf(detail)))
	}
}

// ---- files ---------------------------------------------------------------------------------

func TestOnlyVisibleFilesOfCompletedTasksCanBeDownloaded(t *testing.T) {
	f := newPortalFx(t)
	s := f.newSale(f.advisor)
	token := f.signIn(f.customerOf(s.ID))
	f.advance(s.ID, f.advisor, 3)

	slot := func(code string) uuid.UUID {
		var id uuid.UUID
		if err := f.Pool.QueryRow(f.Ctx, `SELECT id FROM documents WHERE sale_id = $1 AND slot_code = $2`, s.ID, code).Scan(&id); err != nil {
			t.Fatalf("slot %s: %v", code, err)
		}
		return id
	}
	// Visible and completed.
	rec := f.get("/portal/documents/"+slot("quotation_pdf").String()+"/download-url", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("the quotation PDF: %d %s", rec.Code, rec.Body.String())
	}
	signed := decode(t, rec)
	if !strings.HasPrefix(signed["url"].(string), "https://img.test/") || signed["fileName"] == "" || signed["expiresAt"] == "" {
		t.Fatalf("a signed, expiring address: %v", signed)
	}
	// Hidden slots: the Excel quotation and the settlement answer as if they did not exist.
	missing := f.get("/portal/documents/"+uuid.NewString()+"/download-url", token)
	for _, hidden := range []string{"quotation_excel", "unit_settlement"} {
		rec := f.get("/portal/documents/"+slot(hidden).String()+"/download-url", token)
		if rec.Code != http.StatusNotFound || body(rec) != body(missing) {
			t.Errorf("%s must not be offered: %d", hidden, rec.Code)
		}
	}
}

// ---- the session over HTTP -----------------------------------------------------------------

func (f *portalFx) loginHTTP(customer uuid.UUID) *httptest.ResponseRecorder {
	f.signIn(customer)
	var docType, docNumber string
	_ = f.Pool.QueryRow(f.Ctx, `SELECT document_type::text, document_number FROM customers WHERE id = $1`, customer).Scan(&docType, &docNumber)
	payload, _ := json.Marshal(map[string]any{"documentType": docType, "documentNumber": docNumber, "password": testkit.Password, "rememberMe": true})
	return call(f.router, "POST", "/api/v1/auth/customer/login", string(payload), "")
}

func post(h http.Handler, path string, headers map[string]string, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", path, bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestCustomerSessionOverHTTPUsesItsOwnCookieAndOrigin(t *testing.T) {
	f := newPortalFx(t)
	s := f.newSale(f.advisor)
	login := f.loginHTTP(f.customerOf(s.ID))
	if login.Code != http.StatusOK {
		t.Fatalf("login: %d %s", login.Code, login.Body.String())
	}
	var cookie *http.Cookie
	for _, c := range login.Result().Cookies() {
		cookie = c
	}
	if cookie == nil || cookie.Name != "ef_customer_refresh" || cookie.Path != "/api/v1/auth/customer" || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("the customer's own, scoped, HttpOnly, strict cookie: %+v", cookie)
	}
	if decode(t, login)["accessToken"] == "" {
		t.Fatal("an access token comes with the login")
	}

	csrf := map[string]string{"X-Requested-With": "EndosoFlow", "Origin": "http://portal.test"}
	if rec := post(f.router, "/api/v1/auth/customer/refresh", csrf, cookie); rec.Code != http.StatusOK {
		t.Fatalf("refresh from the customer portal: %d %s", rec.Code, rec.Body.String())
	}
	// The employee portal's origin is not the customer portal's.
	wrongOrigin := map[string]string{"X-Requested-With": "EndosoFlow", "Origin": "http://app.test"}
	if rec := post(f.router, "/api/v1/auth/customer/refresh", wrongOrigin, cookie); rec.Code != http.StatusForbidden {
		t.Fatalf("refresh from another origin: %d", rec.Code)
	}
	// Without the custom header a form on another site could send it.
	if rec := post(f.router, "/api/v1/auth/customer/refresh", map[string]string{"Origin": "http://portal.test"}, cookie); rec.Code != http.StatusForbidden {
		t.Fatalf("refresh without the header: %d", rec.Code)
	}
	// The employees' refresh does not accept the customer's cookie.
	employeeCSRF := map[string]string{"X-Requested-With": "EndosoFlow", "Origin": "http://app.test"}
	if rec := post(f.router, "/api/v1/auth/refresh", employeeCSRF, cookie); rec.Code != http.StatusUnauthorized {
		t.Fatalf("a customer's cookie must not refresh an employee session: %d", rec.Code)
	}
	// Logout ends the session and clears the cookie.
	out := post(f.router, "/api/v1/auth/customer/logout", csrf, cookie)
	if out.Code != http.StatusNoContent {
		t.Fatalf("logout: %d", out.Code)
	}
	if rec := post(f.router, "/api/v1/auth/customer/refresh", csrf, cookie); rec.Code != http.StatusUnauthorized {
		t.Fatalf("a logged-out session must not refresh: %d", rec.Code)
	}
}

func TestACustomerCanSeeTheirProfileAndChangeTheirPassword(t *testing.T) {
	f := newPortalFx(t)
	s := f.newSale(f.advisor)
	token := f.signIn(f.customerOf(s.ID))

	me := decode(t, f.get("/portal/me", token))
	if me["displayName"] == "" || me["documentNumber"] == "" || me["email"] == "" {
		t.Fatalf("the profile: %v", me)
	}
	wrong := call(f.router, "PUT", "/api/v1/portal/me/password", `{"currentPassword":"nope-nope-nope-1","newPassword":"Brand-New-Password-5"}`, token)
	if wrong.Code != http.StatusUnprocessableEntity {
		t.Fatalf("a wrong current password: %d %s", wrong.Code, wrong.Body.String())
	}
	ok := call(f.router, "PUT", "/api/v1/portal/me/password", `{"currentPassword":"`+testkit.Password+`","newPassword":"Brand-New-Password-5"}`, token)
	if ok.Code != http.StatusNoContent {
		t.Fatalf("change: %d %s", ok.Code, ok.Body.String())
	}
}
