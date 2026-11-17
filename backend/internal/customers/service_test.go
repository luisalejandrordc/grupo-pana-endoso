package customers

import (
	"context"
	"encoding/json"
	"endosoflow/internal/adapters/codart"
	"endosoflow/internal/auth"
	"endosoflow/internal/platform/ids"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	return dsn
}

func ptrTo[T any](v T) *T { return &v }

// --- create ---

func TestCreatePersonKeepsLeadingZerosNormalizesAndAudits(t *testing.T) {
	e := newTestEnv(t)
	advisor := e.insertEmployee(auth.RoleSalesAdvisor, "")
	in := person()
	in.DocumentNumber = " 00 123456 "
	in.Email = "  LUCIA@Test.PE "
	c := e.createCustomer(advisor.actor(), in)

	if c.DocumentNumber != "00123456" || c.DocumentType != "DNI" {
		t.Fatalf("document = %s %s", c.DocumentType, c.DocumentNumber)
	}
	if c.DisplayName != "Lucía Quispe" || c.Email != "lucia@test.pe" || c.NameSource != "manual" {
		t.Fatalf("customer = %+v", c)
	}
	if c.LegalName != nil || c.ContactFirstName != nil {
		t.Fatalf("organization fields must be empty for a person: %+v", c)
	}
	if c.PortalStatus != "not_invited" || !c.IsActive || c.Version != 1 {
		t.Fatalf("lifecycle = %s active=%v version=%d", c.PortalStatus, c.IsActive, c.Version)
	}
	if e.countAudit(c.ID, "CREATED") != 1 {
		t.Fatal("expected one CREATED audit entry")
	}
}

func TestCreateOrganization(t *testing.T) {
	e := newTestEnv(t)
	manager := e.insertEmployee(auth.RoleSalesManager, "")
	in := organization()
	in.FirstName, in.LastName, in.Gender = "Ignorado", "Ignorado", "M" // do not apply to an organization
	c := e.createCustomer(manager.actor(), in)

	if c.DisplayName != in.LegalName || c.FirstName != nil || c.Gender != nil {
		t.Fatalf("customer = %+v", c)
	}
	if valueOf(c.ContactJobTitle) != "Agregado administrativo" {
		t.Fatalf("contact = %+v", c)
	}
}

func TestCreateRejectsWrongDocumentForTheType(t *testing.T) {
	e := newTestEnv(t)
	admin := e.insertEmployee(auth.RoleSystemAdmin, "")

	org := organization()
	org.DocumentType, org.DocumentNumber = "DNI", "12345678"
	_, err := e.svc.Create(e.ctx, admin.actor(), org)
	wantCode(t, err, "VALIDATION_FAILED")
	if !fieldNames(err)["documentType"] {
		t.Fatalf("an organization with a DNI must fail on documentType: %v", fieldNames(err))
	}

	p := person()
	p.DocumentType, p.DocumentNumber = "RUC", "20123456789"
	_, err = e.svc.Create(e.ctx, admin.actor(), p)
	if !fieldNames(err)["documentType"] {
		t.Fatalf("a person with a RUC must fail on documentType: %v", fieldNames(err))
	}

	for _, bad := range []struct{ docType, number string }{
		{"DNI", "1234567"}, {"DNI", "1234567A"}, {"CE", "123"}, {"PAS", "AB1"}, {"CDI", "AB"},
	} {
		p := person()
		p.DocumentType, p.DocumentNumber = bad.docType, bad.number
		_, err := e.svc.Create(e.ctx, admin.actor(), p)
		if !fieldNames(err)["documentNumber"] {
			t.Fatalf("%s %q should fail on documentNumber: %v", bad.docType, bad.number, fieldNames(err))
		}
	}
	for _, good := range []struct{ docType, number string }{
		{"CE", "ab1234567"}, {"PAS", "x1y2z3w4"}, {"CDI", "cd123456"},
	} {
		p := person()
		p.DocumentType, p.DocumentNumber = good.docType, good.number+randomDigits(0)
		c := e.createCustomer(admin.actor(), p)
		if c.DocumentNumber != strings.ToUpper(good.number) {
			t.Fatalf("%s document = %s", good.docType, c.DocumentNumber)
		}
	}
}

