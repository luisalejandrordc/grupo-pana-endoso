package sales_test

import (
	"endosoflow/internal/auth"
	"endosoflow/internal/sales"
	"endosoflow/internal/tasks"
	"regexp"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSaleCreationReturnsAllMissingFieldErrors(t *testing.T) {
	f := newFx(t)
	_, err := f.sales.Create(
		f.Ctx,
		f.manager.Actor(),
		sales.CreateInput{},
	)
	got := fieldsOf(err)
	expected := []string{
		"customerId",
		"versionId",
		"colorId",
		"temperature",
		"advisorId",
	}
	for _, field := range expected {
		if _, ok := got[field]; !ok {
			t.Errorf("missing validation error for %s: %v", field, got)
		}
	}
}

func TestAnAdvisorCreatesASaleForThemselves(t *testing.T) {
	f := newFx(t)
	customer := f.Customer(f.advisor.ID, false)
	s, err := f.sales.Create(f.Ctx, f.advisor.Actor(), sales.CreateInput{
		CustomerID: customer, VersionID: f.vehicle.VersionID,
		ColorID: f.vehicle.ColorID, Temperature: "warm", Notes: "  Quiere entrega en marzo  ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^EF-\d{4}-\d{5}$`).MatchString(s.Code) {
		t.Fatalf("code = %q", s.Code)
	}
	if s.State != "active" || s.Temperature != "warm" || s.Advisor.ID != f.advisor.ID || s.Version != 1 {
		t.Fatalf("sale = %+v", s)
	}
	if s.Notes == nil || *s.Notes != "Quiere entrega en marzo" {
		t.Fatalf("notes = %v", s.Notes)
	}
	if s.Customer.ID != customer || s.Customer.Name != "Lucía Quispe" || s.Customer.DocumentType != "DNI" || s.Customer.CustomerType != "diplomatico" {
		t.Fatalf("customer snapshot = %+v", s.Customer)
	}
	if s.Vehicle.Version != "SR 4x2" || s.Vehicle.Color != "Blanco" || s.Vehicle.ReferencePrice != "35000.00" || s.Vehicle.ImageURL == nil ||
		*s.Vehicle.ImageURL == "" || s.Vehicle.VIN != nil {
		t.Fatalf("vehicle snapshot = %+v", s.Vehicle)
	}
	if len(s.Tasks) != tasks.TaskCount || s.CurrentTask == nil || s.CurrentTask.Number != 1 || s.CurrentTask.Status != tasks.StatusPending {
		t.Fatalf("a new sale starts at task 1: current=%+v", s.CurrentTask)
	}
	if len(s.Milestones) != 6 || s.Milestones[0].State != tasks.MilestoneInProgress || s.Milestones[1].State != tasks.MilestonePending {
		t.Fatalf("milestones = %+v", s.Milestones)
	}
	for _, task := range s.Tasks {
		if task.Status != tasks.StatusPending || task.IsEligible != (task.Number == 1) {
			t.Fatalf("task %d: %+v", task.Number, task)
		}
	}
	if f.auditCount(s.ID, "sale_register", "CREATED") != 1 {
		t.Fatal("expected a CREATED audit entry")
	}
	// The customer is told; the advisor who created it for themselves is not.
	if n := f.notifications(s.ID); n["sale_created"] != 1 || n["sale_assigned"] != 0 || len(n) != 1 {
		t.Fatalf("notifications = %v", n)
	}
}

func TestAManagerChoosesTheAdvisorAndTheAdvisorIsTold(t *testing.T) {
	f := newFx(t)
	customer := f.Customer(f.manager.ID, false)

	_, err := f.sales.Create(f.Ctx, f.manager.Actor(), f.input(customer, nil))
	wantCode(t, err, "VALIDATION_FAILED")
	if fieldsOf(err)["advisorId"] != "REQUIRED" {
		t.Fatalf("a manager must choose the advisor: %v", fieldsOf(err))
	}
	for name, target := range map[string]uuid.UUID{"a manager": f.manager.ID, "an unknown employee": uuid.New()} {
		_, err = f.sales.Create(f.Ctx, f.manager.Actor(), f.input(customer, &target))
		if fieldsOf(err)["advisorId"] != "INVALID_ADVISOR" {
			t.Errorf("%s cannot be the advisor: %v", name, err)
		}
	}
	if _, err := f.Pool.Exec(f.Ctx, `UPDATE employees SET is_active = false, deactivated_at = now(), deactivated_by = $2 WHERE id = $1`, f.other.ID, f.admin.ID); err != nil {
		t.Fatal(err)
	}
	_, err = f.sales.Create(f.Ctx, f.manager.Actor(), f.input(customer, &f.other.ID))
	if fieldsOf(err)["advisorId"] != "INVALID_ADVISOR" {
		t.Fatalf("an inactive advisor cannot take sales: %v", err)
	}

	s, err := f.sales.Create(f.Ctx, f.admin.Actor(), f.input(customer, &f.advisor.ID))
	if err != nil {
		t.Fatal(err)
	}
	if n := f.notifications(s.ID); n["sale_created"] != 1 || n["sale_assigned"] != 1 {
		t.Fatalf("the assigned advisor must be notified: %v", n)
	}
}

func TestAnAdvisorCannotCreateASaleForAnotherAdvisor(t *testing.T) {
	f := newFx(t)
	customer := f.Customer(f.advisor.ID, false)
	_, err := f.sales.Create(f.Ctx, f.advisor.Actor(), f.input(customer, &f.other.ID))
	if fieldsOf(err)["advisorId"] != "NOT_ALLOWED" {
		t.Fatalf("got %v", err)
	}
	if _, err := f.sales.Create(f.Ctx, f.advisor.Actor(), f.input(customer, &f.advisor.ID)); err != nil {
		t.Fatalf("naming themselves is fine: %v", err)
	}
	_, err = f.sales.Create(f.Ctx, auth.Actor{ID: f.advisor.ID, Role: auth.Role("customer")}, f.input(customer, nil))
	wantCode(t, err, "FORBIDDEN")
}

func TestCreateValidatesTheCustomerAndTheVehicle(t *testing.T) {
	f := newFx(t)
	customer := f.Customer(f.advisor.ID, false)
	mk := func(mutate func(*sales.CreateInput)) error {
		in := f.input(customer, nil)
		mutate(&in)
		_, err := f.sales.Create(f.Ctx, f.advisor.Actor(), in)
		return err
	}

	for _, bad := range []string{"", "tibia", "HOT"} {
		err := mk(func(in *sales.CreateInput) { in.Temperature = bad })
		if fieldsOf(err)["temperature"] != "INVALID_VALUE" {
			t.Errorf("temperature %q must be refused: %v", bad, err)
		}
	}
	wantCode(t, mk(func(in *sales.CreateInput) { in.CustomerID = uuid.New() }), "CUSTOMER_NOT_FOUND")
	wantCode(t, mk(func(in *sales.CreateInput) { in.VersionID = uuid.New() }), "VALIDATION_FAILED")
	wantCode(t, mk(func(in *sales.CreateInput) { in.ColorID = uuid.New() }), "VALIDATION_FAILED")

	// An inactive customer cannot receive a sale.
	if _, err := f.Pool.Exec(f.Ctx, `UPDATE customers SET is_active = false, deactivated_at = now(), deactivated_by = $2 WHERE id = $1`, customer, f.admin.ID); err != nil {
		t.Fatal(err)
	}
	wantCode(t, mk(func(in *sales.CreateInput) {}), "CUSTOMER_INACTIVE")
	if _, err := f.Pool.Exec(f.Ctx, `UPDATE customers SET is_active = true, deactivated_at = NULL, deactivated_by = NULL WHERE id = $1`, customer); err != nil {
		t.Fatal(err)
	}

	// A color of another model does not fit this version.
	other := f.Vehicle(f.admin.ID)
	err := mk(func(in *sales.CreateInput) { in.ColorID = other.ColorID })
	if fieldsOf(err)["colorId"] != "COLOR_MODEL_MISMATCH" {
		t.Fatalf("got %v", err)
	}

	// Archived entries are not offered: the version, the color and the whole model.
	archive := func(table string, id uuid.UUID, active bool) {
		f.Replica(func(c *pgxpool.Conn) {
			sql := `UPDATE ` + table + ` SET is_active = true, deactivated_at = NULL, deactivated_by = NULL WHERE id = $1`
			args := []any{id}
			if !active {
				sql = `UPDATE ` + table + ` SET is_active = false, deactivated_at = now(), deactivated_by = $2 WHERE id = $1`
				args = append(args, f.admin.ID)
			}
			if _, err := c.Exec(f.Ctx, sql, args...); err != nil {
				t.Fatal(err)
			}
		})
	}
	archive("vehicle_versions", f.vehicle.VersionID, false)
	err = mk(func(in *sales.CreateInput) {})
	if fieldsOf(err)["versionId"] != "NOT_AVAILABLE" {
		t.Fatalf("an archived version is not offered: %v", err)
	}
	archive("vehicle_versions", f.vehicle.VersionID, true)
	archive("vehicle_colors", f.vehicle.ColorID, false)
	err = mk(func(in *sales.CreateInput) {})
	if fieldsOf(err)["colorId"] != "NOT_AVAILABLE" {
		t.Fatalf("an archived color is not offered: %v", err)
	}
	archive("vehicle_colors", f.vehicle.ColorID, true)
	archive("vehicle_models", f.vehicle.ModelID, false)
	err = mk(func(in *sales.CreateInput) {})
	if fieldsOf(err)["versionId"] != "NOT_AVAILABLE" {
		t.Fatalf("a version of an archived model is not offered: %v", err)
	}
	archive("vehicle_models", f.vehicle.ModelID, true)
	if err := mk(func(in *sales.CreateInput) {}); err != nil {
		t.Fatalf("everything active again: %v", err)
	}
}

func TestASaleKeepsTheDataItWasCreatedWith(t *testing.T) {
	f := newFx(t)
	s := f.newSale(f.advisor)
	// The customer and the catalog change afterwards...
	if _, err := f.Pool.Exec(f.Ctx, `UPDATE customers SET first_name = 'Otra', last_name = 'Persona' WHERE id = $1`, s.Customer.ID); err != nil {
		t.Fatal(err)
	}
	f.Replica(func(c *pgxpool.Conn) {
		_, _ = c.Exec(f.Ctx, `UPDATE vehicle_versions SET version_name = 'Renombrada', reference_price = 99999 WHERE id = $1`, f.vehicle.VersionID)
		_, _ = c.Exec(f.Ctx, `UPDATE vehicle_colors SET color_name = 'Rojo' WHERE id = $1`, f.vehicle.ColorID)
	})
	// ...and the sale still shows what was recorded.
	got, err := f.sales.Get(f.Ctx, f.advisor.Actor(), s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Customer.Name != "Lucía Quispe" || got.Vehicle.Version != "SR 4x2" || got.Vehicle.Color != "Blanco" || got.Vehicle.ReferencePrice != "35000.00" {
		t.Fatalf("the snapshots must not follow later edits: %+v %+v", got.Customer, got.Vehicle)
	}
}
