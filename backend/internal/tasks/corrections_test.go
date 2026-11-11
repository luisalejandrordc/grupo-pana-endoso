package tasks_test

import (
	"endosoflow/internal/auth"
	"endosoflow/internal/tasks"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// --- who may act ---

func TestAnAdvisorOnlyWorksOnTheirOwnSales(t *testing.T) {
	f := newFx(t)
	sale := f.newSale(f.advisor)

	// For another advisor the sale does not exist at all.
	if _, err := f.tasks.Get(f.Ctx, f.other.Actor(), sale, 1); code(err) != "SALE_NOT_FOUND" {
		t.Fatalf("another advisor must not see the sale: %v", err)
	}
	if _, err := f.tasks.List(f.Ctx, f.other.Actor(), sale); code(err) != "SALE_NOT_FOUND" {
		t.Fatalf("list: %v", err)
	}
	wantCode(t, f.start(f.other, sale, 1), "SALE_NOT_FOUND")
	v := 1
	if _, err := f.tasks.SaveData(f.Ctx, f.other.Actor(), sale, 1, tasks.SaveInput{Version: &v, Data: []byte(`{}`)}); code(err) != "SALE_NOT_FOUND" {
		t.Fatalf("save: %v", err)
	}

	// Managers and admins work on any sale; the owner's advisor can too.
	if err := f.start(f.manager, sale, 1); err != nil {
		t.Fatalf("a manager starts any sale's task: %v", err)
	}
	f.mustSave(f.admin, sale, 1, f.validData(1))
	f.FillRequiredDocuments(sale, 1, f.advisor.ID)
	if err := f.complete(f.advisor, sale, 1); err != nil {
		t.Fatalf("the owner completes it: %v", err)
	}
}

func TestOnlyStaffRolesCanUseTheEngine(t *testing.T) {
	f := newFx(t)
	sale := f.newSale(f.advisor)
	outsider := f.tasks
	actor := auth.Actor{ID: f.advisor.ID, Role: auth.Role("customer")}
	if _, err := outsider.Get(f.Ctx, actor, sale, 1); code(err) != "FORBIDDEN" {
		t.Fatalf("get: %v", err)
	}
	if _, err := outsider.Start(f.Ctx, actor, sale, 1); code(err) != "FORBIDDEN" {
		t.Fatalf("start: %v", err)
	}
}

// --- restart and reopen ---

func TestRestartPutsATaskBackToPendingAndKeepsItsData(t *testing.T) {
	f := newFx(t)
	sale := f.newSale(f.advisor)
	f.run(f.advisor, sale, 1)
	if err := f.start(f.advisor, sale, 2); err != nil {
		t.Fatal(err)
	}
	f.mustSave(f.advisor, sale, 2, f.validData(2))

	_, err := f.tasks.Restart(f.Ctx, f.advisor.Actor(), sale, 2, "x")
	wantCode(t, err, "FORBIDDEN")
	_, err = f.tasks.Restart(f.Ctx, f.manager.Actor(), sale, 2, "  ")
	wantCode(t, err, "REASON_REQUIRED")
	_, err = f.tasks.Restart(f.Ctx, f.manager.Actor(), sale, 1, "x")
	wantCode(t, err, "TASK_NOT_IN_PROGRESS") // task 1 is completed

	d, err := f.tasks.Restart(f.Ctx, f.manager.Actor(), sale, 2, "Se inició por error")
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != tasks.StatusPending || d.StartedAt != nil || d.DueDate != nil || d.CompletionPhase != nil {
		t.Fatalf("a restarted task is pending with a clean clock: %+v", d.Summary)
	}
	if asMap(t, d.Data)["signerFirstName"] != "Ana" {
		t.Fatalf("the data entered is kept: %v", d.Data)
	}
	if f.auditCount(f.taskID(sale, 2), "RESTARTED") != 1 {
		t.Fatal("expected a RESTARTED audit entry")
	}
	// Read-only while pending, and a fresh start gets a fresh clock.
	_, err = f.save(f.manager, sale, 2, `{"signerFirstName":"Otra"}`)
	wantCode(t, err, "TASK_READ_ONLY")
	f.clock.Advance(5 * 3600 * 1e9)
	if err := f.start(f.advisor, sale, 2); err != nil {
		t.Fatalf("it can be started again: %v", err)
	}
	if again := f.detail(f.advisor, sale, 2); again.StartedAt == nil || again.DueDate == nil {
		t.Fatalf("a fresh start sets a fresh clock: %+v", again.Summary)
	}
}

func TestOnlyTheLastCompletedTaskCanBeReopenedAndOnlyWhenNothingRuns(t *testing.T) {
	f := newFx(t)
	sale := f.newSale(f.advisor)
	f.runThrough(f.advisor, sale, 3)

	_, err := f.tasks.Reopen(f.Ctx, f.advisor.Actor(), sale, 3, "x")
	wantCode(t, err, "FORBIDDEN")
	_, err = f.tasks.Reopen(f.Ctx, f.manager.Actor(), sale, 3, " ")
	wantCode(t, err, "REASON_REQUIRED")
	_, err = f.tasks.Reopen(f.Ctx, f.manager.Actor(), sale, 2, "x")
	wantCode(t, err, "TASK_REOPEN_NOT_ALLOWED") // task 3 is completed after it
	_, err = f.tasks.Reopen(f.Ctx, f.manager.Actor(), sale, 4, "x")
	wantCode(t, err, "TASK_NOT_COMPLETED")

	started := f.detail(f.manager, sale, 3).StartedAt
	d, err := f.tasks.Reopen(f.Ctx, f.manager.Actor(), sale, 3, "Faltó corregir la serie")
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != tasks.StatusInProgress || d.CompletedAt != nil || d.CompletionPhase != nil || d.StartedAt == nil || !d.StartedAt.Equal(*started) {
		t.Fatalf("a reopened task is in progress with its original start time: %+v", d.Summary)
	}
	if d.DueDate == nil {
		t.Fatalf("the original due date is kept: %+v", d.Summary)
	}
	if f.auditCount(f.taskID(sale, 3), "REOPENED") != 1 {
		t.Fatal("expected a REOPENED audit entry")
	}
	// With task 3 running, nothing else can be reopened or started.
	_, err = f.tasks.Reopen(f.Ctx, f.manager.Actor(), sale, 2, "x")
	wantCode(t, err, "TASK_REOPEN_NOT_ALLOWED")
	wantCode(t, f.start(f.advisor, sale, 4), "TASK_NOT_ELIGIBLE")

	// Completing it again, later, does not queue another e-mail to the customer.
	f.clock.Advance(2 * time.Hour)
	f.FillRequiredDocuments(sale, 3, f.advisor.ID)
	if err := f.complete(f.advisor, sale, 3); err != nil {
		t.Fatal(err)
	}
	if n := f.notifications(sale)["milestone_task_3"]; n != 1 {
		t.Fatalf("milestone_task_3 queued %d times, want 1", n)
	}
}

func TestReopeningTaskThirteenMakesACompletedSaleActiveAgain(t *testing.T) {
	f := newFx(t)
	sale := f.newSale(f.advisor)
	f.runThrough(f.advisor, sale, 13)

	// Only task 13 can be reopened on a completed sale.
	_, err := f.tasks.Reopen(f.Ctx, f.manager.Actor(), sale, 12, "x")
	wantCode(t, err, "TASK_REOPEN_NOT_ALLOWED")
	if _, err := f.tasks.Reopen(f.Ctx, f.manager.Actor(), sale, 13, "Se entregó por error"); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := f.Pool.QueryRow(f.Ctx, `SELECT overall_state::text FROM sale_registers WHERE id = $1`, sale).Scan(&state); err != nil || state != "active" {
		t.Fatalf("state = %s %v", state, err)
	}
	if f.auditCount(sale, "REOPENED") != 1 {
		t.Fatal("expected a REOPENED audit entry for the sale")
	}
	if err := f.complete(f.advisor, sale, 13); err != nil {
		t.Fatalf("completing task 13 again: %v", err)
	}
	if err := f.Pool.QueryRow(f.Ctx, `SELECT overall_state::text FROM sale_registers WHERE id = $1`, sale).Scan(&state); err != nil || state != "completed" {
		t.Fatalf("state = %s %v", state, err)
	}
}

// --- correcting completed tasks ---

func TestACompletedTaskCanBeCorrectedByAManagerWithAReason(t *testing.T) {
	f := newFx(t)
	sale := f.newSale(f.advisor)
	f.runThrough(f.advisor, sale, 3)

	// An advisor cannot; a manager needs a reason.
	if _, err := f.saveWithReason(f.advisor, sale, 3, `{"vehicleSeries":"NUEVA1"}`, "x"); code(err) != "FORBIDDEN" {
		t.Fatalf("an advisor cannot correct a completed task: %v", err)
	}
	_, err := f.save(f.manager, sale, 3, `{"vehicleSeries":"NUEVA1"}`)
	wantCode(t, err, "REASON_REQUIRED")

	before := asMap(t, f.detail(f.manager, sale, 3).Data)["vehicleSeries"]
	d, err := f.saveWithReason(f.manager, sale, 3, `{"vehicleSeries":"NUEVA1"}`, "Error de digitación")
	if err != nil {
		t.Fatal(err)
	}
	if asMap(t, d.Data)["vehicleSeries"] != "NUEVA1" || d.Status != tasks.StatusCompleted {
		t.Fatalf("the correction applies and the task stays completed: %+v", d)
	}
	var summary, reason string
	if err := f.Pool.QueryRow(f.Ctx, `SELECT changes_summary::text, reason FROM audit_logs WHERE entity_id = $1 AND action = 'CORRECTED'`, f.taskID(sale, 3)).Scan(&summary, &reason); err != nil {
		t.Fatalf("expected a CORRECTED audit entry: %v", err)
	}
	if reason != "Error de digitación" || !strings.Contains(summary, "NUEVA1") || !strings.Contains(summary, fmt.Sprint(before)) {
		t.Fatalf("the audit entry holds the reason and the before and after values: %s / %s", reason, summary)
	}
}

func TestACorrectionCannotLeaveATaskIncomplete(t *testing.T) {
	f := newFx(t)
	sale := f.newSale(f.advisor)
	f.runThrough(f.advisor, sale, 3)
	_, err := f.saveWithReason(f.manager, sale, 3, `{"vin":""}`, "Quitar el VIN")
	wantCode(t, err, "TASK_INCOMPLETE")
	if fieldsOf(err)["vin"] != "REQUIRED" {
		t.Fatalf("clearing a mandatory field must be refused: %v", fieldsOf(err))
	}
	if asMap(t, f.detail(f.manager, sale, 3).Data)["vin"] == nil {
		t.Fatal("a refused correction must change nothing")
	}
}

func TestPersonalDataInACorrectionIsNotCopiedIntoTheAudit(t *testing.T) {
	f := newFx(t)
	sale := f.newSale(f.advisor)
	f.runThrough(f.advisor, sale, 2)
	if _, err := f.saveWithReason(f.manager, sale, 2, `{"signerFirstName":"Beatriz"}`, "Nombre mal escrito"); err != nil {
		t.Fatal(err)
	}
	var summary string
	if err := f.Pool.QueryRow(f.Ctx, `SELECT changes_summary::text FROM audit_logs WHERE entity_id = $1 AND action = 'CORRECTED'`, f.taskID(sale, 2)).Scan(&summary); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(summary, "Beatriz") || strings.Contains(summary, "Ana") || !strings.Contains(summary, "signerFirstName") {
		t.Fatalf("the audit entry names the field but not the person's data: %s", summary)
	}
}

func TestFinancialFieldsLockOnceALaterTaskDependingOnThemIsDone(t *testing.T) {
	f := newFx(t)
	sale := f.newSale(f.advisor)
	f.run(f.advisor, sale, 1)

	// Task 1's amounts can still be corrected while no later task is completed.
	if _, err := f.saveWithReason(f.manager, sale, 1, `{"initialToyotaAmount":"30100","initialCommission":"2400"}`, "Cotización corregida"); err != nil {
		t.Fatalf("correcting task 1 before task 2 completes: %v", err)
	}
	f.run(f.advisor, sale, 2)
	_, err := f.saveWithReason(f.manager, sale, 1, `{"initialToyotaAmount":"30200"}`, "Otra vez")
	wantCode(t, err, "TASK_FINANCIAL_FIELD_LOCKED")
	if fieldsOf(err)["initialToyotaAmount"] != "LOCKED" {
		t.Fatalf("the locked field is named: %v", fieldsOf(err))
	}

	// Task 3's final amount locks once task 4 is done; the series stays correctable.
	f.run(f.advisor, sale, 3)
	f.run(f.advisor, sale, 4)
	_, err = f.saveWithReason(f.manager, sale, 3, `{"finalToyotaAmount":"31600"}`, "Ajuste")
	wantCode(t, err, "TASK_FINANCIAL_FIELD_LOCKED")
	if _, err := f.saveWithReason(f.manager, sale, 3, `{"vehicleSeries":"CORREGIDA1"}`, "Serie mal copiada"); err != nil {
		t.Fatalf("a non-financial field of task 3 stays editable: %v", err)
	}

	// The deposits of tasks 4 and 5 lock once task 6 is done.
	f.run(f.advisor, sale, 5)
	f.run(f.advisor, sale, 6)
	_, err = f.saveWithReason(f.manager, sale, 4, `{"depositedAmount":"34000"}`, "Ajuste")
	wantCode(t, err, "TASK_FINANCIAL_FIELD_LOCKED")
	_, err = f.saveWithReason(f.manager, sale, 5, `{"depositedAmount":"30000"}`, "Ajuste")
	wantCode(t, err, "TASK_FINANCIAL_FIELD_LOCKED")
	if _, err := f.saveWithReason(f.manager, sale, 4, `{"transactionNumber":"CORREGIDO1"}`, "Número mal copiado"); err != nil {
		t.Fatalf("the transaction number stays editable: %v", err)
	}
}

// mustSaveThrough saves into the completed task 3 as a manager would, with a reason.
func (f *fx) mustSaveThrough(sale uuid.UUID, n int, data string) {
	f.T.Helper()
	if _, err := f.saveWithReason(f.manager, sale, n, data, "Ajuste de prueba"); err != nil {
		f.T.Fatalf("correct task %d: %v", n, err)
	}
}

// --- frozen sales ---

func TestACancelledSaleFreezesItsTasks(t *testing.T) {
	f := newFx(t)
	sale := f.newSale(f.advisor)
	f.run(f.advisor, sale, 1)
	if err := f.start(f.advisor, sale, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := f.sales.Cancel(f.Ctx, f.manager.Actor(), sale, "customer_withdrew", "El cliente desistió"); err != nil {
		t.Fatal(err)
	}
	wantCode(t, f.complete(f.advisor, sale, 2), "SALE_NOT_ACTIVE")
	wantCode(t, f.start(f.advisor, sale, 3), "SALE_NOT_ACTIVE")
	if _, err := f.save(f.advisor, sale, 2, `{"signerFirstName":"Ana"}`); code(err) != "SALE_NOT_ACTIVE" {
		t.Fatalf("save on a cancelled sale: %v", err)
	}
	if _, err := f.saveWithReason(f.manager, sale, 1, `{"initialCommission":"2600"}`, "x"); code(err) != "SALE_NOT_ACTIVE" {
		t.Fatalf("correct on a cancelled sale: %v", err)
	}
	_, err := f.tasks.Restart(f.Ctx, f.manager.Actor(), sale, 2, "x")
	wantCode(t, err, "SALE_NOT_ACTIVE")
	_, err = f.tasks.Reopen(f.Ctx, f.manager.Actor(), sale, 1, "x")
	wantCode(t, err, "SALE_NOT_ACTIVE")
	// Reading still works, and nothing can be edited.
	d := f.detail(f.advisor, sale, 2)
	if d.Permissions.CanComplete || d.Permissions.CanEditData || d.Permissions.CanStart || d.Permissions.CanSaveNotes {
		t.Fatalf("a frozen sale offers no actions: %+v", d.Permissions)
	}
}

// --- uniqueness across sales ---

func TestIdentifiersMustBeUniqueAcrossActiveAndCompletedSales(t *testing.T) {
	f := newFx(t)
	first := f.newSale(f.advisor)
	f.runThrough(f.advisor, first, 3)
	series := asMap(t, f.detail(f.advisor, first, 3).Data)["vehicleSeries"].(string)
	vin := asMap(t, f.detail(f.advisor, first, 3).Data)["vin"].(string)

	second := f.newSale(f.advisor)
	f.runThrough(f.advisor, second, 2)
	if err := f.start(f.advisor, second, 3); err != nil {
		t.Fatal(err)
	}
	_, err := f.save(f.advisor, second, 3, fmt.Sprintf(`{"vehicleSeries":"%s"}`, series))
	wantCode(t, err, "SERIES_ALREADY_USED")
	if fieldsOf(err)["vehicleSeries"] == "" {
		t.Fatalf("the conflict names the field: %v", fieldsOf(err))
	}
	_, err = f.save(f.advisor, second, 3, fmt.Sprintf(`{"vin":"%s"}`, vin))
	wantCode(t, err, "VIN_ALREADY_USED")
	if fieldsOf(err)["vin"] == "" {
		t.Fatalf("the conflict names the field: %v", fieldsOf(err))
	}

	// A cancelled sale releases its identifiers.
	if _, err := f.sales.Cancel(f.Ctx, f.manager.Actor(), first, "unit_unavailable", "La unidad se vendió a otro cliente"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.save(f.advisor, second, 3, fmt.Sprintf(`{"vehicleSeries":"%s","vin":"%s"}`, series, vin)); err != nil {
		t.Fatalf("a cancelled sale frees its series and VIN: %v", err)
	}
}
