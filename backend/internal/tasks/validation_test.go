package tasks_test

import (
	"endosoflow/internal/tasks"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestCompletingReportsEveryMissingFieldAndDocumentAtOnce(t *testing.T) {
	f := newFx(t)
	sale := f.newSale(f.advisor)
	if err := f.start(f.advisor, sale, 1); err != nil {
		t.Fatal(err)
	}
	err := f.complete(f.advisor, sale, 1)
	wantCode(t, err, "TASK_INCOMPLETE")
	got := fieldsOf(err)
	for _, want := range []string{"initialToyotaAmount", "initialCommission", "accessoryAmount", "releaseProcessingAmount", "documents.quotation_pdf", "documents.quotation_excel"} {
		if got[want] == "" {
			t.Errorf("missing a problem for %s: %v", want, got)
		}
	}

	// Fields saved, documents still missing: only the documents are reported.
	f.mustSave(f.advisor, sale, 1, f.validData(1))
	err = f.complete(f.advisor, sale, 1)
	wantCode(t, err, "TASK_INCOMPLETE")
	got = fieldsOf(err)
	if len(got) != 2 || got["documents.quotation_pdf"] != "DOCUMENT_REQUIRED" {
		t.Fatalf("only the two documents should be missing: %v", got)
	}
	f.FillRequiredDocuments(sale, 1, f.advisor.ID)
	if err := f.complete(f.advisor, sale, 1); err != nil {
		t.Fatalf("complete: %v", err)
	}
}

func TestThePowerOfAttorneyIsRequiredUnlessTheCustomerIsADiplomat(t *testing.T) {
	f := newFx(t)
	// A diplomat's sale does not need it.
	diplomat := f.newSale(f.advisor)
	f.run(f.advisor, diplomat, 1)
	d := f.detail(f.advisor, diplomat, 2)
	for _, slot := range d.Documents {
		if slot.SlotCode == "power_of_attorney" && slot.Required {
			t.Fatal("a diplomat does not need a power of attorney")
		}
	}
	// An organization's does.
	customer := f.Customer(f.manager.ID, true)
	org, err := f.sales.Create(f.Ctx, f.manager.Actor(), salesInput(f, customer))
	if err != nil {
		t.Fatal(err)
	}
	f.run(f.advisor, org.ID, 1)
	if err := f.start(f.advisor, org.ID, 2); err != nil {
		t.Fatal(err)
	}
	f.mustSave(f.advisor, org.ID, 2, f.validData(2))
	// Everything but the power of attorney.
	taskID := f.taskID(org.ID, 2)
	f.Document(org.ID, taskID, f.advisor.ID, "signer_identity_scan")
	f.Document(org.ID, taskID, f.advisor.ID, "customer_knowledge_affidavit")
	err = f.complete(f.advisor, org.ID, 2)
	wantCode(t, err, "TASK_INCOMPLETE")
	if fieldsOf(err)["documents.power_of_attorney"] != "DOCUMENT_REQUIRED" {
		t.Fatalf("the power of attorney must be required for an organization: %v", fieldsOf(err))
	}
}

func TestAmountsThatDifferFromTheExpectedOnesNeedAReason(t *testing.T) {
	f := newFx(t)
	sale := f.newSale(f.advisor)
	f.runThrough(f.advisor, sale, 3)

	if err := f.start(f.advisor, sale, 4); err != nil {
		t.Fatal(err)
	}
	d := f.mustSave(f.advisor, sale, 4, fmt.Sprintf(`{"bank":"bcp","paymentDate":"%s","transactionNumber":"T%s","depositedAmount":"34000"}`, today(f), digits(8)))
	if d.Calculated["expectedAmount"] != "34500.50" || d.Calculated["amountsMatch"] != false {
		t.Fatalf("calculated = %v", d.Calculated)
	}
	f.FillRequiredDocuments(sale, 4, f.advisor.ID)
	err := f.complete(f.advisor, sale, 4)
	wantCode(t, err, "TASK_INCOMPLETE")
	if fieldsOf(err)["mismatchReason"] != "MISMATCH_REASON_REQUIRED" {
		t.Fatalf("a different amount needs a reason: %v", fieldsOf(err))
	}
	f.mustSave(f.advisor, sale, 4, `{"mismatchReason":"El cliente pagó en dos partes"}`)
	if err := f.complete(f.advisor, sale, 4); err != nil {
		t.Fatalf("with a reason it completes: %v", err)
	}
}

func TestTheFinalAmountMustLeaveAPositiveCommission(t *testing.T) {
	f := newFx(t)
	sale := f.newSale(f.advisor)
	f.run(f.advisor, sale, 1)
	f.run(f.advisor, sale, 2)
	if err := f.start(f.advisor, sale, 3); err != nil {
		t.Fatal(err)
	}
	// 30000 + 2500 - 32500 = 0: not positive.
	_, err := f.save(f.advisor, sale, 3, `{"finalToyotaAmount":"32500"}`)
	wantCode(t, err, "VALIDATION_FAILED")
	if fieldsOf(err)["finalToyotaAmount"] != "FINAL_COMMISSION_NOT_POSITIVE" {
		t.Fatalf("fields = %v", fieldsOf(err))
	}
	d := f.mustSave(f.advisor, sale, 3, `{"finalToyotaAmount":"32499.99"}`)
	if d.Calculated["finalCommission"] != "0.01" || d.Calculated["quotedFinalPrice"] != "34500.50" {
		t.Fatalf("calculated = %v", d.Calculated)
	}
}

func TestSavingFieldsRefusesWrongFormatsAndReportsEveryProblem(t *testing.T) {
	f := newFx(t)
	sale := f.newSale(f.advisor)
	if err := f.start(f.advisor, sale, 1); err != nil {
		t.Fatal(err)
	}
	_, err := f.save(f.advisor, sale, 1, `{"initialToyotaAmount":"-5","initialCommission":"12,5","accessoryAmount":"0","releaseProcessingAmount":"abc"}`)
	wantCode(t, err, "VALIDATION_FAILED")
	if got := fieldsOf(err); len(got) != 4 {
		t.Fatalf("all four amounts should be reported: %v", got)
	}

	// Nothing was saved by the failed request.
	d := f.detail(f.advisor, sale, 1)
	if m := asMap(t, d.Data); m["initialToyotaAmount"] != nil {
		t.Fatalf("a refused save must not store anything: %v", m)
	}

	// Unknown fields and the wrong task's fields are refused.
	_, err = f.save(f.advisor, sale, 1, `{"vin":"JT4RN81A0M5123456"}`)
	wantCode(t, err, "INVALID_JSON")
	_, err = f.save(f.advisor, sale, 1, `not json`)
	wantCode(t, err, "INVALID_JSON")

	// A valid partial save keeps what is saved, and an empty string clears a field.
	f.mustSave(f.advisor, sale, 1, `{"initialToyotaAmount":"30000","accessoryAmount":"100"}`)
	f.mustSave(f.advisor, sale, 1, `{"initialCommission":"2500.5"}`)
	m := asMap(t, f.detail(f.advisor, sale, 1).Data)
	if m["initialToyotaAmount"] != "30000.00" || m["initialCommission"] != "2500.50" || m["accessoryAmount"] != "100.00" {
		t.Fatalf("data = %v", m)
	}
	f.mustSave(f.advisor, sale, 1, `{"accessoryAmount":""}`)
	if m := asMap(t, f.detail(f.advisor, sale, 1).Data); m["accessoryAmount"] != nil || m["initialToyotaAmount"] != "30000.00" {
		t.Fatalf("clearing one field must not touch the others: %v", m)
	}
}

func TestIdentifiersAreNormalizedAndChecked(t *testing.T) {
	f := newFx(t)
	sale := f.newSale(f.advisor)
	f.runThrough(f.advisor, sale, 2)
	if err := f.start(f.advisor, sale, 3); err != nil {
		t.Fatal(err)
	}
	_, err := f.save(f.advisor, sale, 3, `{"vin":"JT4RN81A0M512345","vehicleSeries":"  "}`) // 16 characters; blank series
	wantCode(t, err, "VALIDATION_FAILED")
	if got := fieldsOf(err); got["vin"] == "" {
		t.Fatalf("a short VIN must be refused: %v", got)
	}
	_, err = f.save(f.advisor, sale, 3, `{"vin":"IT4RN81A0M5123456"}`) // contains an I
	if fieldsOf(err)["vin"] == "" {
		t.Fatalf("a VIN with I, O or Q must be refused: %v", err)
	}
	v := fmt.Sprintf("jt4rn81a0m5 %s", digits(6))
	d := f.mustSave(f.advisor, sale, 3, fmt.Sprintf(`{"vin":"%s","vehicleSeries":" ab 12 "}`, v))
	m := asMap(t, d.Data)
	if m["vin"] != strings.ToUpper(strings.ReplaceAll(v, " ", "")) || m["vehicleSeries"] != "AB12" {
		t.Fatalf("identifiers are stored upper-cased without spaces: %v", m)
	}
}

func TestImpossibleDatesAreRefusedAndOutOfOrderDatesOnlyWarn(t *testing.T) {
	f := newFx(t)
	sale := f.newSale(f.advisor)
	f.runThrough(f.advisor, sale, 3)
	if err := f.start(f.advisor, sale, 4); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"2026-02-30", "30/10/2026", "1999-12-31", "2099-01-01", "ayer"} {
		_, err := f.save(f.advisor, sale, 4, fmt.Sprintf(`{"paymentDate":"%s"}`, bad))
		if fieldsOf(err)["paymentDate"] != "INVALID_DATE" {
			t.Errorf("date %q must be refused: %v", bad, err)
		}
	}
	// Before the sale existed: allowed, with a warning.
	d := f.mustSave(f.advisor, sale, 4, `{"paymentDate":"2020-01-15"}`)
	if len(d.Warnings) != 1 || d.Warnings[0].Code != "DATE_OUT_OF_ORDER" || d.Warnings[0].Field != "paymentDate" {
		t.Fatalf("warnings = %+v", d.Warnings)
	}
	// A date after today but within a year: allowed, with a warning.
	future := f.clock.Now().AddDate(0, 2, 0).Format("2006-01-02")
	d = f.mustSave(f.advisor, sale, 4, fmt.Sprintf(`{"paymentDate":"%s"}`, future))
	if len(d.Warnings) != 1 || d.Warnings[0].Code != "DATE_IN_THE_FUTURE" {
		t.Fatalf("warnings = %+v", d.Warnings)
	}
	// And the warnings never block completing.
	f.mustSave(f.advisor, sale, 4, fmt.Sprintf(`{"paymentDate":"2020-01-15","bank":"bcp","transactionNumber":"T%s","depositedAmount":"34500.50"}`, digits(8)))
	f.FillRequiredDocuments(sale, 4, f.advisor.ID)
	if err := f.complete(f.advisor, sale, 4); err != nil {
		t.Fatalf("a warning must not block completion: %v", err)
	}
	if w := f.detail(f.advisor, sale, 4).Warnings; len(w) != 1 {
		t.Fatalf("the warning stays visible after completion: %+v", w)
	}
}

