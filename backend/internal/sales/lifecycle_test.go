package sales_test

import (
	"endosoflow/internal/auth"
	"endosoflow/internal/sales"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNotesNeedTheCurrentVersionAndTheRightPerson(t *testing.T) {
	f := newFx(t)
	s := f.newSale(f.advisor)

	_, err := f.sales.UpdateNotes(f.Ctx, f.advisor.Actor(), s.ID, nil, "x")
	wantCode(t, err, "VERSION_REQUIRED")
	v := s.Version
	updated, err := f.sales.UpdateNotes(f.Ctx, f.advisor.Actor(), s.ID, &v, "  Prefiere llamadas  ")
	if err != nil || updated.Notes == nil || *updated.Notes != "Prefiere llamadas" || updated.Version != v+1 {
		t.Fatalf("notes: %v %+v", err, updated)
	}
	_, err = f.sales.UpdateNotes(f.Ctx, f.advisor.Actor(), s.ID, &v, "otra") // stale
	wantCode(t, err, "VERSION_CONFLICT")
	_, err = f.sales.UpdateNotes(f.Ctx, f.other.Actor(), s.ID, &updated.Version, "intruso")
	wantCode(t, err, "SALE_NOT_FOUND")
	cleared, err := f.sales.UpdateNotes(f.Ctx, f.manager.Actor(), s.ID, &updated.Version, "")
	if err != nil || cleared.Notes != nil {
		t.Fatalf("a manager clears the notes: %v %+v", err, cleared.Notes)
	}
	_, err = f.sales.UpdateNotes(f.Ctx, f.manager.Actor(), s.ID, &cleared.Version, strings.Repeat("x", 2001))
	wantCode(t, err, "VALIDATION_FAILED")
}

func TestReassigningMovesTheSaleTheAccessAndTheNotifications(t *testing.T) {
	f := newFx(t)
	s := f.newSale(f.advisor)

	_, err := f.sales.Reassign(f.Ctx, f.advisor.Actor(), s.ID, f.other.ID, "x")
	wantCode(t, err, "FORBIDDEN")
	_, err = f.sales.Reassign(f.Ctx, f.manager.Actor(), s.ID, f.advisor.ID, "x")
	wantCode(t, err, "SALE_ALREADY_ASSIGNED")
	for name, target := range map[string]uuid.UUID{"a manager": f.manager.ID, "nobody": uuid.New()} {
		_, err = f.sales.Reassign(f.Ctx, f.manager.Actor(), s.ID, target, "x")
		if fieldsOf(err)["advisorId"] != "INVALID_ADVISOR" {
			t.Errorf("%s is not a valid advisor: %v", name, err)
		}
	}

	moved, err := f.sales.Reassign(f.Ctx, f.manager.Actor(), s.ID, f.other.ID, "Carga de trabajo")
	if err != nil {
		t.Fatal(err)
	}
	if moved.Advisor.ID != f.other.ID || moved.Advisor.Name == "" {
		t.Fatalf("advisor = %+v", moved.Advisor)
	}
	if f.auditCount(s.ID, "sale_register", "REASSIGNED") != 1 {
		t.Fatal("expected a REASSIGNED audit entry")
	}
	// The access moves with the sale.
	if _, err := f.sales.Get(f.Ctx, f.advisor.Actor(), s.ID); code(err) != "SALE_NOT_FOUND" {
		t.Fatalf("the previous advisor loses access: %v", err)
	}
	if _, err := f.sales.Get(f.Ctx, f.other.Actor(), s.ID); err != nil {
		t.Fatalf("the new advisor gains it: %v", err)
	}
	// The customer, the new advisor and the previous advisor are told.
	n := f.notifications(s.ID)
	if n["customer_advisor_changed"] != 1 || n["sale_unassigned"] != 1 || n["sale_assigned"] != 2 { // the first assignment, then this one
		t.Fatalf("notifications = %v", n)
	}
	// Handing it back and forth notifies every time.
	if _, err := f.sales.Reassign(f.Ctx, f.manager.Actor(), s.ID, f.advisor.ID, ""); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Second)
	if n := f.notifications(s.ID); n["customer_advisor_changed"] != 2 {
		t.Fatalf("every change notifies the customer: %v", n)
	}
	// A cancelled sale stays where it is.
	if _, err := f.sales.Cancel(f.Ctx, f.manager.Actor(), s.ID, "customer_withdrew", "Desistió"); err != nil {
		t.Fatal(err)
	}
	_, err = f.sales.Reassign(f.Ctx, f.manager.Actor(), s.ID, f.other.ID, "x")
	wantCode(t, err, "SALE_NOT_ACTIVE")
}