func TestCreateReportsEveryProblemAtOnce(t *testing.T) {
	e := newTestEnv(t)
	admin := e.insertEmployee(auth.RoleSystemAdmin, "")

	_, err := e.svc.Create(e.ctx, admin.actor(), CreateInput{CustomerType: TypeDiplomat})
	wantCode(t, err, "VALIDATION_FAILED")
	names := fieldNames(err)
	for _, want := range []string{"documentType", "firstName", "lastName", "gender", "nationality", "phone", "email"} {
		if !names[want] {
			t.Errorf("missing a problem for %s: %v", want, names)
		}
	}

	_, err = e.svc.Create(e.ctx, admin.actor(), CreateInput{CustomerType: TypeEmbassy, DocumentType: "RUC"})
	names = fieldNames(err)
	for _, want := range []string{"documentNumber", "legalName", "contactFirstName", "contactLastName", "contactJobTitle"} {
		if !names[want] {
			t.Errorf("missing a problem for %s: %v", want, names)
		}
	}

	_, err = e.svc.Create(e.ctx, admin.actor(), CreateInput{CustomerType: "otro"})
	if !fieldNames(err)["customerType"] {
		t.Fatalf("unknown type must fail on customerType: %v", fieldNames(err))
	}
}

func TestCreateRejectsDuplicatesAndAnyoneWhoIsNotStaff(t *testing.T) {
	e := newTestEnv(t)
	advisor := e.insertEmployee(auth.RoleSalesAdvisor, "")
	in := person()
	e.createCustomer(advisor.actor(), in)

	again := in
	again.Email = "otro-" + uuid.NewString() + "@test.pe"
	_, err := e.svc.Create(e.ctx, advisor.actor(), again)
	wantCode(t, err, "CUSTOMER_ALREADY_EXISTS")

	_, err = e.svc.Create(e.ctx, auth.Actor{ID: ids.New(), Role: auth.Role("customer")}, person())
	wantCode(t, err, "FORBIDDEN")
}

// --- scope ---

func TestAdvisorScopeFollowsSalesAndCreation(t *testing.T) {
	e := newTestEnv(t)
	manager := e.insertEmployee(auth.RoleSalesManager, "")
	admin := e.insertEmployee(auth.RoleSystemAdmin, "")
	ana := e.insertEmployee(auth.RoleSalesAdvisor, "")
	beto := e.insertEmployee(auth.RoleSalesAdvisor, "")

	c := e.createCustomer(ana.actor(), person())
	visibleTo := func(who testEmployee) bool {
		t.Helper()
		_, err := e.svc.Get(e.ctx, who.actor(), c.ID)
		if err != nil {
			wantCode(t, err, "CUSTOMER_NOT_FOUND")
			return false
		}
		return true
	}
	inList := func(who testEmployee) bool {
		t.Helper()
		res, err := e.svc.List(e.ctx, who.actor(), ListFilter{Search: c.DocumentNumber})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		return res.Total == 1
	}

	// The creator keeps access until the first sale exists; nobody else in sales sees it.
	if !visibleTo(ana) || !inList(ana) || visibleTo(beto) || inList(beto) {
		t.Fatal("a new customer must be visible to its creator only")
	}
	if !visibleTo(manager) || !visibleTo(admin) || !inList(manager) || !inList(admin) {
		t.Fatal("managers and admins see every customer")
	}

	// A sale assigned to Beto links Beto and ends Ana's creator access.
	saleID := e.addSale(c, beto.ID)
	if visibleTo(ana) || inList(ana) || !visibleTo(beto) || !inList(beto) {
		t.Fatal("after the first sale only the assigned advisor keeps access")
	}

	// Reassigning the only sale moves the access.
	e.reassign(saleID, ana.ID)
	if !visibleTo(ana) || visibleTo(beto) {
		t.Fatal("reassigning the only sale must move access to the new advisor")
	}
}

func TestAdvisorCannotEditOrDeactivateOutsideTheirScope(t *testing.T) {
	e := newTestEnv(t)
	ana := e.insertEmployee(auth.RoleSalesAdvisor, "")
	beto := e.insertEmployee(auth.RoleSalesAdvisor, "")
	c := e.createCustomer(ana.actor(), person())

	_, err := e.svc.Update(e.ctx, beto.actor(), c.ID, UpdateInput{Version: ptrTo(1), Phone: ptrTo("999000111")})
	wantCode(t, err, "CUSTOMER_NOT_FOUND")
	_, err = e.svc.Deactivate(e.ctx, ana.actor(), c.ID)
	wantCode(t, err, "FORBIDDEN")
	_, err = e.svc.Reactivate(e.ctx, ana.actor(), c.ID)
	wantCode(t, err, "FORBIDDEN")

	updated, err := e.svc.Update(e.ctx, ana.actor(), c.ID, UpdateInput{Version: ptrTo(1), Phone: ptrTo("999000111")})
	if err != nil || updated.Phone != "999000111" {
		t.Fatalf("an advisor edits customers in scope: %v", err)
	}
}

// --- update ---

