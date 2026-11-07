package sales

import (
	"encoding/json"
	"endosoflow/internal/documents"
	"endosoflow/internal/tasks"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func dataMap(t *testing.T, d *tasks.Detail) map[string]any {
	t.Helper()
	raw, err := json.Marshal(d.Data)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// --- creation ---

func TestSaleCreationSnapshotsTheCustomerAndTheVehicle(t *testing.T) {
	e := newFlowEnv(t)
	cust, cat := e.newCustomer(), e.newCatalog()
	sale := e.newSale(e.advisor, cust, cat)

	if sale.CustomerID != cust.ID || sale.CustomerName != cust.DisplayName || sale.CustomerType != "diplomatico" {
		t.Errorf("customer snapshot: %+v", sale.Sale)
	}
	if sale.ModelID != cat.ModelID || sale.VersionName != "SR" || sale.ColorName != "Blanco" || sale.ReferencePrice != "35000.00" {
		t.Errorf("vehicle snapshot: %+v", sale.Sale)
	}
	if sale.State != StateActive || sale.AdvisorID != e.advisor.ID || sale.Temperature != "hot" {
		t.Errorf("state: %+v", sale.Sale)
	}
	if sale.CurrentTaskNumber == nil || *sale.CurrentTaskNumber != 1 || sale.CompletedTasks != 0 {
		t.Errorf("progress: %+v", sale.Sale)
	}
	if len(sale.Milestones) != 6 || sale.Milestones[0].State != "in_progress" || sale.Milestones[5].State != "pending" {
		t.Errorf("milestones: %+v", sale.Milestones)
	}

	list, err := e.tasks.List(e.ctx, e.advisor.actor(), sale.ID)
	if err != nil || len(list) != 13 {
		t.Fatalf("tasks: %d, %v", len(list), err)
	}
	for i, task := range list {
		if task.Status != tasks.StatusPending || task.Number != i+1 || task.Eligible != (i == 0) {
			t.Errorf("task %d: %+v", i+1, task)
		}
	}
	if n := e.countAudit(sale.ID, "sale_register", "CREATED"); n != 1 {
		t.Errorf("CREATED events = %d", n)
	}

	// A later catalog change never touches the snapshot.
	if _, err := e.pool.Exec(e.ctx, `UPDATE vehicle_versions SET reference_price = 99999 WHERE id = $1`, cat.VersionID); err != nil {
		t.Fatal(err)
	}
	again, err := e.sales.Get(e.ctx, e.advisor.actor(), sale.ID)
	if err != nil || again.ReferencePrice != "35000.00" {
		t.Errorf("snapshot changed: %v, %v", again, err)
	}
}

func TestSaleCreationRules(t *testing.T) {
	e := newFlowEnv(t)
	cust, cat := e.newCustomer(), e.newCatalog()
	base := CreateInput{CustomerID: cust.ID, VersionID: cat.VersionID, ColorID: cat.ColorID, Temperature: "warm"}

	t.Run("a manager must choose the advisor", func(t *testing.T) {
		_, err := e.sales.Create(e.ctx, e.manager.actor(), base)
		wantCode(t, err, "VALIDATION_FAILED")
		if fieldCodes(err)["assignedAdvisorId"] != "REQUIRED" {
			t.Errorf("fields = %v", fieldCodes(err))
		}
	})
	t.Run("an advisor cannot create a sale for someone else", func(t *testing.T) {
		in := base
		in.AssignedAdvisorID = &e.other.ID
		_, err := e.sales.Create(e.ctx, e.advisor.actor(), in)
		wantCode(t, err, "ADVISOR_MISMATCH")
	})
	t.Run("a manager assigns to another advisor", func(t *testing.T) {
		in := base
		in.AssignedAdvisorID = &e.other.ID
		sale, err := e.sales.Create(e.ctx, e.manager.actor(), in)
		if err != nil {
			t.Fatal(err)
		}
		e.trackSale(sale.ID)
		if sale.AdvisorID != e.other.ID {
			t.Errorf("advisor = %v", sale.AdvisorID)
		}
	})
	t.Run("a manager cannot assign a manager", func(t *testing.T) {
		in := base
		in.AssignedAdvisorID = &e.manager.ID
		_, err := e.sales.Create(e.ctx, e.manager.actor(), in)
		wantCode(t, err, "INVALID_ADVISOR")
	})
	t.Run("invalid temperature and missing ids", func(t *testing.T) {
		_, err := e.sales.Create(e.ctx, e.advisor.actor(), CreateInput{Temperature: "tepid"})
		wantCode(t, err, "VALIDATION_FAILED")
		fields := fieldCodes(err)
		for _, f := range []string{"customerId", "versionId", "colorId", "temperature"} {
			if fields[f] == "" {
				t.Errorf("field %s missing from %v", f, fields)
			}
		}
	})
	t.Run("unknown customer", func(t *testing.T) {
		in := base
		in.CustomerID = uuid.New()
		_, err := e.sales.Create(e.ctx, e.advisor.actor(), in)
		wantCode(t, err, "CUSTOMER_NOT_FOUND")
	})
	t.Run("inactive customer", func(t *testing.T) {
		inactive := e.newCustomer()
		if _, err := e.pool.Exec(e.ctx, `UPDATE customers SET is_active = false, deactivated_at = now(), deactivated_by = $2 WHERE id = $1`,
			inactive.ID, e.admin.ID); err != nil {
			t.Fatal(err)
		}
		in := base
		in.CustomerID = inactive.ID
		_, err := e.sales.Create(e.ctx, e.advisor.actor(), in)
		wantCode(t, err, "CUSTOMER_INACTIVE")
	})
	t.Run("version that is no longer selectable", func(t *testing.T) {
		archived := e.newCatalog()
		e.replica(func(c *pgxpool.Conn) {
			_, _ = c.Exec(e.ctx, `UPDATE vehicle_versions SET is_active = false, deactivated_at = now(), deactivated_by = $2 WHERE id = $1`,
				archived.VersionID, e.admin.ID)
		})
		in := base
		in.VersionID, in.ColorID = archived.VersionID, archived.ColorID
		_, err := e.sales.Create(e.ctx, e.advisor.actor(), in)
		wantCode(t, err, "VEHICLE_NOT_SELECTABLE")
	})
	t.Run("color of another model", func(t *testing.T) {
		other := e.newCatalog()
		in := base
		in.ColorID = other.ColorID
		_, err := e.sales.Create(e.ctx, e.advisor.actor(), in)
		wantCode(t, err, "VEHICLE_COLOR_MISMATCH")
	})
	t.Run("scope", func(t *testing.T) {
		sale := e.newSale(e.advisor, cust, cat)
		_, err := e.sales.Get(e.ctx, e.other.actor(), sale.ID)
		wantCode(t, err, "SALE_NOT_FOUND")
		if _, err := e.sales.Get(e.ctx, e.manager.actor(), sale.ID); err != nil {
			t.Errorf("a manager sees every sale: %v", err)
		}
		_, err = e.tasks.Get(e.ctx, e.other.actor(), sale.ID, 1)
		wantCode(t, err, "SALE_NOT_FOUND")
		_, err = e.docs.List(e.ctx, e.other.actor(), sale.ID)
		wantCode(t, err, "SALE_NOT_FOUND")
	})
}

// --- the whole process ---

func TestAFullSaleRunsAllThirteenTasks(t *testing.T) {
	e := newFlowEnv(t)
	sale, _, _ := e.freshSale()

	e.runThrough(sale.ID, 12)
	mid, err := e.sales.Get(e.ctx, e.advisor.actor(), sale.ID)
	if err != nil || mid.State != StateActive || mid.CompletedTasks != 12 || mid.CurrentTaskNumber == nil || *mid.CurrentTaskNumber != 13 {
		t.Fatalf("after 12 tasks: %+v, %v", mid, err)
	}
	if mid.VehicleSeries == nil || mid.Vin == nil || mid.LicensePlate == nil {
		t.Errorf("identifiers were not copied to the sale: %+v", mid.Sale)
	}

	e.runTask(sale.ID, 13, nil)
	done, err := e.sales.Get(e.ctx, e.advisor.actor(), sale.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done.State != StateCompleted || done.CompletedTasks != 13 || done.CurrentTaskNumber != nil || done.CompletedAt == nil {
		t.Fatalf("completed sale: %+v", done.Sale)
	}
	for _, m := range done.Milestones {
		if m.State != "completed" {
			t.Errorf("milestone %d is %s", m.Number, m.State)
		}
	}

	list, err := e.tasks.List(e.ctx, e.advisor.actor(), sale.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range list {
		if task.Status != tasks.StatusCompleted || task.CompletionPhase == nil || task.CompletedAt == nil {
			t.Errorf("task %d: %+v", task.Number, task)
		}
	}
	if *list[0].CompletionPhase != tasks.PhaseNotApplicable || *list[1].CompletionPhase != tasks.PhaseEarly {
		t.Errorf("phases: task 1 %s, task 2 %s", *list[0].CompletionPhase, *list[1].CompletionPhase)
	}

	t1, t3, t4 := dataMap(t, e.task(e.advisor, sale.ID, 1)), dataMap(t, e.task(e.advisor, sale.ID, 3)), dataMap(t, e.task(e.advisor, sale.ID, 4))
	if t1["quotedFinalPrice"] != "35000.00" || t3["finalCommission"] != "3000.00" {
		t.Errorf("calculated values: %v %v", t1["quotedFinalPrice"], t3["finalCommission"])
	}
	if t4["expectedAmount"] != "35000.00" || t4["depositedAmount"] != "35000.00" || t4["mismatchReason"] != nil {
		t.Errorf("task 4: %v", t4)
	}

	if n := e.countAudit(sale.ID, "task_instance", "COMPLETED"); n != 13 {
		t.Errorf("task COMPLETED events = %d", n)
	}
	if n := e.countAudit(sale.ID, "task_instance", "STARTED"); n != 13 {
		t.Errorf("task STARTED events = %d", n)
	}
	if n := e.countAudit(sale.ID, "sale_register", "COMPLETED"); n != 1 {
		t.Errorf("sale COMPLETED events = %d", n)
	}
	if n := e.count(`SELECT count(*) FROM documents WHERE sale_id = $1`, sale.ID); n < 14 {
		t.Errorf("documents = %d", n)
	}

	history, err := e.sales.History(e.ctx, e.advisor.actor(), sale.ID, 1, 100)
	if err != nil || history.Total < 40 || len(history.Items) == 0 {
		t.Fatalf("history: %+v, %v", history, err)
	}
	first, last := history.Items[0], history.Items[len(history.Items)-1]
	if first.CreatedAt.Before(last.CreatedAt) {
		t.Error("history must list the newest event first")
	}

	// A completed sale is read-only.
	_, err = e.start(e.advisor, sale.ID, 13)
	wantCode(t, err, "SALE_NOT_ACTIVE")
}

func TestCompletionReportsEveryProblemAtOnce(t *testing.T) {
	e := newFlowEnv(t)
	sale, _, _ := e.freshSale()
	if _, err := e.start(e.advisor, sale.ID, 1); err != nil {
		t.Fatal(err)
	}

	_, err := e.complete(e.advisor, sale.ID, 1)
	wantCode(t, err, "VALIDATION_FAILED")
	fields := fieldCodes(err)
	for _, f := range []string{"initialToyotaAmount", "initialCommission", "accessoryAmount", "releaseProcessingAmount"} {
		if fields[f] != "REQUIRED" {
			t.Errorf("%s: %v", f, fields)
		}
	}
	if fields["documents.quotation_pdf"] != "DOCUMENT_REQUIRED" || fields["documents.quotation_excel"] != "DOCUMENT_REQUIRED" {
		t.Errorf("documents: %v", fields)
	}

	e.mustSave(e.advisor, sale.ID, 1, e.dataFor(1))
	_, err = e.complete(e.advisor, sale.ID, 1)
	fields = fieldCodes(err)
	if len(fields) != 2 || fields["documents.quotation_pdf"] == "" {
		t.Errorf("only documents should be missing: %v", fields)
	}

	if _, _, err := e.upload(e.advisor, sale.ID, 1, "quotation_pdf", pdfBytes(), ""); err != nil {
		t.Fatal(err)
	}
	_, err = e.complete(e.advisor, sale.ID, 1)
	if fields = fieldCodes(err); len(fields) != 1 || fields["documents.quotation_excel"] == "" {
		t.Errorf("the Excel file should be the only thing missing: %v", fields)
	}

	if _, _, err := e.upload(e.advisor, sale.ID, 1, "quotation_excel", xlsxBytes(t), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := e.complete(e.advisor, sale.ID, 1); err != nil {
		t.Fatalf("complete: %v", err)
	}
}

func TestSettlementAndPaymentRules(t *testing.T) {
	e := newFlowEnv(t)
	sale, _, _ := e.freshSale()
	e.runThrough(sale.ID, 2)
	if _, err := e.start(e.advisor, sale.ID, 3); err != nil {
		t.Fatal(err)
	}

	t.Run("the final commission must stay positive", func(t *testing.T) {
		for _, amount := range []string{"34500", "33000"} {
			_, err := e.saveData(e.advisor, sale.ID, 3, map[string]any{"finalToyotaAmount": amount}, "")
			wantCode(t, err, "VALIDATION_FAILED")
			if fieldCodes(err)["finalToyotaAmount"] != "COMMISSION_NOT_POSITIVE" {
				t.Errorf("%s: %v", amount, fieldCodes(err))
			}
		}
		r := e.mustSave(e.advisor, sale.ID, 3, map[string]any{"finalToyotaAmount": "32999.99"})
		if dataMap(t, r.Task)["finalCommission"] != "0.01" {
			t.Errorf("data = %v", dataMap(t, r.Task))
		}
	})
	t.Run("identifiers are validated and normalized", func(t *testing.T) {
		_, err := e.saveData(e.advisor, sale.ID, 3, map[string]any{"vin": "JTDKB20U97312345I"}, "")
		if fieldCodes(err)["vin"] != "VIN_INVALID" {
			t.Errorf("fields = %v", fieldCodes(err))
		}
		r := e.mustSave(e.advisor, sale.ID, 3, map[string]any{"vin": " jtdkb20u973123456 ", "vehicleSeries": " ser 77 "})
		d := dataMap(t, r.Task)
		if d["vin"] != "JTDKB20U973123456" || d["vehicleSeries"] != "SER77" {
			t.Errorf("data = %v", d)
		}
	})
	t.Run("a VIN cannot be used by two active sales", func(t *testing.T) {
		vin := randVIN()
		e.mustSave(e.advisor, sale.ID, 3, map[string]any{"vin": vin})
		second, _, _ := e.freshSale()
		e.runThrough(second.ID, 2)
		if _, err := e.start(e.advisor, second.ID, 3); err != nil {
			t.Fatal(err)
		}
		_, err := e.saveData(e.advisor, second.ID, 3, map[string]any{"vin": vin}, "")
		wantCode(t, err, "VIN_ALREADY_USED")
	})

	e.mustSave(e.advisor, sale.ID, 3, map[string]any{"finalToyotaAmount": "30000"})
	e.uploadRequired(e.advisor, sale.ID, 3)
	if _, err := e.complete(e.advisor, sale.ID, 3); err != nil {
		t.Fatal(err)
	}

	if _, err := e.start(e.advisor, sale.ID, 4); err != nil {
		t.Fatal(err)
	}
	t.Run("a deposit that differs needs its reason in the same save", func(t *testing.T) {
		_, err := e.saveData(e.advisor, sale.ID, 4, map[string]any{"depositedAmount": "34000"}, "")
		wantCode(t, err, "VALIDATION_FAILED")
		if fieldCodes(err)["mismatchReason"] != "MISMATCH_REASON_REQUIRED" {
			t.Errorf("fields = %v", fieldCodes(err))
		}
		r := e.mustSave(e.advisor, sale.ID, 4, map[string]any{"depositedAmount": "34000", "mismatchReason": "El banco cobró una comisión"})
		d := dataMap(t, r.Task)
		if d["expectedAmount"] != "35000.00" || d["depositedAmount"] != "34000.00" || d["mismatchReason"] == nil {
			t.Errorf("data = %v", d)
		}
	})
	t.Run("an exact deposit drops a stale reason", func(t *testing.T) {
		r := e.mustSave(e.advisor, sale.ID, 4, map[string]any{"depositedAmount": "35000"})
		if d := dataMap(t, r.Task); d["mismatchReason"] != nil {
			t.Errorf("data = %v", d)
		}
	})
	t.Run("amounts and dates are validated", func(t *testing.T) {
		_, err := e.saveData(e.advisor, sale.ID, 4, map[string]any{"depositedAmount": "-5", "paymentDate": "2026-02-30", "bank": "banco"}, "")
		fields := fieldCodes(err)
		if fields["depositedAmount"] != "INVALID_AMOUNT" || fields["paymentDate"] != "INVALID_DATE" || fields["bank"] != "INVALID_VALUE" {
			t.Errorf("fields = %v", fields)
		}
	})
	t.Run("a date before the sale is only a warning", func(t *testing.T) {
		old := e.clock.Now().AddDate(0, 0, -40).Format("2006-01-02")
		r := e.mustSave(e.advisor, sale.ID, 4, map[string]any{"paymentDate": old})
		if len(r.Warnings) != 1 || r.Warnings[0].Code != "DATE_BEFORE_SALE" {
			t.Errorf("warnings = %+v", r.Warnings)
		}
	})
}

func TestInvoiceRules(t *testing.T) {
	e := newFlowEnv(t)
	sale, _, _ := e.freshSale()
	e.runThrough(sale.ID, 5)
	if _, err := e.start(e.advisor, sale.ID, 6); err != nil {
		t.Fatal(err)
	}

	_, err := e.saveData(e.advisor, sale.ID, 6, map[string]any{"toyota": map[string]any{"number": "F1-1"}}, "")
	if fieldCodes(err)["toyota.number"] != "INVOICE_NUMBER_INVALID" {
		t.Errorf("fields = %v", fieldCodes(err))
	}
	_, err = e.saveData(e.advisor, sale.ID, 6, map[string]any{"toyota": map[string]any{"amount": "29999"}}, "")
	if fieldCodes(err)["toyota.mismatchReason"] != "MISMATCH_REASON_REQUIRED" {
		t.Errorf("fields = %v", fieldCodes(err))
	}

	data := e.dataFor(6)
	r := e.mustSave(e.advisor, sale.ID, 6, data)
	d := dataMap(t, r.Task)
	toyota := d["toyota"].(map[string]any)
	commission := d["commission"].(map[string]any)
	if toyota["expectedAmount"] != "30000.00" || commission["expectedAmount"] != "3000.00" {
		t.Errorf("expected amounts: %v %v", toyota, commission)
	}

	// Commission, accessories and release-processing numbers share one namespace.
	number := commission["number"].(string)
	_, err = e.saveData(e.advisor, sale.ID, 6, map[string]any{"accessories": map[string]any{"number": number}}, "")
	wantCode(t, err, "INVOICE_NUMBER_ALREADY_USED")
}

// --- state machine ---

func TestTaskStateRules(t *testing.T) {
	e := newFlowEnv(t)
	sale, _, _ := e.freshSale()

	_, err := e.start(e.advisor, sale.ID, 2)
	wantCode(t, err, "TASK_NOT_ELIGIBLE")
	_, err = e.start(e.advisor, sale.ID, 14)
	wantCode(t, err, "TASK_NOT_FOUND")
	_, err = e.start(e.other, sale.ID, 1)
	wantCode(t, err, "SALE_NOT_FOUND")

	if _, err := e.start(e.advisor, sale.ID, 1); err != nil {
		t.Fatal(err)
	}
	_, err = e.start(e.advisor, sale.ID, 1)
	wantCode(t, err, "TASK_INVALID_STATE")
	_, err = e.complete(e.advisor, sale.ID, 2)
	wantCode(t, err, "TASK_INVALID_STATE")

	_, err = e.saveData(e.advisor, sale.ID, 2, map[string]any{"pepStatus": "no_pep"}, "")
	wantCode(t, err, "TASK_NOT_IN_PROGRESS")
	_, err = e.saveData(e.other, sale.ID, 1, map[string]any{"accessoryAmount": "1"}, "")
	wantCode(t, err, "SALE_NOT_FOUND")

	t.Run("versions", func(t *testing.T) {
		old := e.task(e.advisor, sale.ID, 1).Version
		e.mustSave(e.advisor, sale.ID, 1, map[string]any{"accessoryAmount": "1000"})
		stale := old
		raw, _ := json.Marshal(map[string]any{"accessoryAmount": "1100"})
		_, err := e.tasks.SaveData(e.ctx, e.advisor.actor(), sale.ID, 1, tasks.SaveInput{Version: &stale, Data: raw})
		wantCode(t, err, "VERSION_CONFLICT")
		_, err = e.tasks.SaveData(e.ctx, e.advisor.actor(), sale.ID, 1, tasks.SaveInput{Data: raw})
		wantCode(t, err, "VERSION_REQUIRED")
	})
	t.Run("unknown and calculated fields", func(t *testing.T) {
		_, err := e.saveData(e.advisor, sale.ID, 1, map[string]any{"nope": 1}, "")
		wantCode(t, err, "INVALID_JSON")
		_, err = e.saveData(e.advisor, sale.ID, 1, map[string]any{"quotedFinalPrice": "1"}, "")
		wantCode(t, err, "INVALID_JSON")
	})
	t.Run("notes", func(t *testing.T) {
		if _, err := e.tasks.SaveNotes(e.ctx, e.advisor.actor(), sale.ID, 1, "  llamar al cliente  "); err != nil {
			t.Fatal(err)
		}
		if d := e.task(e.advisor, sale.ID, 1); d.Notes == nil || *d.Notes != "llamar al cliente" {
			t.Errorf("notes = %v", d.Notes)
		}
		_, err := e.tasks.SaveNotes(e.ctx, e.advisor.actor(), sale.ID, 2, "nota")
		wantCode(t, err, "TASK_NOT_IN_PROGRESS")
		if _, err := e.tasks.SaveNotes(e.ctx, e.manager.actor(), sale.ID, 2, "nota de gerencia"); err != nil {
			t.Errorf("a manager can write notes on any task: %v", err)
		}
	})
}

func TestRestartReopenAndCompletionPhases(t *testing.T) {
	e := newFlowEnv(t)
	sale, _, _ := e.freshSale()
	e.runTask(sale.ID, 1, nil)

	t0 := e.clock.Now()
	d, err := e.start(e.advisor, sale.ID, 2)
	if err != nil || d.DueDate == nil || !d.DueDate.Equal(t0.Add(72*time.Hour)) {
		t.Fatalf("start: %+v, %v", d, err)
	}

	_, err = e.tasks.Restart(e.ctx, e.advisor.actor(), sale.ID, 2, "x")
	wantCode(t, err, "FORBIDDEN")
	_, err = e.tasks.Restart(e.ctx, e.manager.actor(), sale.ID, 2, "   ")
	wantCode(t, err, "REASON_REQUIRED")
	r, err := e.tasks.Restart(e.ctx, e.manager.actor(), sale.ID, 2, "se abrió por error")
	if err != nil || r.Status != tasks.StatusPending || r.StartedAt != nil || r.DueDate != nil {
		t.Fatalf("restart: %+v, %v", r, err)
	}
	if n := e.countAudit(sale.ID, "task_instance", "RESTARTED"); n != 1 {
		t.Errorf("RESTARTED events = %d", n)
	}

	e.clock.Advance(2 * time.Hour)
	t1 := e.clock.Now()
	d, err = e.start(e.advisor, sale.ID, 2)
	if err != nil || !d.DueDate.Equal(t1.Add(72*time.Hour)) {
		t.Fatalf("second start: %+v, %v", d, err)
	}
	e.mustSave(e.advisor, sale.ID, 2, e.dataFor(2))
	e.uploadRequired(e.advisor, sale.ID, 2)
	e.clock.Set(t1.Add(10 * time.Hour))
	done, err := e.complete(e.advisor, sale.ID, 2)
	if err != nil || done.CompletionPhase == nil || *done.CompletionPhase != tasks.PhaseEarly {
		t.Fatalf("complete early: %+v, %v", done, err)
	}

	t.Run("reopen rules", func(t *testing.T) {
		_, err := e.tasks.Reopen(e.ctx, e.advisor.actor(), sale.ID, 2, "x")
		wantCode(t, err, "FORBIDDEN")
		_, err = e.tasks.Reopen(e.ctx, e.manager.actor(), sale.ID, 2, "")
		wantCode(t, err, "REASON_REQUIRED")
		_, err = e.tasks.Reopen(e.ctx, e.manager.actor(), sale.ID, 1, "no es la última")
		wantCode(t, err, "TASK_NOT_LAST_COMPLETED")

		if _, err := e.start(e.advisor, sale.ID, 3); err != nil {
			t.Fatal(err)
		}
		_, err = e.tasks.Reopen(e.ctx, e.manager.actor(), sale.ID, 2, "hay otra tarea en curso")
		wantCode(t, err, "TASK_ALREADY_IN_PROGRESS")
		if _, err := e.tasks.Restart(e.ctx, e.manager.actor(), sale.ID, 3, "volver atrás"); err != nil {
			t.Fatal(err)
		}
	})

	phaseAfterReopen := func(at time.Duration, want string) {
		t.Helper()
		e.clock.Set(t1.Add(11 * time.Hour))
		re, err := e.tasks.Reopen(e.ctx, e.manager.actor(), sale.ID, 2, "corregir datos")
		if err != nil {
			t.Fatal(err)
		}
		if re.Status != tasks.StatusInProgress || re.CompletedAt != nil || re.CompletionPhase != nil ||
			re.DueDate == nil || !re.DueDate.Equal(t1.Add(72*time.Hour)) || re.StartedAt == nil || !re.StartedAt.Equal(t1) {
			t.Fatalf("a reopened task keeps its start and due date: %+v", re)
		}
		e.clock.Set(t1.Add(at))
		c, err := e.complete(e.advisor, sale.ID, 2)
		if err != nil || c.CompletionPhase == nil || *c.CompletionPhase != want {
			t.Fatalf("complete at %s: %+v, %v (want %s)", at, c, err, want)
		}
	}
	phaseAfterReopen(60*time.Hour, tasks.PhaseInApproaching)
	phaseAfterReopen(80*time.Hour, tasks.PhaseOverdue)
	phaseAfterReopen(90*time.Hour, tasks.PhaseRecurringOverdue)
}

func TestCorrectingACompletedTaskAndTheFinancialLock(t *testing.T) {
	e := newFlowEnv(t)
	sale, _, _ := e.freshSale()
	e.runTask(sale.ID, 1, nil)

	t.Run("who and how", func(t *testing.T) {
		_, err := e.saveData(e.advisor, sale.ID, 1, map[string]any{"accessoryAmount": "1200"}, "motivo")
		wantCode(t, err, "FORBIDDEN")
		_, err = e.saveData(e.manager, sale.ID, 1, map[string]any{"accessoryAmount": "1200"}, "")
		wantCode(t, err, "REASON_REQUIRED")

		r, err := e.saveData(e.manager, sale.ID, 1, map[string]any{"accessoryAmount": "1200"}, "el cliente cambió de accesorio")
		if err != nil {
			t.Fatal(err)
		}
		if dataMap(t, r.Task)["quotedFinalPrice"] != "35200.00" {
			t.Errorf("data = %v", dataMap(t, r.Task))
		}
		if n := e.countAudit(sale.ID, "task_instance", "CORRECTED"); n != 1 {
			t.Errorf("CORRECTED events = %d", n)
		}
		var reason *string
		if err := e.pool.QueryRow(e.ctx, `SELECT reason FROM audit_logs WHERE sale_id = $1 AND action = 'CORRECTED'`, sale.ID).Scan(&reason); err != nil || reason == nil || !strings.Contains(*reason, "accesorio") {
			t.Errorf("reason = %v, %v", reason, err)
		}

		// Saving the same value again changes nothing and writes no event.
		if _, err := e.saveData(e.manager, sale.ID, 1, map[string]any{"accessoryAmount": "1200"}, "sin cambios"); err != nil {
			t.Fatal(err)
		}
		if n := e.countAudit(sale.ID, "task_instance", "CORRECTED"); n != 1 {
			t.Errorf("an unchanged save wrote an event: %d", n)
		}
		if _, err := e.saveData(e.manager, sale.ID, 1, map[string]any{"accessoryAmount": "1000"}, "se revirtió el cambio"); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("a completed task must stay complete", func(t *testing.T) {
		_, err := e.saveData(e.manager, sale.ID, 1, map[string]any{"initialCommission": nil}, "borrar")
		wantCode(t, err, "VALIDATION_FAILED")
		if fieldCodes(err)["initialCommission"] != "REQUIRED" {
			t.Errorf("fields = %v", fieldCodes(err))
		}
		if dataMap(t, e.task(e.manager, sale.ID, 1))["initialCommission"] != "3000.00" {
			t.Error("a refused correction must leave the data as it was")
		}
	})

	e.runTask(sale.ID, 2, nil)
	t.Run("task 1 amounts lock when task 2 is completed", func(t *testing.T) {
		_, err := e.saveData(e.manager, sale.ID, 1, map[string]any{"initialCommission": "3500"}, "corregir")
		wantCode(t, err, "FIELD_LOCKED")
		if fieldCodes(err)["initialCommission"] != "LOCKED" {
			t.Errorf("fields = %v", fieldCodes(err))
		}
		// Reopening in reverse order frees them again.
		if _, err := e.tasks.Reopen(e.ctx, e.manager.actor(), sale.ID, 2, "corregir la cotización"); err != nil {
			t.Fatal(err)
		}
		if _, err := e.saveData(e.manager, sale.ID, 1, map[string]any{"initialCommission": "3100"}, "ajuste"); err != nil {
			t.Fatalf("after the reopen: %v", err)
		}
		if _, err := e.saveData(e.manager, sale.ID, 1, map[string]any{"initialCommission": "3000"}, "se revirtió"); err != nil {
			t.Fatal(err)
		}
		if _, err := e.complete(e.advisor, sale.ID, 2); err != nil {
			t.Fatal(err)
		}
	})

	e.runTask(sale.ID, 3, nil)
	e.runTask(sale.ID, 4, nil)
	t.Run("task 3 locks its amount at task 4 but not its identifiers", func(t *testing.T) {
		_, err := e.saveData(e.manager, sale.ID, 3, map[string]any{"finalToyotaAmount": "29000"}, "corregir")
		wantCode(t, err, "FIELD_LOCKED")
		vin := randVIN()
		r, err := e.saveData(e.manager, sale.ID, 3, map[string]any{"vin": vin}, "error de digitación del VIN")
		if err != nil || dataMap(t, r.Task)["vin"] != vin {
			t.Fatalf("a VIN correction: %v, %v", r, err)
		}
		if sale2, _ := e.sales.Get(e.ctx, e.manager.actor(), sale.ID); sale2 == nil || sale2.Vin == nil || *sale2.Vin != vin {
			t.Error("the corrected VIN must reach the sale")
		}
	})
}

// --- cancellation and reopening of a sale ---

func TestCancellingAndReopeningASale(t *testing.T) {
	e := newFlowEnv(t)
	sale, _, _ := e.freshSale()
	e.runTask(sale.ID, 1, nil)
	t0 := e.clock.Now()
	started, err := e.start(e.advisor, sale.ID, 2)
	if err != nil {
		t.Fatal(err)
	}

	_, err = e.sales.Cancel(e.ctx, e.advisor.actor(), sale.ID, CancelInput{Category: "weather"})
	wantCode(t, err, "VALIDATION_FAILED")
	_, err = e.sales.Cancel(e.ctx, e.advisor.actor(), sale.ID, CancelInput{Category: "other"})
	wantCode(t, err, "VALIDATION_FAILED")
	_, err = e.sales.Cancel(e.ctx, e.other.actor(), sale.ID, CancelInput{Category: "customer_withdrew"})
	wantCode(t, err, "SALE_NOT_FOUND")

	e.clock.Advance(time.Hour)
	cancelled, err := e.sales.Cancel(e.ctx, e.advisor.actor(), sale.ID, CancelInput{Category: "customer_withdrew", Text: "se fue a otra marca"})
	if err != nil || cancelled.State != StateCancelled || cancelled.CancelledAtTask == nil || *cancelled.CancelledAtTask != 2 ||
		cancelled.CancellationCategory == nil || *cancelled.CancellationCategory != "customer_withdrew" {
		t.Fatalf("cancel: %+v, %v", cancelled, err)
	}

	t.Run("a cancelled sale is frozen", func(t *testing.T) {
		_, err := e.saveData(e.advisor, sale.ID, 2, map[string]any{"pepStatus": "no_pep"}, "")
		wantCode(t, err, "SALE_NOT_ACTIVE")
		_, err = e.start(e.advisor, sale.ID, 3)
		wantCode(t, err, "SALE_NOT_ACTIVE")
		_, err = e.complete(e.advisor, sale.ID, 2)
		wantCode(t, err, "SALE_NOT_ACTIVE")
		_, _, err = e.upload(e.advisor, sale.ID, 2, "signer_identity_scan", pdfBytes(), "")
		wantCode(t, err, "SALE_NOT_ACTIVE")
		_, err = e.sales.Reassign(e.ctx, e.manager.actor(), sale.ID, ReassignInput{AdvisorID: e.other.ID})
		wantCode(t, err, "SALE_NOT_ACTIVE")
		version := cancelled.Version
		_, err = e.sales.UpdateNotes(e.ctx, e.advisor.actor(), sale.ID, UpdateInput{Version: &version})
		wantCode(t, err, "SALE_NOT_ACTIVE")
		_, err = e.sales.Cancel(e.ctx, e.manager.actor(), sale.ID, CancelInput{Category: "other", Text: "x"})
		wantCode(t, err, "SALE_NOT_ACTIVE")
	})
	t.Run("reopening", func(t *testing.T) {
		_, err := e.sales.Reopen(e.ctx, e.advisor.actor(), sale.ID, ReasonInput{Reason: "x"})
		wantCode(t, err, "FORBIDDEN")
		_, err = e.sales.Reopen(e.ctx, e.manager.actor(), sale.ID, ReasonInput{})
		wantCode(t, err, "REASON_REQUIRED")

		e.clock.Advance(4 * time.Hour)
		reopened, err := e.sales.Reopen(e.ctx, e.manager.actor(), sale.ID, ReasonInput{Reason: "el cliente volvió"})
		if err != nil || reopened.State != StateActive || reopened.CancelledAt != nil || reopened.CancellationCategory != nil {
			t.Fatalf("reopen: %+v, %v", reopened, err)
		}
		task := e.task(e.advisor, sale.ID, 2)
		if task.DueDate == nil || !task.DueDate.Equal(started.DueDate.Add(4*time.Hour)) {
			t.Errorf("the due date must move by the 4h spent cancelled (started %v): got %v", t0, task.DueDate)
		}
		_, err = e.sales.Reopen(e.ctx, e.manager.actor(), sale.ID, ReasonInput{Reason: "otra vez"})
		wantCode(t, err, "SALE_NOT_CANCELLED")
	})
}

func TestCancellationAndReopenAuthorizationByStage(t *testing.T) {
	e := newFlowEnv(t)

	t.Run("an advisor cancels only while tasks 1 to 3 are current", func(t *testing.T) {
		sale, _, _ := e.freshSale()
		e.runThrough(sale.ID, 3) // task 4 is now current
		_, err := e.sales.Cancel(e.ctx, e.advisor.actor(), sale.ID, CancelInput{Category: "customer_withdrew"})
		wantCode(t, err, "CANCEL_NOT_ALLOWED")
		if _, err := e.sales.Cancel(e.ctx, e.manager.actor(), sale.ID, CancelInput{Category: "customer_withdrew"}); err != nil {
			t.Fatalf("a manager can cancel through task 6: %v", err)
		}
	})
	t.Run("after task 6 only an admin cancels and reopens", func(t *testing.T) {
		sale, _, _ := e.freshSale()
		e.runThrough(sale.ID, 6) // task 7 is now current
		_, err := e.sales.Cancel(e.ctx, e.manager.actor(), sale.ID, CancelInput{Category: "unit_unavailable"})
		wantCode(t, err, "CANCEL_NOT_ALLOWED")
		if _, err := e.sales.Cancel(e.ctx, e.admin.actor(), sale.ID, CancelInput{Category: "unit_unavailable"}); err != nil {
			t.Fatal(err)
		}
		_, err = e.sales.Reopen(e.ctx, e.manager.actor(), sale.ID, ReasonInput{Reason: "intento"})
		wantCode(t, err, "REOPEN_NOT_ALLOWED")
		if _, err := e.sales.Reopen(e.ctx, e.admin.actor(), sale.ID, ReasonInput{Reason: "autorizado"}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("reopening checks the identifiers again", func(t *testing.T) {
		vin := randVIN()
		first, _, _ := e.freshSale()
		e.runThrough(first.ID, 2)
		e.runTask(first.ID, 3, map[string]any{"vin": vin})
		if _, err := e.sales.Cancel(e.ctx, e.manager.actor(), first.ID, CancelInput{Category: "customer_withdrew"}); err != nil {
			t.Fatal(err)
		}

		second, _, _ := e.freshSale()
		e.runThrough(second.ID, 2)
		e.runTask(second.ID, 3, map[string]any{"vin": vin}) // allowed: a cancelled sale frees its VIN

		_, err := e.sales.Reopen(e.ctx, e.manager.actor(), first.ID, ReasonInput{Reason: "volvió el cliente"})
		wantCode(t, err, "VIN_ALREADY_USED")
	})
}

func TestReassigningASale(t *testing.T) {
	e := newFlowEnv(t)
	sale, _, _ := e.freshSale()

	_, err := e.sales.Reassign(e.ctx, e.advisor.actor(), sale.ID, ReassignInput{AdvisorID: e.other.ID})
	wantCode(t, err, "FORBIDDEN")
	_, err = e.sales.Reassign(e.ctx, e.manager.actor(), sale.ID, ReassignInput{AdvisorID: e.advisor.ID})
	wantCode(t, err, "SAME_ADVISOR")
	_, err = e.sales.Reassign(e.ctx, e.manager.actor(), sale.ID, ReassignInput{AdvisorID: e.manager.ID})
	wantCode(t, err, "INVALID_ADVISOR")

	moved, err := e.sales.Reassign(e.ctx, e.manager.actor(), sale.ID, ReassignInput{AdvisorID: e.other.ID})
	if err != nil || moved.AdvisorID != e.other.ID {
		t.Fatalf("reassign: %+v, %v", moved, err)
	}
	if n := e.countAudit(sale.ID, "sale_register", "REASSIGNED"); n != 1 {
		t.Errorf("REASSIGNED events = %d", n)
	}

	// The previous advisor loses access at once, everywhere.
	_, err = e.sales.Get(e.ctx, e.advisor.actor(), sale.ID)
	wantCode(t, err, "SALE_NOT_FOUND")
	_, err = e.start(e.advisor, sale.ID, 1)
	wantCode(t, err, "SALE_NOT_FOUND")
	_, err = e.docs.List(e.ctx, e.advisor.actor(), sale.ID)
	wantCode(t, err, "SALE_NOT_FOUND")
	if _, err := e.start(e.other, sale.ID, 1); err != nil {
		t.Errorf("the new advisor works the sale: %v", err)
	}
}

// --- documents ---

func TestDocumentSlotsUploadsAndDownloads(t *testing.T) {
	e := newFlowEnv(t)
	sale, _, _ := e.freshSale()

	list, err := e.docs.List(e.ctx, e.advisor.actor(), sale.ID)
	if err != nil {
		t.Fatal(err)
	}
	slots := e.count(`SELECT count(*) FROM document_slots`)
	if len(list.Items) != slots {
		t.Fatalf("slots = %d, want %d", len(list.Items), slots)
	}
	required := map[string]bool{}
	for _, s := range list.Items {
		required[s.SlotCode] = s.Required
		if s.File != nil {
			t.Errorf("slot %s has a file in a new sale", s.SlotCode)
		}
	}
	if !required["quotation_pdf"] || required["power_of_attorney"] {
		t.Errorf("required flags for a Diplomático: %v", required)
	}

	_, _, err = e.upload(e.advisor, sale.ID, 1, "quotation_pdf", pdfBytes(), "")
	wantCode(t, err, "TASK_NOT_IN_PROGRESS")
	if _, err := e.start(e.advisor, sale.ID, 1); err != nil {
		t.Fatal(err)
	}

	first, replaced, err := e.upload(e.advisor, sale.ID, 1, "quotation_pdf", pdfBytes(), "")
	if err != nil || replaced || first.MimeType != "application/pdf" || first.UploadedBy == "" || len(first.ChecksumSHA) != 64 {
		t.Fatalf("first upload: %+v, %v, %v", first, replaced, err)
	}
	if len(e.store.Keys()) != 1 {
		t.Fatalf("stored objects = %v", e.store.Keys())
	}
	oldKey := e.store.Keys()[0]
	if !strings.HasPrefix(oldKey, "documents/"+sale.ID.String()+"/") || strings.Contains(oldKey, "quotation") {
		t.Errorf("storage key = %s", oldKey)
	}

	second, replaced, err := e.upload(e.advisor, sale.ID, 1, "quotation_pdf", pdfBytes(), "")
	if err != nil || !replaced || second.ID != first.ID || second.ChecksumSHA == first.ChecksumSHA {
		t.Fatalf("replacement: %+v, %v, %v", second, replaced, err)
	}
	if keys := e.store.Keys(); len(keys) != 1 || keys[0] == oldKey {
		t.Errorf("the replaced file must be deleted after the commit: %v", keys)
	}
	if n := e.countAudit(sale.ID, "document", "UPLOADED"); n != 1 {
		t.Errorf("UPLOADED events = %d", n)
	}
	if n := e.countAudit(sale.ID, "document", "REPLACED"); n != 1 {
		t.Errorf("REPLACED events = %d", n)
	}

	t.Run("validation by content", func(t *testing.T) {
		for name, data := range map[string][]byte{
			"a spreadsheet in a PDF slot": xlsxBytes(t),
			"text with a PDF name":        []byte("hola, esto no es un PDF"),
			"an executable":               []byte("MZ\x90\x00\x03 program"),
		} {
			_, _, err := e.upload(e.advisor, sale.ID, 1, "quotation_pdf", data, "")
			if codeOf(err) != "DOCUMENT_TYPE_NOT_ALLOWED" {
				t.Errorf("%s: %v", name, err)
			}
		}
		_, _, err := e.upload(e.advisor, sale.ID, 1, "quotation_excel", pdfBytes(), "")
		wantCode(t, err, "DOCUMENT_TYPE_NOT_ALLOWED")
		_, _, err = e.upload(e.advisor, sale.ID, 1, "quotation_pdf", nil, "")
		wantCode(t, err, "DOCUMENT_REQUIRED")
		_, _, err = e.upload(e.advisor, sale.ID, 1, "quotation_pdf", append(pdfBytes(), make([]byte, documents.MaxBytes)...), "")
		wantCode(t, err, "DOCUMENT_TOO_LARGE")
		if len(e.store.Keys()) != 1 {
			t.Errorf("a refused upload must not leave objects behind: %v", e.store.Keys())
		}
	})
	t.Run("slots belong to their task", func(t *testing.T) {
		_, _, err := e.upload(e.advisor, sale.ID, 1, "signer_identity_scan", pdfBytes(), "")
		wantCode(t, err, "DOCUMENT_SLOT_NOT_FOUND")
		_, _, err = e.upload(e.advisor, sale.ID, 1, "no_such_slot", pdfBytes(), "")
		wantCode(t, err, "DOCUMENT_SLOT_NOT_FOUND")
		_, _, err = e.upload(e.advisor, sale.ID, 14, "quotation_pdf", pdfBytes(), "")
		wantCode(t, err, "TASK_NOT_FOUND")
		_, _, err = e.upload(e.advisor, sale.ID, 2, "signer_identity_scan", pdfBytes(), "")
		wantCode(t, err, "TASK_NOT_IN_PROGRESS")
	})
	t.Run("scope", func(t *testing.T) {
		_, _, err := e.upload(e.other, sale.ID, 1, "quotation_pdf", pdfBytes(), "")
		wantCode(t, err, "SALE_NOT_FOUND")
		_, err = e.docs.DownloadURL(e.ctx, e.other.actor(), first.ID)
		wantCode(t, err, "DOCUMENT_NOT_FOUND")
		_, err = e.docs.DownloadURL(e.ctx, e.advisor.actor(), uuid.New())
		wantCode(t, err, "DOCUMENT_NOT_FOUND")
	})
	t.Run("signed download", func(t *testing.T) {
		url, err := e.docs.DownloadURL(e.ctx, e.advisor.actor(), first.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(url.URL, e.store.Keys()[0]) || url.FileName != second.FileName || url.MimeType != "application/pdf" ||
			!url.ExpiresAt.Equal(e.clock.Now().Add(documents.DefaultDownloadTTL)) {
			t.Errorf("download = %+v", url)
		}
		if _, err := e.docs.DownloadURL(e.ctx, e.manager.actor(), first.ID); err != nil {
			t.Errorf("a manager can download: %v", err)
		}
	})
	t.Run("removal while the task is in progress", func(t *testing.T) {
		if err := e.docs.Remove(e.ctx, e.advisor.actor(), sale.ID, 1, "quotation_pdf"); err != nil {
			t.Fatal(err)
		}
		if len(e.store.Keys()) != 0 {
			t.Errorf("the removed file must leave storage: %v", e.store.Keys())
		}
		wantCode(t, e.docs.Remove(e.ctx, e.advisor.actor(), sale.ID, 1, "quotation_pdf"), "DOCUMENT_NOT_FOUND")
		if n := e.countAudit(sale.ID, "document", "REMOVED"); n != 1 {
			t.Errorf("REMOVED events = %d", n)
		}
	})

	// On a completed task a file can be replaced, never removed.
	e.mustSave(e.advisor, sale.ID, 1, e.dataFor(1))
	e.uploadRequired(e.advisor, sale.ID, 1)
	if _, err := e.complete(e.advisor, sale.ID, 1); err != nil {
		t.Fatal(err)
	}
	wantCode(t, e.docs.Remove(e.ctx, e.advisor.actor(), sale.ID, 1, "quotation_pdf"), "TASK_NOT_IN_PROGRESS")
	wantCode(t, e.docs.Remove(e.ctx, e.manager.actor(), sale.ID, 1, "quotation_pdf"), "TASK_NOT_IN_PROGRESS")
	_, _, err = e.upload(e.advisor, sale.ID, 1, "quotation_pdf", pdfBytes(), "")
	wantCode(t, err, "FORBIDDEN")
	_, _, err = e.upload(e.manager, sale.ID, 1, "quotation_pdf", pdfBytes(), "")
	wantCode(t, err, "REASON_REQUIRED")
	if _, replaced, err := e.upload(e.manager, sale.ID, 1, "quotation_pdf", pdfBytes(), "versión corregida"); err != nil || !replaced {
		t.Fatalf("a manager replaces with a reason: %v, %v", replaced, err)
	}
	var reason *string
	if err := e.pool.QueryRow(e.ctx, `SELECT reason FROM audit_logs WHERE sale_id = $1 AND entity_type = 'document' AND action = 'REPLACED' AND reason IS NOT NULL`,
		sale.ID).Scan(&reason); err != nil || reason == nil || *reason != "versión corregida" {
		t.Errorf("the replacement reason is audited: %v, %v", reason, err)
	}
}

// --- permanent deletion ---

func TestPermanentDeletionOfASaleThatNeverWasDelivered(t *testing.T) {
	e := newFlowEnv(t)
	sale, cust, _ := e.freshSale()
	e.runThrough(sale.ID, 2)
	docs := e.count(`SELECT count(*) FROM documents WHERE sale_id = $1`, sale.ID)
	if docs < 4 || len(e.store.Keys()) != docs {
		t.Fatalf("documents = %d, stored = %d", docs, len(e.store.Keys()))
	}

	ok := DeleteInput{Reason: "venta de prueba", Confirmation: cust.DocumentNumber}
	wantCode(t, e.sales.Delete(e.ctx, e.manager.actor(), sale.ID, ok), "FORBIDDEN")
	wantCode(t, e.sales.Delete(e.ctx, e.advisor.actor(), sale.ID, ok), "FORBIDDEN")
	wantCode(t, e.sales.Delete(e.ctx, e.admin.actor(), sale.ID, DeleteInput{Confirmation: cust.DocumentNumber}), "REASON_REQUIRED")
	wantCode(t, e.sales.Delete(e.ctx, e.admin.actor(), sale.ID, DeleteInput{Reason: "x", Confirmation: "99999999"}), "CONFIRMATION_MISMATCH")
	if _, err := e.sales.Get(e.ctx, e.admin.actor(), sale.ID); err != nil {
		t.Fatalf("a refused deletion must leave the sale: %v", err)
	}

	typed := DeleteInput{Reason: "venta de prueba", Confirmation: " " + strings.ToLower(cust.DocumentNumber) + " "}
	if err := e.sales.Delete(e.ctx, e.admin.actor(), sale.ID, typed); err != nil {
		t.Fatalf("delete (this needs migration 0013): %v", err)
	}
	_, err := e.sales.Get(e.ctx, e.admin.actor(), sale.ID)
	wantCode(t, err, "SALE_NOT_FOUND")
	for _, table := range []string{"task_instances", "documents", "sale_financials", "sale_due_diligence", "sale_fulfillment"} {
		if n := e.count(`SELECT count(*) FROM `+table+` WHERE sale_id = $1`, sale.ID); n != 0 {
			t.Errorf("%s still has %d rows", table, n)
		}
	}
	if len(e.store.Keys()) != 0 {
		t.Errorf("the sale's files must leave storage: %v", e.store.Keys())
	}
	if n := e.countAudit(sale.ID, "sale_register", "DELETED"); n != 1 {
		t.Errorf("the DELETED event must survive: %d", n)
	}
	if n := e.count(`SELECT count(*) FROM audit_logs WHERE sale_id = $1`, sale.ID); n < 10 {
		t.Errorf("the sale's audit rows must survive: %d", n)
	}
	if n := e.count(`SELECT count(*) FROM storage_deletions WHERE storage_key LIKE 'documents/' || $1::text || '/%'
		AND reason = 'sale_deleted' AND completed_at IS NOT NULL`, sale.ID); n != docs {
		t.Errorf("completed deletions = %d, want %d", n, docs)
	}
}

func TestADeliveredSaleCanNeverBeDeleted(t *testing.T) {
	e := newFlowEnv(t)
	sale, cust, _ := e.freshSale()
	e.runThrough(sale.ID, 13)
	in := DeleteInput{Reason: "prueba", Confirmation: cust.DocumentNumber}
	wantCode(t, e.sales.Delete(e.ctx, e.admin.actor(), sale.ID, in), "SALE_ALREADY_DELIVERED")

	// Reopening Task 13 clears its completion date, but the audit log remembers.
	if _, err := e.tasks.Reopen(e.ctx, e.manager.actor(), sale.ID, 13, "corregir la entrega"); err != nil {
		t.Fatal(err)
	}
	reopened, err := e.sales.Get(e.ctx, e.manager.actor(), sale.ID)
	if err != nil || reopened.State != StateActive {
		t.Fatalf("reopening task 13 reactivates the sale: %+v, %v", reopened, err)
	}
	if n := e.countAudit(sale.ID, "sale_register", "REOPENED"); n != 1 {
		t.Errorf("sale REOPENED events = %d", n)
	}
	wantCode(t, e.sales.Delete(e.ctx, e.admin.actor(), sale.ID, in), "SALE_ALREADY_DELIVERED")

	// Task 13 can be completed again, and the sale with it.
	if _, err := e.complete(e.advisor, sale.ID, 13); err != nil {
		t.Fatal(err)
	}
	if again, _ := e.sales.Get(e.ctx, e.manager.actor(), sale.ID); again == nil || again.State != StateCompleted {
		t.Errorf("sale = %+v", again)
	}
}

// --- list, filters and history ---

func TestTheWorkspaceListScopesFiltersAndPages(t *testing.T) {
	e := newFlowEnv(t)
	cust, cat := e.newCustomer(), e.newCatalog()
	mine := e.newSale(e.advisor, cust, cat)
	e.clock.Advance(time.Second)
	theirs := e.newSale(e.other, cust, cat)

	search := func(actor testEmployee, tweak func(*ListFilter)) *ListResult {
		t.Helper()
		f := ListFilter{Search: cust.DisplayName}
		if tweak != nil {
			tweak(&f)
		}
		r, err := e.sales.List(e.ctx, actor.actor(), f)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	ids := func(r *ListResult) []uuid.UUID {
		out := make([]uuid.UUID, len(r.Items))
		for i, s := range r.Items {
			out[i] = s.ID
		}
		return out
	}

	if r := search(e.advisor, nil); r.Total != 1 || r.Items[0].ID != mine.ID {
		t.Errorf("advisor sees %v", ids(r))
	}
	if r := search(e.other, nil); r.Total != 1 || r.Items[0].ID != theirs.ID {
		t.Errorf("the other advisor sees %v", ids(r))
	}
	all := search(e.manager, nil)
	if all.Total != 2 || all.Items[0].ID != theirs.ID || all.Items[1].ID != mine.ID {
		t.Errorf("a manager sees both, newest first: %v", ids(all))
	}
	if r := search(e.manager, func(f *ListFilter) { f.Sort = "createdAt" }); r.Items[0].ID != mine.ID {
		t.Error("ascending order puts the oldest first")
	}

	counts := map[string]int{
		"state active":           search(e.manager, func(f *ListFilter) { f.State = "active" }).Total,
		"state cancelled":        search(e.manager, func(f *ListFilter) { f.State = "cancelled" }).Total,
		"current task 1":         search(e.manager, func(f *ListFilter) { f.CurrentTask = 1 }).Total,
		"current task 2":         search(e.manager, func(f *ListFilter) { f.CurrentTask = 2 }).Total,
		"milestone 1":            search(e.manager, func(f *ListFilter) { f.Milestone = 1 }).Total,
		"milestone 2":            search(e.manager, func(f *ListFilter) { f.Milestone = 2 }).Total,
		"hot":                    search(e.manager, func(f *ListFilter) { f.Temperature = "hot" }).Total,
		"cold":                   search(e.manager, func(f *ListFilter) { f.Temperature = "cold" }).Total,
		"diplomat":               search(e.manager, func(f *ListFilter) { f.CustomerType = "diplomatico" }).Total,
		"embassy":                search(e.manager, func(f *ListFilter) { f.CustomerType = "embajada" }).Total,
		"model":                  search(e.manager, func(f *ListFilter) { f.ModelID = &cat.ModelID }).Total,
		"version":                search(e.manager, func(f *ListFilter) { f.VersionID = &cat.VersionID }).Total,
		"advisor filter":         search(e.manager, func(f *ListFilter) { f.AdvisorID = &e.other.ID }).Total,
		"document number prefix": search(e.manager, func(f *ListFilter) { f.Search = cust.DocumentNumber }).Total,
		"unknown text":           search(e.manager, func(f *ListFilter) { f.Search = "zzzzzzqq" }).Total,
	}
	want := map[string]int{
		"state active": 2, "state cancelled": 0, "current task 1": 2, "current task 2": 0, "milestone 1": 2, "milestone 2": 0,
		"hot": 2, "cold": 0, "diplomat": 2, "embassy": 0, "model": 2, "version": 2, "advisor filter": 1,
		"document number prefix": 2, "unknown text": 0,
	}
	for name, n := range want {
		if counts[name] != n {
			t.Errorf("%s: got %d, want %d", name, counts[name], n)
		}
	}

	page1 := search(e.manager, func(f *ListFilter) { f.PageSize = 1 })
	page2 := search(e.manager, func(f *ListFilter) { f.PageSize = 1; f.Page = 2 })
	if page1.Total != 2 || len(page1.Items) != 1 || len(page2.Items) != 1 || page1.Items[0].ID == page2.Items[0].ID {
		t.Errorf("paging: %v %v", ids(page1), ids(page2))
	}

	// History follows the same scope.
	_, err := e.sales.History(e.ctx, e.other.actor(), mine.ID, 1, 20)
	wantCode(t, err, "SALE_NOT_FOUND")
	if h, err := e.sales.History(e.ctx, e.manager.actor(), mine.ID, 1, 20); err != nil || h.Total < 1 || h.Items[0].Action != "CREATED" {
		t.Errorf("history: %+v, %v", h, err)
	}
}

// --- concurrency ---

func TestTwoSimultaneousCompletionsCompleteTheTaskOnce(t *testing.T) {
	e := newFlowEnv(t)
	sale, _, _ := e.freshSale()
	e.runTask(sale.ID, 1, nil)
	if _, err := e.start(e.advisor, sale.ID, 2); err != nil {
		t.Fatal(err)
	}
	e.mustSave(e.advisor, sale.ID, 2, e.dataFor(2))
	e.uploadRequired(e.advisor, sale.ID, 2)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = e.complete(e.advisor, sale.ID, 2)
		}(i)
	}
	wg.Wait()

	succeeded, refused := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			succeeded++
		case codeOf(err) == "TASK_INVALID_STATE":
			refused++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if succeeded != 1 || refused != 1 {
		t.Fatalf("succeeded %d, refused %d", succeeded, refused)
	}
	if n := e.countAudit(sale.ID, "task_instance", "COMPLETED"); n != 2 { // task 1 and task 2, once each
		t.Errorf("COMPLETED events = %d", n)
	}
}