func TestEditingAVersionedTaskNeedsTheCurrentVersion(t *testing.T) {
	f := newFx(t)
	sale := f.newSale(f.advisor)
	if err := f.start(f.advisor, sale, 1); err != nil {
		t.Fatal(err)
	}
	version := f.detail(f.advisor, sale, 1).Version
	if _, err := f.tasks.SaveData(f.Ctx, f.advisor.Actor(), sale, 1, tasks.SaveInput{Data: []byte(`{}`)}); code(err) != "VERSION_REQUIRED" {
		t.Fatalf("a missing version must be refused: %v", err)
	}
	in := tasks.SaveInput{Version: &version, Data: []byte(`{"initialToyotaAmount":"30000"}`)}
	if _, err := f.tasks.SaveData(f.Ctx, f.advisor.Actor(), sale, 1, in); err != nil {
		t.Fatal(err)
	}
	_, err := f.tasks.SaveData(f.Ctx, f.advisor.Actor(), sale, 1, in) // same stale version again
	wantCode(t, err, "VERSION_CONFLICT")
	if _, err := f.tasks.SaveNotes(f.Ctx, f.advisor.Actor(), sale, 1, &version, "nota"); code(err) != "VERSION_CONFLICT" {
		t.Fatalf("notes need the current version too: %v", err)
	}
}