func TestWhoCanCancelDependsOnTheRoleAndTheCurrentTask(t *testing.T) {
	cases := []struct {
		name      string
		advanceTo int
		actor     func(f *fx) auth.Actor
		allowed   bool
	}{
		{"advisor at task 1", 0, func(f *fx) auth.Actor { return f.advisor.Actor() }, true},
		{"advisor at task 3", 2, func(f *fx) auth.Actor { return f.advisor.Actor() }, true},
		{"advisor at task 4", 3, func(f *fx) auth.Actor { return f.advisor.Actor() }, false},
		{"manager at task 6", 5, func(f *fx) auth.Actor { return f.manager.Actor() }, true},
		{"manager at task 7", 6, func(f *fx) auth.Actor { return f.manager.Actor() }, false},
		{"admin at task 7", 6, func(f *fx) auth.Actor { return f.admin.Actor() }, true},
		{"admin at task 13", 12, func(f *fx) auth.Actor { return f.admin.Actor() }, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFx(t)
			s := f.newSale(f.advisor)
			f.advance(s.ID, f.advisor, c.advanceTo)
			_, err := f.sales.Cancel(f.Ctx, c.actor(f), s.ID, "customer_withdrew", "El cliente desistió")
			if c.allowed && err != nil {
				t.Fatalf("should be allowed: %v", err)
			}
			if !c.allowed {
				wantCode(t, err, "CANCEL_NOT_ALLOWED")
			}
		})
	}
}

func TestCancellingNeedsACategoryAndAReasonAndFreezesTheSale(t *testing.T) {
	f := newFx(t)
	s := f.newSale(f.advisor)
	f.advance(s.ID, f.advisor, 1)
	if _, err := f.tasks.Start(f.Ctx, f.advisor.Actor(), s.ID, 2); err != nil {
		t.Fatal(err)
	}

	_, err := f.sales.Cancel(f.Ctx, f.advisor.Actor(), s.ID, "inventado", "x")
	if fieldsOf(err)["category"] != "INVALID_VALUE" {
		t.Fatalf("category: %v", err)
	}
	_, err = f.sales.Cancel(f.Ctx, f.advisor.Actor(), s.ID, "other", "   ")
	if fieldsOf(err)["reason"] != "REQUIRED" {
		t.Fatalf("reason: %v", err)
	}
	_, err = f.sales.Cancel(f.Ctx, f.other.Actor(), s.ID, "other", "x")
	wantCode(t, err, "SALE_NOT_FOUND")

	cancelled, err := f.sales.Cancel(f.Ctx, f.advisor.Actor(), s.ID, "payment_or_financing_failed", "El banco rechazó el financiamiento")
	if err != nil {
		t.Fatal(err)
	}
	c := cancelled.Cancellation
	if cancelled.State != "cancelled" || c == nil || c.Category != "payment_or_financing_failed" || c.AtTaskNumber != 2 ||
		c.Text != "El banco rechazó el financiamiento" || c.CancelledByName == "" {
		t.Fatalf("cancellation = %+v", cancelled.Cancellation)
	}
	// The frozen state: the in-progress task stays in progress, the permissions shrink, the audit has the reason.
	if cancelled.Tasks[1].Status != "in_progress" || cancelled.Permissions.CanCancel || cancelled.Permissions.CanReassign {
		t.Fatalf("a cancelled sale is frozen: %+v", cancelled.Permissions)
	}
	var reason string
	if err := f.Pool.QueryRow(f.Ctx, `SELECT reason FROM audit_logs WHERE sale_id = $1 AND action = 'CANCELLED'`, s.ID).Scan(&reason); err != nil || reason != "El banco rechazó el financiamiento" {
		t.Fatalf("audit reason = %q %v", reason, err)
	}
	_, err = f.sales.Cancel(f.Ctx, f.manager.Actor(), s.ID, "other", "otra vez")
	wantCode(t, err, "SALE_NOT_ACTIVE")
}

func TestReopeningDependsOnTheRoleAndTheStageItWasCancelledAt(t *testing.T) {
	f := newFx(t)
	early := f.newSale(f.advisor)
	if _, err := f.sales.Cancel(f.Ctx, f.advisor.Actor(), early.ID, "customer_withdrew", "Desistió"); err != nil {
		t.Fatal(err)
	}
	late := f.newSale(f.advisor)
	f.advance(late.ID, f.advisor, 6)
	if _, err := f.sales.Cancel(f.Ctx, f.admin.Actor(), late.ID, "unit_unavailable", "La unidad no llegó"); err != nil {
		t.Fatal(err)
	}

	_, err := f.sales.Reopen(f.Ctx, f.advisor.Actor(), early.ID, "x")
	wantCode(t, err, "REOPEN_NOT_ALLOWED") // advisors never reopen
	_, err = f.sales.Reopen(f.Ctx, f.manager.Actor(), early.ID, " ")
	wantCode(t, err, "REASON_REQUIRED")
	_, err = f.sales.Reopen(f.Ctx, f.manager.Actor(), late.ID, "x")
	wantCode(t, err, "REOPEN_NOT_ALLOWED") // cancelled at task 7: admin only
	if _, err := f.sales.Reopen(f.Ctx, f.admin.Actor(), late.ID, "La unidad llegó"); err != nil {
		t.Fatalf("an admin reopens any: %v", err)
	}
	reopened, err := f.sales.Reopen(f.Ctx, f.manager.Actor(), early.ID, "El cliente volvió")
	if err != nil {
		t.Fatal(err)
	}
	if reopened.State != "active" || reopened.Cancellation != nil {
		t.Fatalf("reopened = %+v", reopened)
	}
	_, err = f.sales.Reopen(f.Ctx, f.manager.Actor(), early.ID, "otra vez")
	wantCode(t, err, "SALE_NOT_CANCELLED")
}

