package notifications

import (
	"context"
	"endosoflow/internal/adapters/storage"
	"endosoflow/internal/auth"
	"endosoflow/internal/sales"
	"endosoflow/internal/tasks"
	"endosoflow/internal/testkit"
	"testing"

	"github.com/google/uuid"
)

// discardSalesNotifier keeps sale creation independent from the Writer under test.
// Each test invokes the Writer method explicitly and inspects the resulting outbox rows.
type discardSalesNotifier struct{}

func (discardSalesNotifier) SaleCreated(
	context.Context,
	tasks.Querier,
	uuid.UUID,
	uuid.UUID,
	uuid.UUID,
) error {
	return nil
}

func (discardSalesNotifier) AdvisorChanged(
	context.Context,
	tasks.Querier,
	uuid.UUID,
	uuid.UUID,
	uuid.UUID,
) error {
	return nil
}

type writerFixture struct {
	env     *testkit.Env
	sales   *sales.Service
	admin   testkit.Employee
	manager testkit.Employee
	advisor testkit.Employee
	other   testkit.Employee
	vehicle testkit.Vehicle
	writer  Writer
}

func newWriterFixture(t *testing.T) *writerFixture {
	t.Helper()

	env := testkit.New(t)
	f := &writerFixture{env: env, writer: Writer{}}

	f.admin = env.Employee(auth.RoleSystemAdmin, false)
	f.manager = env.Employee(auth.RoleSalesManager, false)
	f.advisor = env.Employee(auth.RoleSalesAdvisor, false)
	f.other = env.Employee(auth.RoleSalesAdvisor, false)
	f.vehicle = env.Vehicle(f.admin.ID)

	f.sales = sales.NewService(sales.Deps{
		DB:       env.Pool,
		Images:   storage.NewMemory("https://img.test"),
		Notifier: discardSalesNotifier{},
		Logger:   env.Logger,
	})

	return f
}

// createSale creates a real sale and its 13 task instances, but suppresses the
// service-level notification call so the test can invoke Writer methods directly.
func (f *writerFixture) createSale(t *testing.T, actor testkit.Employee, advisor testkit.Employee) (saleID, customerID uuid.UUID) {
	t.Helper()

	customerID = f.env.Customer(actor.ID, false)
	advisorID := advisor.ID

	created, err := f.sales.Create(f.env.Ctx, actor.Actor(), sales.CreateInput{
		CustomerID:  customerID,
		VersionID:   f.vehicle.VersionID,
		ColorID:     f.vehicle.ColorID,
		Temperature: "hot",
		AdvisorID:   &advisorID,
	})
	if err != nil {
		t.Fatalf("create sale fixture: %v", err)
	}

	return created.ID, customerID
}

func (f *writerFixture) countType(t *testing.T, saleID uuid.UUID, kind string) int {
	t.Helper()

	var count int
	err := f.env.Pool.QueryRow(f.env.Ctx, `
		SELECT count(*)
		FROM notification_logs
		WHERE sale_id = $1
		  AND notification_type = $2::text::notification_type
	`, saleID, kind).Scan(&count)
	if err != nil {
		t.Fatalf("count %s notifications: %v", kind, err)
	}
	return count
}

func (f *writerFixture) countCustomerRecipient(t *testing.T, saleID uuid.UUID, kind string, customerID uuid.UUID) int {
	t.Helper()

	var count int
	err := f.env.Pool.QueryRow(f.env.Ctx, `
		SELECT count(*)
		FROM notification_logs
		WHERE sale_id = $1
		  AND notification_type = $2::text::notification_type
		  AND recipient_customer_id = $3
	`, saleID, kind, customerID).Scan(&count)
	if err != nil {
		t.Fatalf("count %s customer notifications: %v", kind, err)
	}
	return count
}

func (f *writerFixture) countEmployeeRecipient(t *testing.T, saleID uuid.UUID, kind string, employeeID uuid.UUID) int {
	t.Helper()

	var count int
	err := f.env.Pool.QueryRow(f.env.Ctx, `
		SELECT count(*)
		FROM notification_logs
		WHERE sale_id = $1
		  AND notification_type = $2::text::notification_type
		  AND recipient_employee_id = $3
	`, saleID, kind, employeeID).Scan(&count)
	if err != nil {
		t.Fatalf("count %s employee notifications: %v", kind, err)
	}
	return count
}