func TestTaskNotes(t *testing.T) {
	f := newFx(t)
	sale := f.newSale(f.advisor)
	v := f.detail(f.advisor, sale, 1).Version
	d, err := f.tasks.SaveNotes(f.Ctx, f.advisor.Actor(), sale, 1, &v, "  El cliente prefiere llamadas por la tarde  ")
	if err != nil || d.Notes == nil || *d.Notes != "El cliente prefiere llamadas por la tarde" {
		t.Fatalf("notes: %v %+v", err, d)
	}
	v = d.Version
	d, err = f.tasks.SaveNotes(f.Ctx, f.advisor.Actor(), sale, 1, &v, "")
	if err != nil || d.Notes != nil {
		t.Fatalf("an empty note clears it: %v %+v", err, d.Notes)
	}
	v = d.Version
	_, err = f.tasks.SaveNotes(f.Ctx, f.advisor.Actor(), sale, 1, &v, strings.Repeat("x", 2001))
	wantCode(t, err, "VALIDATION_FAILED")
}

func TestCompletionPhasesFollowTheSLA(t *testing.T) {
	// Task 2 has 72 hours and a 12-hour reminder interval.
	cases := []struct {
		name  string
		after time.Duration
		want  tasks.Phase
	}{
		{"well inside the time", 10 * time.Hour, tasks.PhaseEarly},
		{"approaching the limit", 60 * time.Hour, tasks.PhaseApproaching},
		{"just overdue", 73 * time.Hour, tasks.PhaseOverdue},
		{"long overdue", 100 * time.Hour, tasks.PhaseRecurringOverdue},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFx(t)
			sale := f.newSale(f.advisor)
			f.run(f.advisor, sale, 1)
			if err := f.start(f.advisor, sale, 2); err != nil {
				t.Fatal(err)
			}
			f.mustSave(f.advisor, sale, 2, f.validData(2))
			f.FillRequiredDocuments(sale, 2, f.advisor.ID)
			f.clock.Advance(c.after)
			if err := f.complete(f.advisor, sale, 2); err != nil {
				t.Fatal(err)
			}
			d := f.detail(f.advisor, sale, 2)
			if d.CompletionPhase == nil || *d.CompletionPhase != c.want {
				t.Fatalf("phase = %v, want %s", d.CompletionPhase, c.want)
			}
		})
	}
	f := newFx(t)
	sale := f.newSale(f.advisor)
	f.run(f.advisor, sale, 1)
	if d := f.detail(f.advisor, sale, 1); d.CompletionPhase == nil || *d.CompletionPhase != tasks.PhaseNotApplicable {
		t.Fatalf("a task without an SLA is not applicable: %v", d.CompletionPhase)
	}
}