func TestReopeningResumesTheRunningTaskWithTheTimeItHadLeft(t *testing.T) {
	f := newFx(t)
	s := f.newSale(f.advisor)
	f.advance(s.ID, f.advisor, 1)
	if _, err := f.tasks.Start(f.Ctx, f.advisor.Actor(), s.ID, 2); err != nil {
		t.Fatal(err)
	}
	before, _ := f.tasks.Get(f.Ctx, f.advisor.Actor(), s.ID, 2)

	f.clock.Advance(10 * time.Hour)
	if _, err := f.sales.Cancel(f.Ctx, f.manager.Actor(), s.ID, "customer_withdrew", "Desistió"); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(48 * time.Hour) // two days cancelled
	if _, err := f.sales.Reopen(f.Ctx, f.manager.Actor(), s.ID, "Volvió"); err != nil {
		t.Fatal(err)
	}
	after, _ := f.tasks.Get(f.Ctx, f.advisor.Actor(), s.ID, 2)
	if got := after.StartedAt.Sub(*before.StartedAt); (got - 48*time.Hour).Abs() > time.Second {
		t.Fatalf("the start time moves forward by the time spent cancelled: %v", got)
	}
	if got := after.DueDate.Sub(*before.DueDate); (got - 48*time.Hour).Abs() > time.Second {
		t.Fatalf("the due date moves forward by the same amount: %v", got)
	}
	if after.DueDate.Sub(*after.StartedAt) != 72*time.Hour {
		t.Fatal("the task keeps its full 72-hour window")
	}
}

func TestReopeningRefusesASaleWhoseUnitWasSoldMeanwhile(t *testing.T) {
	f := newFx(t)
	first := f.newSale(f.advisor)
	f.advance(first.ID, f.advisor, 3)
	var vin string
	if err := f.Pool.QueryRow(f.Ctx, `SELECT vin FROM sale_registers WHERE id = $1`, first.ID).Scan(&vin); err != nil {
		t.Fatal(err)
	}
	if _, err := f.sales.Cancel(f.Ctx, f.manager.Actor(), first.ID, "customer_withdrew", "Desistió"); err != nil {
		t.Fatal(err)
	}
	// Another sale takes the same VIN while the first one is cancelled.
	second := f.newSale(f.advisor)
	f.advance(second.ID, f.advisor, 2)
	if _, err := f.tasks.Start(f.Ctx, f.advisor.Actor(), second.ID, 3); err != nil {
		t.Fatal(err)
	}
	v := f.taskVersion(f.advisor, second.ID, 3)
	if _, err := f.tasks.SaveData(f.Ctx, f.advisor.Actor(), second.ID, 3, tasksSave(v, fmt.Sprintf(`{"vin":"%s"}`, vin))); err != nil {
		t.Fatalf("a cancelled sale frees its VIN: %v", err)
	}
	_, err := f.sales.Reopen(f.Ctx, f.manager.Actor(), first.ID, "Volvió")
	wantCode(t, err, "VIN_ALREADY_USED")
	var state string
	if err := f.Pool.QueryRow(f.Ctx, `SELECT overall_state::text FROM sale_registers WHERE id = $1`, first.ID).Scan(&state); err != nil || state != "cancelled" {
		t.Fatalf("a refused reopen changes nothing: %s %v", state, err)
	}
}

