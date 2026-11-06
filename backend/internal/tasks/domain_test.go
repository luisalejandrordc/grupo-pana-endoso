package tasks

import (
	"endosoflow/internal/auth"
	"testing"
	"time"
)

func shape(completed int, inProgress bool) []Status {
	s := make([]Status, TaskCount)
	for i := range s {
		s[i] = StatusPending
		if i < completed {
			s[i] = StatusCompleted
		}
	}
	if inProgress && completed < TaskCount {
		s[completed] = StatusInProgress
	}
	return s
}

func TestEveryValidShapeIsAccepted(t *testing.T) {
	for k := 0; k <= TaskCount; k++ {
		for _, inProgress := range []bool{false, true} {
			if err := CheckShape(shape(k, inProgress)); err != nil {
				t.Errorf("k=%d inProgress=%v: %v", k, inProgress, err)
			}
		}
	}
}

func TestInvalidShapesAreRejected(t *testing.T) {
	gap := shape(2, false)
	gap[4] = StatusCompleted // task 5 completed while task 3 is not
	skip := shape(2, false)
	skip[3] = StatusInProgress // task 4 in progress while task 3 is pending
	two := shape(2, true)
	two[5] = StatusInProgress
	for name, s := range map[string][]Status{"completed after a gap": gap, "in progress skips a task": skip, "two in progress": two, "too short": shape(0, false)[:5]} {
		if err := CheckShape(s); err == nil {
			t.Errorf("%s must be rejected", name)
		}
	}
}

func TestCurrentTaskAndEligibility(t *testing.T) {
	cases := []struct {
		completed  int
		inProgress bool
		current    int
	}{{0, false, 1}, {0, true, 1}, {3, false, 4}, {3, true, 4}, {12, true, 13}, {13, false, 0}}
	for _, c := range cases {
		s := shape(c.completed, c.inProgress)
		if got := CurrentTask(s); got != c.current {
			t.Errorf("completed=%d inProgress=%v: current = %d, want %d", c.completed, c.inProgress, got, c.current)
		}
		for task := 1; task <= TaskCount; task++ {
			want := !c.inProgress && task == c.completed+1 && task <= TaskCount
			if IsEligible(s, task) != want {
				t.Errorf("completed=%d inProgress=%v: IsEligible(%d) = %v, want %v", c.completed, c.inProgress, task, !want, want)
			}
		}
	}
	if IsEligible(shape(0, false), 0) || IsEligible(shape(0, false), 14) {
		t.Error("out-of-range tasks are never eligible")
	}
}

func TestMilestoneMapping(t *testing.T) {
	want := map[int]int{1: 1, 2: 1, 3: 2, 4: 2, 5: 3, 6: 3, 7: 4, 8: 4, 9: 5, 10: 5, 11: 6, 12: 6, 13: 6}
	for task, m := range want {
		if MilestoneOf(task) != m {
			t.Errorf("MilestoneOf(%d) = %d, want %d", task, MilestoneOf(task), m)
		}
	}
	if MilestoneOf(0) != 0 || MilestoneOf(14) != 0 {
		t.Error("out-of-range tasks have no milestone")
	}
}

func TestMilestoneDerivation(t *testing.T) {
	type states = [MilestoneCount]MilestoneState
	P, I, C := MilestonePending, MilestoneInProgress, MilestoneCompleted
	cases := []struct {
		completed  int
		inProgress bool
		want       states
	}{
		{0, false, states{I, P, P, P, P, P}},  // a new sale: task 1 is current and eligible
		{1, true, states{I, P, P, P, P, P}},   // still inside milestone 1
		{2, false, states{C, I, P, P, P, P}},  // milestone 1 done, task 3 is current
		{4, true, states{C, C, I, P, P, P}},   // task 5 in progress
		{10, false, states{C, C, C, C, C, I}}, // task 11 is current
		{12, true, states{C, C, C, C, C, I}},  // task 13 in progress
		{13, false, states{C, C, C, C, C, C}}, // delivered: everything completed, no current task
	}
	for _, c := range cases {
		if got := DeriveMilestones(shape(c.completed, c.inProgress)); got != c.want {
			t.Errorf("completed=%d inProgress=%v: %v, want %v", c.completed, c.inProgress, got, c.want)
		}
	}
}