func (f *writerFixture) taskID(t *testing.T, saleID uuid.UUID, taskNumber int) uuid.UUID {
	t.Helper()

	var taskID uuid.UUID
	err := f.env.Pool.QueryRow(f.env.Ctx, `
		SELECT id
		FROM task_instances
		WHERE sale_id = $1 AND task_number = $2
	`, saleID, taskNumber).Scan(&taskID)
	if err != nil {
		t.Fatalf("find task %d: %v", taskNumber, err)
	}
	return taskID
}

func TestWriterTaskCompletedMilestone(t *testing.T) {
	f := newWriterFixture(t)
	saleID, customerID := f.createSale(t, f.manager, f.advisor)
	taskID := f.taskID(t, saleID, 1)

	if err := f.writer.TaskCompleted(f.env.Ctx, f.env.Pool, saleID, taskID, 1); err != nil {
		t.Fatalf("TaskCompleted: %v", err)
	}

	if got := f.countType(t, saleID, "milestone_task_1"); got != 1 {
		t.Fatalf("milestone_task_1 count = %d, want 1", got)
	}

	var gotTaskID, gotCustomerID uuid.UUID
	var gotEmail, gotDedupeKey string
	err := f.env.Pool.QueryRow(f.env.Ctx, `
		SELECT task_instance_id, recipient_customer_id, recipient_email, dedupe_key
		FROM notification_logs
		WHERE sale_id = $1
		  AND notification_type = 'milestone_task_1'::notification_type
	`, saleID).Scan(&gotTaskID, &gotCustomerID, &gotEmail, &gotDedupeKey)
	if err != nil {
		t.Fatalf("read milestone notification: %v", err)
	}

	if gotTaskID != taskID {
		t.Errorf("task_instance_id = %s, want %s", gotTaskID, taskID)
	}
	if gotCustomerID != customerID {
		t.Errorf("recipient_customer_id = %s, want %s", gotCustomerID, customerID)
	}
	if gotEmail == "" {
		t.Error("recipient_email is empty")
	}
	wantDedupeKey := "milestone_task_1:" + saleID.String()
	if gotDedupeKey != wantDedupeKey {
		t.Errorf("dedupe_key = %q, want %q", gotDedupeKey, wantDedupeKey)
	}
}

func TestWriterTaskCompletedNonMilestone(t *testing.T) {
	f := newWriterFixture(t)
	saleID, _ := f.createSale(t, f.manager, f.advisor)
	taskID := f.taskID(t, saleID, 2)

	if err := f.writer.TaskCompleted(f.env.Ctx, f.env.Pool, saleID, taskID, 2); err != nil {
		t.Fatalf("TaskCompleted for non-milestone: %v", err)
	}

	var total int
	if err := f.env.Pool.QueryRow(f.env.Ctx,
		`SELECT count(*) FROM notification_logs WHERE sale_id = $1`, saleID,
	).Scan(&total); err != nil {
		t.Fatalf("count all notifications: %v", err)
	}
	if total != 0 {
		t.Fatalf("notifications for non-milestone task = %d, want 0", total)
	}
}

func TestWriterTaskCompletedDeduplicates(t *testing.T) {
	f := newWriterFixture(t)
	saleID, _ := f.createSale(t, f.manager, f.advisor)
	taskID := f.taskID(t, saleID, 3)

	for i := 0; i < 2; i++ {
		if err := f.writer.TaskCompleted(f.env.Ctx, f.env.Pool, saleID, taskID, 3); err != nil {
			t.Fatalf("TaskCompleted call %d: %v", i+1, err)
		}
	}

	if got := f.countType(t, saleID, "milestone_task_3"); got != 1 {
		t.Fatalf("milestone_task_3 count after duplicate enqueue = %d, want 1", got)
	}
}

func TestWriterSaleCreatedNotifiesCustomerAndAssignedAdvisor(t *testing.T) {
	f := newWriterFixture(t)
	saleID, customerID := f.createSale(t, f.manager, f.advisor)

	if err := f.writer.SaleCreated(f.env.Ctx, f.env.Pool, saleID, f.advisor.ID, f.manager.ID); err != nil {
		t.Fatalf("SaleCreated: %v", err)
	}

	if got := f.countCustomerRecipient(t, saleID, "sale_created", customerID); got != 1 {
		t.Errorf("customer sale_created notifications = %d, want 1", got)
	}
	if got := f.countEmployeeRecipient(t, saleID, "sale_assigned", f.advisor.ID); got != 1 {
		t.Errorf("advisor sale_assigned notifications = %d, want 1", got)
	}
	if got := f.countType(t, saleID, "sale_created"); got != 1 {
		t.Errorf("sale_created count = %d, want 1", got)
	}
	if got := f.countType(t, saleID, "sale_assigned"); got != 1 {
		t.Errorf("sale_assigned count = %d, want 1", got)
	}
}