func TestDeletingASaleIsForAdminsAndOnlyBeforeDelivery(t *testing.T) {
	f := newFx(t)
	s := f.newSale(f.advisor)
	f.advance(s.ID, f.advisor, 2)
	// A file uploaded to the sale: it must leave storage with it.
	var taskID uuid.UUID
	if err := f.Pool.QueryRow(f.Ctx, `SELECT id FROM task_instances WHERE sale_id = $1 AND task_number = 2`, s.ID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	var key string
	if err := f.Pool.QueryRow(f.Ctx, `SELECT storage_key FROM documents WHERE task_instance_id = $1 LIMIT 1`, taskID).Scan(&key); err != nil {
		t.Fatal(err)
	}
	_ = f.store.Put(f.Ctx, key, "application/pdf", []byte("pdf"))

	wantCode(t, f.sales.Delete(f.Ctx, f.manager.Actor(), s.ID, "Prueba", s.Code), "FORBIDDEN")
	wantCode(t, f.sales.Delete(f.Ctx, f.advisor.Actor(), s.ID, "Prueba", s.Code), "FORBIDDEN")
	wantCode(t, f.sales.Delete(f.Ctx, f.other.Actor(), s.ID, "Prueba", s.Code), "SALE_NOT_FOUND")
	wantCode(t, f.sales.Delete(f.Ctx, f.admin.Actor(), s.ID, " ", s.Code), "REASON_REQUIRED")
	err := f.sales.Delete(f.Ctx, f.admin.Actor(), s.ID, "Venta de prueba", "EF-0000-00000")
	if fieldsOf(err)["confirm"] != "CONFIRMATION_MISMATCH" {
		t.Fatalf("the typed confirmation must match: %v", err)
	}

	if err := f.sales.Delete(f.Ctx, f.admin.Actor(), s.ID, "Venta de prueba", strings.ToLower(s.Code)); err != nil {
		t.Fatalf("delete: %v", err)
	}
	var n int
	for table, col := range map[string]string{"sale_registers": "id", "task_instances": "sale_id", "documents": "sale_id", "sale_financials": "sale_id"} {
		if err := f.Pool.QueryRow(f.Ctx, `SELECT count(*) FROM `+table+` WHERE `+col+` = $1`, s.ID).Scan(&n); err != nil || n != 0 {
			t.Fatalf("%s rows left after the deletion: %d %v", table, n, err)
		}
	}
	if _, ok := f.store.Get(key); ok {
		t.Fatal("the sale's files must leave storage")
	}
	if f.auditCount(s.ID, "sale_register", "DELETED") != 1 {
		t.Fatal("the audit trail survives the deletion")
	}
	if _, err := f.sales.Get(f.Ctx, f.admin.Actor(), s.ID); code(err) != "SALE_NOT_FOUND" {
		t.Fatalf("got %v", err)
	}

	// A sale that reached delivery can never be deleted, even if task 13 was reopened.
	done := f.newSale(f.advisor)
	f.advance(done.ID, f.advisor, 13)
	err = f.sales.Delete(f.Ctx, f.admin.Actor(), done.ID, "Prueba", done.Code)
	wantCode(t, err, "SALE_ALREADY_DELIVERED")
	if _, err := f.tasks.Reopen(f.Ctx, f.admin.Actor(), done.ID, 13, "Se entregó por error"); err != nil {
		t.Fatal(err)
	}
	wantCode(t, f.sales.Delete(f.Ctx, f.admin.Actor(), done.ID, "Prueba", done.Code), "SALE_ALREADY_DELIVERED")
}

func TestTheHistoryListsEveryEventNewestFirst(t *testing.T) {
	f := newFx(t)
	s := f.newSale(f.advisor)
	f.advance(s.ID, f.advisor, 2)
	v := s.Version
	if _, err := f.sales.UpdateNotes(f.Ctx, f.advisor.Actor(), s.ID, &v, "nota"); err != nil {
		t.Fatal(err)
	}

	h, err := f.sales.History(f.Ctx, f.advisor.Actor(), s.ID, 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	// 1 sale created + notes + 2 tasks x (started + updated + completed) = more than 7 events.
	if h.Total < 8 || len(h.Items) != h.Total {
		t.Fatalf("history = %d events (total %d)", len(h.Items), h.Total)
	}
	for i := 1; i < len(h.Items); i++ {
		if h.Items[i].At.After(h.Items[i-1].At) {
			t.Fatal("newest first")
		}
	}
	last := h.Items[len(h.Items)-1]
	if last.Action != "CREATED" || last.EntityType != "sale_register" || last.ActorName == "" {
		t.Fatalf("the oldest event is the creation: %+v", last)
	}
	var sawTask bool
	for _, e := range h.Items {
		if e.EntityType == "task_instance" && e.TaskNumber != nil && *e.TaskNumber == 2 && e.Action == "COMPLETED" {
			sawTask = true
		}
	}
	if !sawTask {
		t.Fatal("task events carry their task number")
	}
	page := func(n, size int) *sales.History {
		h, err := f.sales.History(f.Ctx, f.manager.Actor(), s.ID, n, size)
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	if p := page(2, 3); len(p.Items) != 3 || p.Page != 2 || p.Total != h.Total || p.Items[0].ID != h.Items[3].ID {
		t.Fatalf("pagination: %+v", p)
	}
}