func TestUpdateChecksVersionAuditsWithoutLeakingContactData(t *testing.T) {
	e := newTestEnv(t)
	manager := e.insertEmployee(auth.RoleSalesManager, "")
	c := e.createCustomer(manager.actor(), person())

	_, err := e.svc.Update(e.ctx, manager.actor(), c.ID, UpdateInput{Phone: ptrTo("999000111")})
	wantCode(t, err, "VERSION_REQUIRED")

	updated, err := e.svc.Update(e.ctx, manager.actor(), c.ID, UpdateInput{
		Version: ptrTo(1), Phone: ptrTo("999000111"), Email: ptrTo("Nuevo@Test.pe"), Nationality: ptrTo("Chilena"),
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Version != 2 || updated.Phone != "999000111" || updated.Email != "nuevo@test.pe" || updated.Nationality != "Chilena" {
		t.Fatalf("updated = %+v", updated)
	}
	if e.countAudit(c.ID, "UPDATED") != 1 {
		t.Fatal("expected one UPDATED audit entry")
	}
	var summary string
	if err := e.pool.QueryRow(e.ctx, `SELECT changes_summary::text FROM audit_logs
		WHERE entity_id = $1 AND action = 'UPDATED'`, c.ID).Scan(&summary); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(summary, "999000111") || strings.Contains(summary, "nuevo@test.pe") {
		t.Fatalf("the audit entry must not contain contact data: %s", summary)
	}
	if !strings.Contains(summary, "Chilena") {
		t.Fatalf("the audit entry should record the nationality change: %s", summary)
	}

	_, err = e.svc.Update(e.ctx, manager.actor(), c.ID, UpdateInput{Version: ptrTo(1), Phone: ptrTo("999222333")})
	wantCode(t, err, "VERSION_CONFLICT")
}

func TestEditingANameMarksItManualUnlessToldOtherwise(t *testing.T) {
	e := newTestEnv(t)
	manager := e.insertEmployee(auth.RoleSalesManager, "")
	in := person()
	in.NameSource = "looked_up"
	c := e.createCustomer(manager.actor(), in)
	if c.NameSource != "looked_up" {
		t.Fatalf("source = %s", c.NameSource)
	}

	phoneOnly, err := e.svc.Update(e.ctx, manager.actor(), c.ID, UpdateInput{Version: ptrTo(1), Phone: ptrTo("999000111")})
	if err != nil || phoneOnly.NameSource != "looked_up" {
		t.Fatalf("a non-name edit keeps the source: %v %+v", err, phoneOnly)
	}
	renamed, err := e.svc.Update(e.ctx, manager.actor(), c.ID, UpdateInput{Version: ptrTo(2), LastName: ptrTo("Quispe Rojas")})
	if err != nil || renamed.NameSource != "manual" || renamed.DisplayName != "Lucía Quispe Rojas" {
		t.Fatalf("a name edit marks it manual: %v %+v", err, renamed)
	}
}

func TestUpdateCanChangeTheShapeOnlyWithTheNewFields(t *testing.T) {
	e := newTestEnv(t)
	manager := e.insertEmployee(auth.RoleSalesManager, "")
	c := e.createCustomer(manager.actor(), person())

	_, err := e.svc.Update(e.ctx, manager.actor(), c.ID, UpdateInput{Version: ptrTo(1), CustomerType: ptrTo(TypeEmbassy)})
	wantCode(t, err, "VALIDATION_FAILED") // still has a DNI and no legal name or contact

	org := organization()
	switched, err := e.svc.Update(e.ctx, manager.actor(), c.ID, UpdateInput{
		Version: ptrTo(1), CustomerType: ptrTo(TypeEmbassy), DocumentType: ptrTo("RUC"), DocumentNumber: &org.DocumentNumber,
		LegalName: &org.LegalName, ContactFirstName: &org.ContactFirstName, ContactLastName: &org.ContactLastName,
		ContactJobTitle: &org.ContactJobTitle,
	})
	if err != nil {
		t.Fatalf("switch shape: %v", err)
	}
	if switched.FirstName != nil || switched.Gender != nil || switched.LegalName == nil {
		t.Fatalf("the person fields must be cleared: %+v", switched)
	}
}

func TestEditingACustomerNeverChangesExistingSaleSnapshots(t *testing.T) {
	e := newTestEnv(t)
	manager := e.insertEmployee(auth.RoleSalesManager, "")
	advisor := e.insertEmployee(auth.RoleSalesAdvisor, "")
	c := e.createCustomer(manager.actor(), organization())
	saleID := e.addSale(c, advisor.ID)

	_, err := e.svc.Update(e.ctx, manager.actor(), c.ID, UpdateInput{Version: ptrTo(1), LegalName: ptrTo("Nombre Nuevo SAC")})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	var snapshot string
	if err := e.pool.QueryRow(e.ctx, `SELECT customer_legal_name_snapshot FROM sale_registers WHERE id = $1`, saleID).Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot != c.DisplayName {
		t.Fatalf("snapshot changed to %q, want %q", snapshot, c.DisplayName)
	}
}

// --- activate, deactivate, delete ---

func TestDeactivateAndReactivate(t *testing.T) {
	e := newTestEnv(t)
	manager := e.insertEmployee(auth.RoleSalesManager, "")
	c := e.createCustomer(manager.actor(), person())

	off, err := e.svc.Deactivate(e.ctx, manager.actor(), c.ID)
	if err != nil || off.IsActive {
		t.Fatalf("deactivate: %v %+v", err, off)
	}
	_, err = e.svc.Deactivate(e.ctx, manager.actor(), c.ID)
	wantCode(t, err, "CUSTOMER_ALREADY_INACTIVE")

	on, err := e.svc.Reactivate(e.ctx, manager.actor(), c.ID)
	if err != nil || !on.IsActive {
		t.Fatalf("reactivate: %v %+v", err, on)
	}
	_, err = e.svc.Reactivate(e.ctx, manager.actor(), c.ID)
	wantCode(t, err, "CUSTOMER_ALREADY_ACTIVE")

	if e.countAudit(c.ID, "DEACTIVATED") != 1 || e.countAudit(c.ID, "REACTIVATED") != 1 {
		t.Fatal("expected one DEACTIVATED and one REACTIVATED audit entry")
	}
	_, err = e.svc.Deactivate(e.ctx, manager.actor(), ids.New())
	wantCode(t, err, "CUSTOMER_NOT_FOUND")
}

func TestDeleteIsAdminOnlyNeedsAReasonAndRefusesCustomersWithSales(t *testing.T) {
	e := newTestEnv(t)
	admin := e.insertEmployee(auth.RoleSystemAdmin, "")
	manager := e.insertEmployee(auth.RoleSalesManager, "")
	advisor := e.insertEmployee(auth.RoleSalesAdvisor, "")

	withSale := e.createCustomer(manager.actor(), person())
	e.addSale(withSale, advisor.ID)
	wantCode(t, e.svc.Delete(e.ctx, admin.actor(), withSale.ID, "Creado por error"), "CUSTOMER_HAS_SALES")

	c := e.createCustomer(manager.actor(), person())
	wantCode(t, e.svc.Delete(e.ctx, manager.actor(), c.ID, "Creado por error"), "FORBIDDEN")
	wantCode(t, e.svc.Delete(e.ctx, admin.actor(), c.ID, "  "), "REASON_REQUIRED")
	if err := e.svc.Delete(e.ctx, admin.actor(), c.ID, "Creado por error"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if e.countAudit(c.ID, "DELETED") != 1 {
		t.Fatal("expected a DELETED audit entry with the reason")
	}
	_, err := e.svc.Get(e.ctx, admin.actor(), c.ID)
	wantCode(t, err, "CUSTOMER_NOT_FOUND")
	wantCode(t, e.svc.Delete(e.ctx, admin.actor(), c.ID, "otra vez"), "CUSTOMER_NOT_FOUND")
}

// --- list ---

func TestListSearchFilterSortAndPagination(t *testing.T) {
	e := newTestEnv(t)
	manager := e.insertEmployee(auth.RoleSalesManager, "")
	tag := randomDigits(6)

	a := person()
	a.FirstName, a.LastName = "José"+tag, "Álvarez"
	b := person()
	b.FirstName, b.LastName = "Beatriz"+tag, "Zegarra"
	org := organization()
	org.LegalName = "Entidad " + tag
	org.CustomerType = TypeCatholicEntity
	ca := e.createCustomer(manager.actor(), a)
	cb := e.createCustomer(manager.actor(), b)
	co := e.createCustomer(manager.actor(), org)
	if _, err := e.svc.Deactivate(e.ctx, manager.actor(), cb.ID); err != nil {
		t.Fatal(err)
	}

	all, err := e.svc.List(e.ctx, manager.actor(), ListFilter{Search: tag})
	if err != nil || all.Total != 3 {
		t.Fatalf("search by tag: %v %+v", err, all)
	}
	if res, _ := e.svc.List(e.ctx, manager.actor(), ListFilter{Search: "JOSE" + tag}); res == nil || res.Total != 1 || res.Items[0].ID != ca.ID {
		t.Fatalf("search must ignore accents and case: %+v", res)
	}
	if res, _ := e.svc.List(e.ctx, manager.actor(), ListFilter{Search: ca.DocumentNumber[:5]}); res == nil || res.Total < 1 {
		t.Fatalf("search by document prefix: %+v", res)
	}
	if res, _ := e.svc.List(e.ctx, manager.actor(), ListFilter{Search: tag, Status: "inactive"}); res.Total != 1 || res.Items[0].ID != cb.ID {
		t.Fatalf("inactive filter: %+v", res)
	}
	if res, _ := e.svc.List(e.ctx, manager.actor(), ListFilter{Search: tag, CustomerType: TypeCatholicEntity}); res.Total != 1 || res.Items[0].ID != co.ID {
		t.Fatalf("type filter: %+v", res)
	}

	asc, _ := e.svc.List(e.ctx, manager.actor(), ListFilter{Search: tag, Sort: "displayName"})
	desc, _ := e.svc.List(e.ctx, manager.actor(), ListFilter{Search: tag, Sort: "-displayName"})
	if asc.Items[0].ID != desc.Items[2].ID || asc.Items[2].ID != desc.Items[0].ID {
		t.Fatal("-displayName must reverse displayName")
	}

	page2, _ := e.svc.List(e.ctx, manager.actor(), ListFilter{Search: tag, Page: 2, PageSize: 2})
	if page2.Total != 3 || len(page2.Items) != 1 || page2.PageSize != 2 {
		t.Fatalf("pagination: %+v", page2)
	}
	if clamped, _ := e.svc.List(e.ctx, manager.actor(), ListFilter{PageSize: 5000}); clamped.PageSize != 100 {
		t.Fatalf("page size must be capped at 100, got %d", clamped.PageSize)
	}
}

func TestSearchTreatsWildcardsAsPlainText(t *testing.T) {
	e := newTestEnv(t)
	manager := e.insertEmployee(auth.RoleSalesManager, "")
	e.createCustomer(manager.actor(), person())
	res, err := e.svc.List(e.ctx, manager.actor(), ListFilter{Search: "%"})
	if err != nil || res.Total != 0 {
		t.Fatalf("a lone %% must not match everything: %v %+v", err, res)
	}
}

// --- existence lookup ---

func TestLookupGivesFullAccessInScopeAndNameOnlyOutside(t *testing.T) {
	e := newTestEnv(t)
	manager := e.insertEmployee(auth.RoleSalesManager, "")
	ana := e.insertEmployee(auth.RoleSalesAdvisor, "")
	beto := e.insertEmployee(auth.RoleSalesAdvisor, "")
	in := person()
	c := e.createCustomer(ana.actor(), in)

	none, err := e.svc.Lookup(e.ctx, manager.actor(), "DNI", "11112222")
	if err != nil || none.Exists || none.Customer != nil || none.Brief != nil {
		t.Fatalf("unknown document: %v %+v", err, none)
	}

	full, err := e.svc.Lookup(e.ctx, manager.actor(), "dni", " "+c.DocumentNumber+" ")
	if err != nil || !full.Exists || full.Access != "full" || full.Customer == nil || full.Customer.Email == "" {
		t.Fatalf("manager lookup: %v %+v", err, full)
	}
	own, _ := e.svc.Lookup(e.ctx, ana.actor(), "DNI", c.DocumentNumber)
	if own.Access != "full" || own.Customer == nil {
		t.Fatalf("the creator is linked: %+v", own)
	}

	other, err := e.svc.Lookup(e.ctx, beto.actor(), "DNI", c.DocumentNumber)
	if err != nil || other.Access != "name_only" || other.Customer != nil || other.Brief == nil {
		t.Fatalf("unlinked advisor: %v %+v", err, other)
	}
	body, _ := json.Marshal(other)
	for _, secret := range []string{c.Email, c.Phone, "sale", "gender", c.DocumentNumber} {
		if strings.Contains(strings.ToLower(string(body)), strings.ToLower(secret)) {
			t.Fatalf("a name-only answer leaks %q: %s", secret, body)
		}
	}
	if other.Brief.DisplayName != c.DisplayName || other.Brief.ID != c.ID {
		t.Fatalf("brief = %+v", other.Brief)
	}

	_, err = e.svc.Lookup(e.ctx, beto.actor(), "DNI", "123")
	wantCode(t, err, "VALIDATION_FAILED")
	if e.countLookupLogs(beto.ID, "customer_existence", "success") != 1 {
		t.Fatal("every lookup must be logged")
	}
}

func TestLookupIsRateLimitedPerEmployee(t *testing.T) {
	e := newTestEnv(t, func(d *Deps) { d.LookupsPerMinute = 3 })
	ana := e.insertEmployee(auth.RoleSalesAdvisor, "")
	beto := e.insertEmployee(auth.RoleSalesAdvisor, "")

	for i := 0; i < 3; i++ {
		if _, err := e.svc.Lookup(e.ctx, ana.actor(), "DNI", "11112222"); err != nil {
			t.Fatalf("lookup %d: %v", i, err)
		}
	}
	_, err := e.svc.Lookup(e.ctx, ana.actor(), "DNI", "11112222")
	wantCode(t, err, "RATE_LIMITED")
	if e.countLookupLogs(ana.ID, "customer_existence", "rate_limited") != 1 {
		t.Fatal("the refusal must be logged")
	}
	if _, err := e.svc.Lookup(e.ctx, beto.actor(), "DNI", "11112222"); err != nil {
		t.Fatalf("another employee is not affected: %v", err)
	}
}

// --- identity (Codart) ---

func TestIdentityLookupDNIAndRUC(t *testing.T) {
	e := newTestEnv(t)
	advisor := e.insertEmployee(auth.RoleSalesAdvisor, "")
	dni := "7" + randomDigits(7)
	ruc := "20" + randomDigits(9)

	res, err := e.svc.LookupIdentity(e.ctx, advisor.actor(), "DNI", dni)
	if err != nil || res.Source != "provider" || valueOf(res.FirstName) != "LUCIA" || valueOf(res.LastName) != "QUISPE MAMANI" || res.LegalName != nil {
		t.Fatalf("dni: %v %+v", err, res)
	}
	res, err = e.svc.LookupIdentity(e.ctx, advisor.actor(), "ruc", ruc)
	if err != nil || valueOf(res.LegalName) != "EMBAJADA DE PRUEBA" || res.FirstName != nil {
		t.Fatalf("ruc: %v %+v", err, res)
	}
	if e.identity.count() != 2 || e.countLookupLogs(advisor.ID, "codart_dni", "success") != 1 || e.countLookupLogs(advisor.ID, "codart_ruc", "success") != 1 {
		t.Fatalf("provider calls = %d, and each must be logged", e.identity.count())
	}
}

func TestIdentityLookupOnlySupportsDNIAndRUC(t *testing.T) {
	e := newTestEnv(t)
	advisor := e.insertEmployee(auth.RoleSalesAdvisor, "")
	for _, docType := range []string{"CE", "PAS", "CDI", "XYZ"} {
		_, err := e.svc.LookupIdentity(e.ctx, advisor.actor(), docType, "AB1234567")
		wantCode(t, err, "LOOKUP_NOT_SUPPORTED")
	}
	_, err := e.svc.LookupIdentity(e.ctx, advisor.actor(), "DNI", "123")
	wantCode(t, err, "VALIDATION_FAILED")
	_, err = e.svc.LookupIdentity(e.ctx, auth.Actor{ID: ids.New(), Role: auth.Role("customer")}, "DNI", "12345678")
	wantCode(t, err, "FORBIDDEN")
	if e.identity.count() != 0 {
		t.Fatal("none of these may reach the provider")
	}
}

func TestIdentityLookupChecksOurDatabaseFirstAndCostsNoProviderCall(t *testing.T) {
	e := newTestEnv(t)
	manager := e.insertEmployee(auth.RoleSalesManager, "")
	advisor := e.insertEmployee(auth.RoleSalesAdvisor, "")
	c := e.createCustomer(manager.actor(), person())

	res, err := e.svc.LookupIdentity(e.ctx, advisor.actor(), "DNI", c.DocumentNumber)
	if err != nil || res.Source != "database" || valueOf(res.FirstName) != "Lucía" {
		t.Fatalf("known customer: %v %+v", err, res)
	}
	if e.identity.count() != 0 {
		t.Fatal("a known customer must not call the provider")
	}
	var used int
	from, to := quotaPeriod(time.Now())
	used, _ = countProviderCalls(e.ctx, e.pool, from, to)
	if e.countLookupLogs(advisor.ID, "codart_dni", "success") != 0 {
		t.Fatalf("a database answer must not count as a provider call (used=%d)", used)
	}
}

func TestIdentityLookupFailuresAreMappedAndStillCounted(t *testing.T) {
	e := newTestEnv(t)
	advisor := e.insertEmployee(auth.RoleSalesAdvisor, "")

	e.identity.dni = func(string) (codart.Person, error) { return codart.Person{}, codart.ErrNotFound }
	_, err := e.svc.LookupIdentity(e.ctx, advisor.actor(), "DNI", "7"+randomDigits(7))
	wantCode(t, err, "IDENTITY_NOT_FOUND")
	if e.countLookupLogs(advisor.ID, "codart_dni", "not_found") != 1 {
		t.Fatal("a not-found answer must be logged as not_found")
	}

	e.identity.dni = func(string) (codart.Person, error) { return codart.Person{}, os.ErrDeadlineExceeded }
	_, err = e.svc.LookupIdentity(e.ctx, advisor.actor(), "DNI", "7"+randomDigits(7))
	wantCode(t, err, "LOOKUP_UNAVAILABLE")
	if e.countLookupLogs(advisor.ID, "codart_dni", "error") != 1 {
		t.Fatal("a provider failure must be logged as error")
	}
}

func TestIdentityLookupWithoutAProviderAsksForManualEntry(t *testing.T) {
	e := newTestEnv(t, func(d *Deps) { d.Identity = nil })
	advisor := e.insertEmployee(auth.RoleSalesAdvisor, "")
	_, err := e.svc.LookupIdentity(e.ctx, advisor.actor(), "DNI", "7"+randomDigits(7))
	wantCode(t, err, "LOOKUP_UNAVAILABLE")
}

func TestIdentityLookupStopsAtTheMonthlyQuota(t *testing.T) {
	probe := newTestEnv(t)
	from, to := quotaPeriod(time.Now())
	baseline, err := countProviderCalls(probe.ctx, probe.pool, from, to)
	if err != nil {
		t.Fatal(err)
	}
	e := newTestEnv(t, func(d *Deps) { d.MonthlyQuota = baseline + 2 })
	admin := e.insertEmployee(auth.RoleSystemAdmin, "")
	advisor := e.insertEmployee(auth.RoleSalesAdvisor, "")

	for i := 0; i < 2; i++ {
		if _, err := e.svc.LookupIdentity(e.ctx, advisor.actor(), "DNI", "7"+randomDigits(7)); err != nil {
			t.Fatalf("lookup %d: %v", i, err)
		}
	}
	_, err = e.svc.LookupIdentity(e.ctx, advisor.actor(), "DNI", "7"+randomDigits(7))
	wantCode(t, err, "LOOKUP_QUOTA_EXHAUSTED")
	if e.identity.count() != 2 || e.countLookupLogs(advisor.ID, "codart_dni", "quota_blocked") != 1 {
		t.Fatalf("a blocked lookup must not reach the provider and must be logged (calls=%d)", e.identity.count())
	}

	// A customer we already have still answers when the quota is gone.
	known := e.createCustomer(advisor.actor(), person())
	if res, err := e.svc.LookupIdentity(e.ctx, advisor.actor(), "DNI", known.DocumentNumber); err != nil || res.Source != "database" {
		t.Fatalf("known customer after the quota: %v %+v", err, res)
	}

	status, err := e.svc.IdentityQuota(e.ctx, admin.actor())
	if err != nil || status.Limit != baseline+2 || status.Used != baseline+2 || status.Remaining != 0 {
		t.Fatalf("quota = %+v (%v)", status, err)
	}
	if !status.PeriodStart.Before(status.PeriodEnd) {
		t.Fatalf("period = %v to %v", status.PeriodStart, status.PeriodEnd)
	}
	_, err = e.svc.IdentityQuota(e.ctx, advisor.actor())
	wantCode(t, err, "FORBIDDEN")
}

func TestQuotaCannotBeOvershotByConcurrentRequests(t *testing.T) {
	probe := newTestEnv(t)
	from, to := quotaPeriod(time.Now())
	baseline, _ := countProviderCalls(probe.ctx, probe.pool, from, to)
	e := newTestEnv(t, func(d *Deps) { d.MonthlyQuota = baseline + 1; d.LookupsPerMinute = 50 })
	advisor := e.insertEmployee(auth.RoleSalesAdvisor, "")

	var wg sync.WaitGroup
	var succeeded, blocked atomic.Int32
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := e.svc.LookupIdentity(e.ctx, advisor.actor(), "DNI", "7"+randomDigits(7))
			switch code(err) {
			case "":
				succeeded.Add(1)
			case "LOOKUP_QUOTA_EXHAUSTED":
				blocked.Add(1)
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()
	if succeeded.Load() != 1 || blocked.Load() != 7 || e.identity.count() != 1 {
		t.Fatalf("succeeded=%d blocked=%d provider calls=%d, want 1, 7 and 1", succeeded.Load(), blocked.Load(), e.identity.count())
	}
}

// Overshooting needs unlucky timing, so this checks the cause instead: while another connection holds the
// quota lock, a lookup must wait, and it must go through once the lock is released.
func TestQuotaReservationWaitsForTheQuotaLock(t *testing.T) {
	e := newTestEnv(t)
	advisor := e.insertEmployee(auth.RoleSalesAdvisor, "")

	holder, err := e.pool.Acquire(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Release()
	if _, err := holder.Exec(e.ctx, `SELECT pg_advisory_lock(hashtext('codart_quota'))`); err != nil {
		t.Fatal(err)
	}
	released := false
	defer func() {
		if !released {
			_, _ = holder.Exec(e.ctx, `SELECT pg_advisory_unlock(hashtext('codart_quota'))`)
		}
	}()

	done := make(chan error, 1)
	go func() {
		_, err := e.svc.LookupIdentity(e.ctx, advisor.actor(), "DNI", "7"+randomDigits(7))
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("the lookup finished while the quota lock was held (err=%v)", err)
	case <-time.After(400 * time.Millisecond):
	}
	if _, err := holder.Exec(e.ctx, `SELECT pg_advisory_unlock(hashtext('codart_quota'))`); err != nil {
		t.Fatal(err)
	}
	released = true
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("the lookup failed after the lock was released: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the lookup never finished after the lock was released")
	}
}

func TestIdentityLookupIsRateLimitedPerEmployee(t *testing.T) {
	e := newTestEnv(t, func(d *Deps) { d.LookupsPerMinute = 2 })
	advisor := e.insertEmployee(auth.RoleSalesAdvisor, "")
	for i := 0; i < 2; i++ {
		if _, err := e.svc.LookupIdentity(e.ctx, advisor.actor(), "DNI", "7"+randomDigits(7)); err != nil {
			t.Fatalf("lookup %d: %v", i, err)
		}
	}
	_, err := e.svc.LookupIdentity(e.ctx, advisor.actor(), "DNI", "7"+randomDigits(7))
	wantCode(t, err, "RATE_LIMITED")
	if e.identity.count() != 2 {
		t.Fatalf("a throttled request must not reach the provider (calls=%d)", e.identity.count())
	}
}

func TestQuotaPeriodUsesTheLimaMonth(t *testing.T) {
	// 03:00 UTC on 1 Oct is still 22:00 on 30 Sep in Lima.
	start, end := quotaPeriod(time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC))
	if start.Format("2006-01-02 -0700") != "2026-09-01 -0500" || end.Format("2006-01-02 -0700") != "2026-10-01 -0500" {
		t.Fatalf("period = %v to %v", start, end)
	}
	start, _ = quotaPeriod(time.Date(2026, 10, 1, 6, 0, 0, 0, time.UTC))
	if start.Format("2006-01-02") != "2026-10-01" {
		t.Fatalf("start = %v", start)
	}
}

// notifierRecorder stands in for the notifications writer: it records the call and writes the real queue row, so
// the tests see what the database holds.
type notifierRecorder struct {
	mu    sync.Mutex
	calls []string // emails
}

func (n *notifierRecorder) CustomerCreated(ctx context.Context, tx pgx.Tx, id uuid.UUID, email string) error {
	n.mu.Lock()
	n.calls = append(n.calls, email)
	n.mu.Unlock()
	_, err := tx.Exec(ctx, `INSERT INTO notification_logs (id, notification_type, recipient_customer_id, recipient_email, dedupe_key)
		VALUES ($1, 'customer_account_created', $2, $3, $4)`, ids.New(), id, email, "customer_account_created:"+id.String())
	return err
}

func TestCreateQueuesTheWelcomeEmailAndDeleteCleansItUp(t *testing.T) {
	rec := &notifierRecorder{}
	e := newTestEnv(t, func(d *Deps) { d.Notifier = rec })
	admin := e.insertEmployee(auth.RoleSystemAdmin, "")

	in := person()
	c := e.createCustomer(admin.actor(), in)
	if len(rec.calls) != 1 || rec.calls[0] != c.Email {
		t.Fatalf("expected the welcome queued to %q, got %v", c.Email, rec.calls)
	}
	var queued int
	if err := e.pool.QueryRow(e.ctx, `SELECT count(*) FROM notification_logs WHERE recipient_customer_id = $1`, c.ID).Scan(&queued); err != nil || queued != 1 {
		t.Fatalf("queued rows: %d (%v)", queued, err)
	}

	// The dispatcher mints an activation link when it sends; the customer must still be deletable afterwards.
	if _, err := e.pool.Exec(e.ctx, `INSERT INTO account_tokens (purpose, customer_id, token_hash, expires_at)
		VALUES ('activation', $1, $2, now() + interval '1 hour')`, c.ID, "hash-"+ids.NewString()); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Delete(e.ctx, admin.actor(), c.ID, "Creado por error"); err != nil {
		t.Fatalf("a customer with a welcome email and a link must be deletable: %v", err)
	}
	if err := e.pool.QueryRow(e.ctx, `SELECT count(*) FROM notification_logs WHERE recipient_customer_id = $1`, c.ID).Scan(&queued); err != nil || queued != 0 {
		t.Fatalf("the queued email must go with the customer (%d, %v)", queued, err)
	}
}

func TestAFailingNotifierRollsBackTheCustomer(t *testing.T) {
	e := newTestEnv(t, func(d *Deps) { d.Notifier = failingNotifier{} })
	admin := e.insertEmployee(auth.RoleSystemAdmin, "")
	in := person()
	if _, err := e.svc.Create(e.ctx, admin.actor(), in); err == nil {
		t.Fatal("expected the creation to fail")
	}
	if _, err := e.svc.Lookup(e.ctx, admin.actor(), in.DocumentType, in.DocumentNumber); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = e.pool.QueryRow(e.ctx, `SELECT count(*) FROM customers WHERE document_number = $1`, in.DocumentNumber).Scan(&n)
	if n != 0 {
		t.Fatal("the customer must not exist when its email could not be queued")
	}
}

type failingNotifier struct{}

func (failingNotifier) CustomerCreated(context.Context, pgx.Tx, uuid.UUID, string) error {
	return errors.New("queue unavailable")
}