func TestWriterSaleCreatedByAdvisorDoesNotNotifyAdvisorOfOwnAssignment(t *testing.T) {
	f := newWriterFixture(t)
	saleID, customerID := f.createSale(t, f.advisor, f.advisor)

	if err := f.writer.SaleCreated(f.env.Ctx, f.env.Pool, saleID, f.advisor.ID, f.advisor.ID); err != nil {
		t.Fatalf("SaleCreated: %v", err)
	}

	if got := f.countCustomerRecipient(t, saleID, "sale_created", customerID); got != 1 {
		t.Errorf("customer sale_created notifications = %d, want 1", got)
	}
	if got := f.countType(t, saleID, "sale_assigned"); got != 0 {
		t.Errorf("sale_assigned count = %d, want 0 when advisor created their own sale", got)
	}
}

func TestWriterSaleCreatedDeduplicates(t *testing.T) {
	f := newWriterFixture(t)
	saleID, _ := f.createSale(t, f.manager, f.advisor)

	for i := 0; i < 2; i++ {
		if err := f.writer.SaleCreated(f.env.Ctx, f.env.Pool, saleID, f.advisor.ID, f.manager.ID); err != nil {
			t.Fatalf("SaleCreated call %d: %v", i+1, err)
		}
	}

	if got := f.countType(t, saleID, "sale_created"); got != 1 {
		t.Errorf("sale_created count after duplicate enqueue = %d, want 1", got)
	}
	if got := f.countType(t, saleID, "sale_assigned"); got != 1 {
		t.Errorf("sale_assigned count after duplicate enqueue = %d, want 1", got)
	}
}

func TestWriterAdvisorChangedNotifiesCustomerAndBothAdvisors(t *testing.T) {
	f := newWriterFixture(t)
	saleID, customerID := f.createSale(t, f.manager, f.advisor)

	if err := f.writer.AdvisorChanged(
		f.env.Ctx, f.env.Pool, saleID, f.advisor.ID, f.other.ID,
	); err != nil {
		t.Fatalf("AdvisorChanged: %v", err)
	}

	if got := f.countCustomerRecipient(t, saleID, "customer_advisor_changed", customerID); got != 1 {
		t.Errorf("customer advisor-change notifications = %d, want 1", got)
	}
	if got := f.countEmployeeRecipient(t, saleID, "sale_assigned", f.other.ID); got != 1 {
		t.Errorf("new advisor assignment notifications = %d, want 1", got)
	}
	if got := f.countEmployeeRecipient(t, saleID, "sale_unassigned", f.advisor.ID); got != 1 {
		t.Errorf("previous advisor unassignment notifications = %d, want 1", got)
	}
}

func TestWriterAdvisorChangedMultipleTimesCreatesDistinctEvents(t *testing.T) {
	f := newWriterFixture(t)
	saleID, _ := f.createSale(t, f.manager, f.advisor)

	for i := 0; i < 2; i++ {
		if err := f.writer.AdvisorChanged(
			f.env.Ctx, f.env.Pool, saleID, f.advisor.ID, f.other.ID,
		); err != nil {
			t.Fatalf("AdvisorChanged call %d: %v", i+1, err)
		}
	}

	for _, kind := range []string{"customer_advisor_changed", "sale_assigned", "sale_unassigned"} {
		if got := f.countType(t, saleID, kind); got != 2 {
			t.Errorf("%s count after two distinct reassignments = %d, want 2", kind, got)
		}
	}
}

func TestWriterAdvisorChangedMissingPreviousEmployeeIsTolerated(t *testing.T) {
	f := newWriterFixture(t)
	saleID, customerID := f.createSale(t, f.manager, f.advisor)
	missingPreviousID := uuid.New()

	if err := f.writer.AdvisorChanged(
		f.env.Ctx, f.env.Pool, saleID, missingPreviousID, f.other.ID,
	); err != nil {
		t.Fatalf("AdvisorChanged with missing previous employee: %v", err)
	}

	if got := f.countCustomerRecipient(t, saleID, "customer_advisor_changed", customerID); got != 1 {
		t.Errorf("customer advisor-change notifications = %d, want 1", got)
	}
	if got := f.countEmployeeRecipient(t, saleID, "sale_assigned", f.other.ID); got != 1 {
		t.Errorf("new advisor assignment notifications = %d, want 1", got)
	}
	if got := f.countType(t, saleID, "sale_unassigned"); got != 0 {
		t.Errorf("sale_unassigned count = %d, want 0 for a missing previous employee", got)
	}
}