func TestCompletionPhase(t *testing.T) {
	start := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	// A 3-day task (72 h) with a 12-hour reminder interval, like Task 2.
	cases := []struct {
		elapsedHours float64
		want         Phase
	}{
		{0, PhaseEarly},
		{57.5, PhaseEarly},          // below 80% (57.6 h)
		{57.6, PhaseApproaching},    // exactly 80%
		{72, PhaseApproaching},      // exactly the due date
		{72.01, PhaseOverdue},       // just past due
		{83.9, PhaseOverdue},        // less than one interval late
		{84, PhaseRecurringOverdue}, // one full interval late
		{500, PhaseRecurringOverdue},
	}
	for _, c := range cases {
		done := start.Add(time.Duration(c.elapsedHours * float64(time.Hour)))
		if got := CompletionPhase(72, 12, start, done); got != c.want {
			t.Errorf("elapsed %.2f h: %s, want %s", c.elapsedHours, got, c.want)
		}
	}
	if CompletionPhase(0, 0, start, start.Add(1000*time.Hour)) != PhaseNotApplicable {
		t.Error("a task without a duration is not applicable")
	}
	if DueDate(start, 72) != start.Add(72*time.Hour) {
		t.Error("due date")
	}
}

func TestCancelRuleByRoleAndStage(t *testing.T) {
	type c struct {
		role    auth.Role
		own     bool
		current int
		want    bool
	}
	for _, x := range []c{
		{auth.RoleSalesAdvisor, true, 1, true},
		{auth.RoleSalesAdvisor, true, 3, true},
		{auth.RoleSalesAdvisor, true, 4, false},
		{auth.RoleSalesAdvisor, false, 1, false},
		{auth.RoleSalesManager, false, 1, true},
		{auth.RoleSalesManager, false, 6, true},
		{auth.RoleSalesManager, false, 7, false},
		{auth.RoleSystemAdmin, false, 1, true},
		{auth.RoleSystemAdmin, false, 13, true},
		{auth.Role("customer"), true, 1, false},
	} {
		if got := CanCancel(x.role, x.own, x.current); got != x.want {
			t.Errorf("CanCancel(%s, own=%v, current=%d) = %v, want %v", x.role, x.own, x.current, got, x.want)
		}
	}
}

func TestReopenRuleByRoleAndStage(t *testing.T) {
	for _, x := range []struct {
		role      auth.Role
		cancelled int
		want      bool
	}{
		{auth.RoleSystemAdmin, 1, true},
		{auth.RoleSystemAdmin, 9, true},
		{auth.RoleSalesManager, 6, true},
		{auth.RoleSalesManager, 7, false},
		{auth.RoleSalesAdvisor, 1, false},
	} {
		if got := CanReopenSale(x.role, x.cancelled); got != x.want {
			t.Errorf("CanReopenSale(%s, %d) = %v, want %v", x.role, x.cancelled, got, x.want)
		}
	}
}

func TestWhoCanActOnATask(t *testing.T) {
	if !CanActOnTask(auth.RoleSalesAdvisor, true) || CanActOnTask(auth.RoleSalesAdvisor, false) {
		t.Error("an advisor acts only on their own sale")
	}
	if !CanActOnTask(auth.RoleSalesManager, false) || !CanActOnTask(auth.RoleSystemAdmin, false) {
		t.Error("managers and admins act on any sale")
	}
	if CanActOnTask(auth.Role("customer"), true) {
		t.Error("customers never act on tasks")
	}
}

