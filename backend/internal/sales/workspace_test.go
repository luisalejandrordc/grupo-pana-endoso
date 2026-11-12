package sales_test

import (
	"endosoflow/internal/auth"
	"endosoflow/internal/sales"
	"endosoflow/internal/tasks"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func (f *fx) ids(l *sales.List) map[uuid.UUID]bool {
	m := map[uuid.UUID]bool{}
	for _, it := range l.Items {
		m[it.ID] = true
	}
	return m
}

func (f *fx) list(actor auth.Actor, filter sales.ListFilter) *sales.List {
	f.T.Helper()
	l, err := f.sales.List(f.Ctx, actor, filter)
	if err != nil {
		f.T.Fatalf("list: %v", err)
	}
	return l
}

func TestAnAdvisorSeesOnlyTheirOwnSalesInEveryListSearchAndCount(t *testing.T) {
	f := newFx(t)
	mine := f.newSale(f.advisor)
	theirs := f.newSale(f.other)

	got := f.list(f.advisor.Actor(), sales.ListFilter{Search: theirs.Customer.DocumentNumber})
	if got.Total != 0 {
		t.Fatalf("searching another advisor's customer must find nothing: %+v", got)
	}
	all := f.list(f.advisor.Actor(), sales.ListFilter{PageSize: 100})
	ids := f.ids(all)
	if !ids[mine.ID] || ids[theirs.ID] {
		t.Fatalf("an advisor's list holds only their sales: %v", ids)
	}
	for _, it := range all.Items {
		if it.AdvisorID != f.advisor.ID {
			t.Fatalf("someone else's sale leaked: %+v", it)
		}
	}
	// Filtering by another advisor does not widen the scope.
	narrowed := f.list(f.advisor.Actor(), sales.ListFilter{AdvisorID: &f.other.ID, PageSize: 100})
	if f.ids(narrowed)[theirs.ID] {
		t.Fatal("an advisor cannot list another advisor's sales through the advisor filter")
	}
	// Managers and admins see both.
	managerIDs := f.ids(f.list(f.manager.Actor(), sales.ListFilter{PageSize: 100}))
	if !managerIDs[mine.ID] || !managerIDs[theirs.ID] {
		t.Fatal("a manager sees every sale")
	}
	onlyOther := f.list(f.manager.Actor(), sales.ListFilter{AdvisorID: &f.other.ID, PageSize: 100})
	if !f.ids(onlyOther)[theirs.ID] || f.ids(onlyOther)[mine.ID] {
		t.Fatal("the advisor filter narrows a manager's list")
	}
	// Reading another advisor's sale is "not found", never "forbidden".
	if _, err := f.sales.Get(f.Ctx, f.advisor.Actor(), theirs.ID); code(err) != "SALE_NOT_FOUND" {
		t.Fatalf("got %v", err)
	}
	if _, err := f.sales.History(f.Ctx, f.advisor.Actor(), theirs.ID, 1, 10); code(err) != "SALE_NOT_FOUND" {
		t.Fatalf("history: %v", err)
	}
}

func TestTheWorkspaceFiltersSearchesSortsAndPages(t *testing.T) {
	f := newFx(t)
	a := f.newSale(f.advisor)
	b := f.newSale(f.advisor)
	c := f.newSale(f.advisor)
	f.advance(b.ID, f.advisor, 3) // current task 4 (milestone 2)
	if _, err := f.sales.Cancel(f.Ctx, f.manager.Actor(), c.ID, "customer_withdrew", "Desistió"); err != nil {
		t.Fatal(err)
	}
	// An organization with its own temperature.
	org := f.Customer(f.manager.ID, true)
	in := f.input(org, &f.advisor.ID)
	in.Temperature = "cold"
	d, err := f.sales.Create(f.Ctx, f.manager.Actor(), in)
	if err != nil {
		t.Fatal(err)
	}
	mine := func(filter sales.ListFilter) map[uuid.UUID]bool {
		filter.AdvisorID = &f.advisor.ID
		filter.PageSize = 100
		return f.ids(f.list(f.manager.Actor(), filter))
	}

	if got := mine(sales.ListFilter{State: "cancelled"}); !got[c.ID] || got[a.ID] || got[b.ID] {
		t.Fatalf("state filter: %v", got)
	}
	if got := mine(sales.ListFilter{State: "active"}); got[c.ID] || !got[a.ID] || !got[b.ID] {
		t.Fatalf("state filter: %v", got)
	}
	if got := mine(sales.ListFilter{CurrentTask: 4}); !got[b.ID] || got[a.ID] {
		t.Fatalf("current task filter: %v", got)
	}
	if got := mine(sales.ListFilter{Milestone: 2}); !got[b.ID] || got[a.ID] {
		t.Fatalf("milestone filter: %v", got)
	}
	if got := mine(sales.ListFilter{Milestone: 1}); !got[a.ID] || got[b.ID] {
		t.Fatalf("milestone filter: %v", got)
	}
	if got := mine(sales.ListFilter{CustomerType: "embajada"}); !got[d.ID] || got[a.ID] {
		t.Fatalf("customer type filter: %v", got)
	}
	if got := mine(sales.ListFilter{Temperature: "cold"}); !got[d.ID] || got[a.ID] {
		t.Fatalf("temperature filter: %v", got)
	}
	if got := mine(sales.ListFilter{ModelID: &f.vehicle.ModelID}); !got[a.ID] || !got[d.ID] {
		t.Fatalf("model filter: %v", got)
	}

	// Search: by customer name, document, the code and the vehicle series that task 3 recorded.
	series := ""
	if err := f.Pool.QueryRow(f.Ctx, `SELECT vehicle_series FROM sale_registers WHERE id = $1`, b.ID).Scan(&series); err != nil {
		t.Fatal(err)
	}
	for name, q := range map[string]string{"document": a.Customer.DocumentNumber[:6], "code": b.Code, "series": strings.ToLower(series), "organization": d.Customer.Name[:20]} {
		found := mine(sales.ListFilter{Search: q})
		want := map[string]uuid.UUID{"document": a.ID, "code": b.ID, "series": b.ID, "organization": d.ID}[name]
		if !found[want] {
			t.Errorf("searching by %s (%q) must find the sale: %v", name, q, found)
		}
	}
	if got := mine(sales.ListFilter{Search: "%"}); len(got) != 0 {
		t.Fatalf("a lone %% must not match everything: %v", got)
	}
	if got := mine(sales.ListFilter{Search: "zzzzzzzz"}); len(got) != 0 {
		t.Fatalf("nothing matches: %v", got)
	}

	// Newest first by default, and the order can be reversed.
	desc := f.list(f.manager.Actor(), sales.ListFilter{AdvisorID: &f.advisor.ID, PageSize: 100})
	asc := f.list(f.manager.Actor(), sales.ListFilter{AdvisorID: &f.advisor.ID, PageSize: 100, Sort: "createdAt"})
	if desc.Items[0].ID != asc.Items[len(asc.Items)-1].ID {
		t.Fatal("-createdAt must reverse createdAt")
	}
	// Pages.
	p1 := f.list(f.manager.Actor(), sales.ListFilter{AdvisorID: &f.advisor.ID, PageSize: 2, Page: 1})
	p2 := f.list(f.manager.Actor(), sales.ListFilter{AdvisorID: &f.advisor.ID, PageSize: 2, Page: 2})
	if p1.Total != 4 || len(p1.Items) != 2 || len(p2.Items) != 2 || p1.Items[0].ID == p2.Items[0].ID {
		t.Fatalf("pagination: %+v %+v", p1, p2)
	}
	if clamped := f.list(f.manager.Actor(), sales.ListFilter{PageSize: 5000}); clamped.PageSize != 100 {
		t.Fatalf("page size must be capped: %d", clamped.PageSize)
	}
}

func TestTheListShowsEachSalesCurrentTaskAndWhetherItIsOverdue(t *testing.T) {
	f := newFx(t)
	s := f.newSale(f.advisor)
	f.advance(s.ID, f.advisor, 1)
	if _, err := f.tasks.Start(f.Ctx, f.advisor.Actor(), s.ID, 2); err != nil {
		t.Fatal(err)
	}
	row := func() sales.ListItem {
		for _, it := range f.list(f.advisor.Actor(), sales.ListFilter{PageSize: 100}).Items {
			if it.ID == s.ID {
				return it
			}
		}
		t.Fatal("sale missing from the list")
		return sales.ListItem{}
	}
	it := row()
	if it.CurrentTask == nil || it.CurrentTask.Number != 2 || it.CurrentTask.Status != tasks.StatusInProgress || it.CurrentTask.DueDate == nil ||
		it.CurrentTask.IsOverdue || it.CurrentTask.Milestone != 1 {
		t.Fatalf("current task = %+v", it.CurrentTask)
	}
	if it.CustomerName != "Lucía Quispe" || it.VehicleModel == "" || it.ImageURL == nil || it.Code != s.Code {
		t.Fatalf("row = %+v", it)
	}
	f.clock.Advance(100 * time.Hour) // task 2 has 72 hours
	if !row().CurrentTask.IsOverdue {
		t.Fatal("a task past its due date is overdue")
	}
}

func TestTheDetailShowsMilestonesAndWhatEachRoleCanDo(t *testing.T) {
	f := newFx(t)
	s := f.newSale(f.advisor)
	f.advance(s.ID, f.advisor, 5) // tasks 1-5 done: milestones 1 and 2 completed, milestone 3 in progress

	d, err := f.sales.Get(f.Ctx, f.advisor.Actor(), s.ID)
	if err != nil {
		t.Fatal(err)
	}
	var states []tasks.MilestoneState
	for _, m := range d.Milestones {
		states = append(states, m.State)
	}
	want := []tasks.MilestoneState{tasks.MilestoneCompleted, tasks.MilestoneCompleted, tasks.MilestoneInProgress, tasks.MilestonePending, tasks.MilestonePending, tasks.MilestonePending}
	if fmt.Sprint(states) != fmt.Sprint(want) {
		t.Fatalf("milestones = %v, want %v", states, want)
	}
	if d.Milestones[0].Name == "" || d.Milestones[0].Description == "" {
		t.Fatalf("milestones carry their names: %+v", d.Milestones[0])
	}
	if d.CurrentTask == nil || d.CurrentTask.Number != 6 || d.Vehicle.Series == nil || d.Vehicle.VIN == nil || d.QuotedFinalPrice == nil || *d.QuotedFinalPrice != "34500.50" {
		t.Fatalf("detail = current %+v, vehicle %+v, price %v", d.CurrentTask, d.Vehicle, d.QuotedFinalPrice)
	}

	type perms = sales.Permissions
	check := func(name string, actor auth.Actor, want perms) {
		t.Helper()
		got, err := f.sales.Get(f.Ctx, actor, s.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Permissions != want {
			t.Errorf("%s: permissions = %+v, want %+v", name, got.Permissions, want)
		}
	}
	// At task 6 an advisor can no longer cancel; a manager still can; an admin can do everything including delete.
	check("owner advisor", f.advisor.Actor(), perms{CanEditNotes: true})
	check("manager", f.manager.Actor(), perms{CanEditNotes: true, CanCancel: true, CanReassign: true})
	check("admin", f.admin.Actor(), perms{CanEditNotes: true, CanCancel: true, CanReassign: true, CanDelete: true})
}