func TestTheDetailExplainsWhatThePersonCanDo(t *testing.T) {
	f := newFx(t)
	sale := f.newSale(f.advisor)
	d := f.detail(f.advisor, sale, 1)
	if !d.Permissions.CanStart || d.Permissions.CanComplete || d.Permissions.CanEditData {
		t.Fatalf("a pending eligible task can be started only: %+v", d.Permissions)
	}
	if d := f.detail(f.advisor, sale, 2); d.Permissions.CanStart {
		t.Fatalf("task 2 is not eligible yet: %+v", d.Permissions)
	}
	if err := f.start(f.advisor, sale, 1); err != nil {
		t.Fatal(err)
	}
	d = f.detail(f.advisor, sale, 1)
	if d.Permissions.CanStart || !d.Permissions.CanComplete || !d.Permissions.CanEditData || d.Permissions.NeedsReason || d.Permissions.CanRestart {
		t.Fatalf("an advisor on an in-progress task: %+v", d.Permissions)
	}
	if m := f.detail(f.manager, sale, 1); !m.Permissions.CanRestart {
		t.Fatalf("a manager can restart an in-progress task: %+v", m.Permissions)
	}
	f.mustSave(f.advisor, sale, 1, f.validData(1))
	f.FillRequiredDocuments(sale, 1, f.advisor.ID)
	if err := f.complete(f.advisor, sale, 1); err != nil {
		t.Fatal(err)
	}
	if d := f.detail(f.advisor, sale, 1); d.Permissions.CanEditData || d.Permissions.CanReopen || !d.Permissions.NeedsReason {
		t.Fatalf("an advisor cannot correct or reopen a completed task: %+v", d.Permissions)
	}
	if d := f.detail(f.manager, sale, 1); !d.Permissions.CanEditData || !d.Permissions.CanReopen {
		t.Fatalf("a manager can correct and reopen the last completed task: %+v", d.Permissions)
	}
}