func TestIdentifierNormalization(t *testing.T) {
	if n, ok := NormalizeVIN(" jt 4rn81a 0m5 123456 "); !ok || n != "JT4RN81A0M5123456" {
		t.Errorf("VIN = %q %v", n, ok)
	}
	for _, bad := range []string{"JT4RN81A0M512345", "JT4RN81A0M51234567", "IT4RN81A0M5123456", "OT4RN81A0M5123456", "QT4RN81A0M5123456", ""} {
		if _, ok := NormalizeVIN(bad); ok {
			t.Errorf("VIN %q must be rejected", bad)
		}
	}
	if n, ok := NormalizePlate(" abc 123 "); !ok || n != "ABC123" {
		t.Errorf("plate = %q %v", n, ok)
	}
	for _, bad := range []string{"ABC12", "ABC1234", "ABC-12", ""} {
		if _, ok := NormalizePlate(bad); ok {
			t.Errorf("plate %q must be rejected", bad)
		}
	}
	if n, ok := NormalizeInvoiceNumber(" f001-000123 "); !ok || n != "F001-000123" {
		t.Errorf("invoice = %q %v", n, ok)
	}
	for _, bad := range []string{"F01-1", "F001000123", "F001-", "F001-123456789", "F0-01-1", "FFFFF-1"} {
		if _, ok := NormalizeInvoiceNumber(bad); ok {
			t.Errorf("invoice %q must be rejected", bad)
		}
	}
	if n, ok := NormalizeSeries(" ab 12 "); !ok || n != "AB12" {
		t.Errorf("series = %q %v", n, ok)
	}
	if _, ok := NormalizeSeries("   "); ok {
		t.Error("an empty series must be rejected")
	}
	if n, ok := NormalizeTransaction(" tx 99 "); !ok || n != "TX99" {
		t.Errorf("transaction = %q %v", n, ok)
	}
	if n, ok := NormalizeDAM(" 118-2026 10 123456 "); !ok || n != "118-202610123456" {
		t.Errorf("dam = %q %v", n, ok)
	}
}

func TestDateParsing(t *testing.T) {
	today := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	good := map[string]string{"2026-10-30": "2026-10-30", " 2024-02-29 ": "2024-02-29", "2026-12-01": "2026-12-01", "2000-01-01": "2000-01-01"}
	for in, want := range good {
		d, _, ok := ParseDate(in, today)
		if !ok || d.Format("2006-01-02") != want {
			t.Errorf("ParseDate(%q) = %v %v", in, d, ok)
		}
	}
	bad := map[string]DateProblem{
		"2026-02-30": DateInvalid, "2025-02-29": DateInvalid, "30/10/2026": DateInvalid, "": DateInvalid, "2026-13-01": DateInvalid,
		"1999-12-31": DateTooOld, "0001-01-01": DateTooOld,
		"2027-10-31": DateTooFarAhead, "2062-01-01": DateTooFarAhead,
	}
	for in, want := range bad {
		if _, problem, ok := ParseDate(in, today); ok || problem != want {
			t.Errorf("ParseDate(%q) = %v %v, want problem %s", in, problem, ok, want)
		}
	}
}

func TestChronologyWarnings(t *testing.T) {
	today := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	d := func(s string) time.Time { t, _ := time.Parse("2006-01-02", s); return t }
	fields := func(w []Warning) map[string]string {
		m := map[string]string{}
		for _, x := range w {
			m[x.Field] = x.Code
		}
		return m
	}

	ordered := []DateEntry{
		{"created", GroupSaleCreated, d("2026-09-01")},
		{"pana", GroupPanaPayment, d("2026-09-05")},
		{"toyota", GroupToyotaPayment, d("2026-09-06")},
		{"inv1", GroupInvoices, d("2026-09-10")},
		{"inv2", GroupInvoices, d("2026-09-08")}, // invoices have no order among themselves
		{"reg", GroupRegistration, d("2026-09-20")},
		{"pds", GroupPDSTransfer, d("2026-09-25")},
		{"equip", GroupEquipment, d("2026-10-01")},
		{"surquillo", GroupSurquillo, d("2026-10-02")},
		{"delivery", GroupDelivery, d("2026-10-03")},
	}
	if w := ChronologyWarnings(ordered, today); len(w) != 0 {
		t.Fatalf("a well-ordered process has no warnings: %v", w)
	}

	out := []DateEntry{
		{"created", GroupSaleCreated, d("2026-09-10")}, {"pana", GroupPanaPayment, d("2026-09-05")}, // before the sale existed
		{"reg", GroupRegistration, d("2026-09-20")}, {"pds", GroupPDSTransfer, d("2026-09-15")}, // transfer before registration
		{"delivery", GroupDelivery, d("2026-11-15")}, // in the future
	}
	got := fields(ChronologyWarnings(out, today))
	if got["pana"] != "DATE_OUT_OF_ORDER" || got["pds"] != "DATE_OUT_OF_ORDER" || got["delivery"] != "DATE_IN_THE_FUTURE" {
		t.Fatalf("warnings = %v", got)
	}
	if _, flagged := got["reg"]; flagged {
		t.Fatalf("an in-order date must not be flagged: %v", got)
	}
}
