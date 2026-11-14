package tasks_test

import (
	"endosoflow/internal/tasks"
	"sync"
	"testing"
	"time"
)

func TestASaleGoesThroughAllThirteenTasksInOrder(t *testing.T) {
	f := newFx(t)
	sale := f.newSale(f.advisor)

	for n := 1; n <= tasks.TaskCount; n++ {
		list, err := f.tasks.List(f.Ctx, f.advisor.Actor(), sale)
		if err != nil || len(list) != tasks.TaskCount {
			t.Fatalf("list before task %d: %v (%d tasks)", n, err, len(list))
		}
		for _, s := range list {
			wantEligible := s.Number == n
			if s.IsEligible != wantEligible {
				t.Fatalf("before task %d: task %d eligible = %v, want %v", n, s.Number, s.IsEligible, wantEligible)
			}
		}
		f.run(f.advisor, sale, n)
		got := f.detail(f.advisor, sale, n)
		if got.Status != tasks.StatusCompleted || got.CompletedAt == nil || got.CompletionPhase == nil {
			t.Fatalf("task %d after completing: %+v", n, got.Summary)
		}
	}

	var state string
	var completedAt any
	if err := f.Pool.QueryRow(f.Ctx, `SELECT overall_state::text, completed_at FROM sale_registers WHERE id = $1`, sale).Scan(&state, &completedAt); err != nil {
		t.Fatal(err)
	}
	if state != "completed" || completedAt == nil {
		t.Fatalf("after task 13 the sale must be completed: %s %v", state, completedAt)
	}
	// Task 13 completing the sale is recorded as automatic.
	if f.auditCount(sale, "COMPLETED") != 1 {
		t.Fatal("expected one COMPLETED audit entry for the sale")
	}
	// Every task has its own STARTED and COMPLETED entries.
	for n := 1; n <= tasks.TaskCount; n++ {
		id := f.taskID(sale, n)
		if f.auditCount(id, "STARTED") != 1 || f.auditCount(id, "COMPLETED") != 1 {
			t.Fatalf("task %d audit entries missing", n)
		}
	}
	// The customer is told at the milestone tasks only: 1, 3, 6, 7, 10, 11, 13 (and the sale was created). The
	// manager assigned the advisor, so the advisor was told too.
	notes := f.notifications(sale)
	for _, kind := range []string{"milestone_task_1", "milestone_task_3", "milestone_task_6", "milestone_task_7", "milestone_task_10", "milestone_task_11", "milestone_task_13", "sale_created", "sale_assigned"} {
		if notes[kind] != 1 {
			t.Errorf("notification %s queued %d times, want 1 (all: %v)", kind, notes[kind], notes)
		}
	}
	if len(notes) != 9 {
		t.Errorf("unexpected notifications: %v", notes)
	}
}

func TestTheNextTaskCannotStartBeforeThePreviousIsDoneNorWhileAnotherRuns(t *testing.T) {
	f := newFx(t)
	sale := f.newSale(f.advisor)

	wantCode(t, f.start(f.advisor, sale, 2), "TASK_NOT_ELIGIBLE")
	wantCode(t, f.start(f.advisor, sale, 13), "TASK_NOT_ELIGIBLE")
	wantCode(t, f.complete(f.advisor, sale, 1), "TASK_NOT_IN_PROGRESS") // pending, never started

	if err := f.start(f.advisor, sale, 1); err != nil {
		t.Fatal(err)
	}
	wantCode(t, f.start(f.advisor, sale, 1), "TASK_NOT_ELIGIBLE") // a second click on Start
	wantCode(t, f.start(f.advisor, sale, 2), "TASK_NOT_ELIGIBLE") // another task while task 1 runs
	wantCode(t, f.complete(f.advisor, sale, 2), "TASK_NOT_IN_PROGRESS")

	f.mustSave(f.advisor, sale, 1, f.validData(1))
	f.FillRequiredDocuments(sale, 1, f.advisor.ID)
	if err := f.complete(f.advisor, sale, 1); err != nil {
		t.Fatal(err)
	}
	wantCode(t, f.complete(f.advisor, sale, 1), "TASK_NOT_IN_PROGRESS") // double click on Complete
	wantCode(t, f.start(f.advisor, sale, 3), "TASK_NOT_ELIGIBLE")       // task 2 is still pending
	if err := f.start(f.advisor, sale, 2); err != nil {
		t.Fatalf("task 2 is next: %v", err)
	}
	wantCode(t, f.start(f.advisor, sale, 0), "TASK_NOT_FOUND")
	wantCode(t, f.start(f.advisor, sale, 14), "TASK_NOT_FOUND")
}

func TestStartingATaskSetsItsClockAndMilestonesFollow(t *testing.T) {
	f := newFx(t)
	sale := f.newSale(f.advisor)
	f.run(f.advisor, sale, 1)

	before := f.clock.Now()
	if err := f.start(f.advisor, sale, 2); err != nil {
		t.Fatal(err)
	}
	d := f.detail(f.advisor, sale, 2)
	if d.StartedAt == nil || !d.StartedAt.Equal(before.Truncate(time.Microsecond)) && d.StartedAt.Sub(before).Abs() > time.Second {
		t.Fatalf("started at = %v, want about %v", d.StartedAt, before)
	}
	if d.DueDate == nil || d.DueDate.Sub(*d.StartedAt) != 72*time.Hour {
		t.Fatalf("task 2 has a 72-hour SLA: due %v from %v", d.DueDate, d.StartedAt)
	}
	// A task without an SLA has no due date.
	f.FillRequiredDocuments(sale, 2, f.advisor.ID)
	f.mustSave(f.advisor, sale, 2, f.validData(2))
	if err := f.complete(f.advisor, sale, 2); err != nil {
		t.Fatal(err)
	}
	f.run(f.advisor, sale, 3)
	f.run(f.advisor, sale, 4)
	if err := f.start(f.advisor, sale, 5); err != nil {
		t.Fatal(err)
	}
	if d := f.detail(f.advisor, sale, 5); d.DueDate != nil || d.RecommendedDurationHours != nil {
		t.Fatalf("task 5 has no SLA: %+v", d.Summary)
	}
}

func TestTwoSimultaneousCompletionsCompleteTheTaskOnce(t *testing.T) {
	f := newFx(t)
	sale := f.newSale(f.advisor)
	// Complete task 1 so task 2 becomes eligible.
	f.runThrough(f.advisor, sale, 1)
	if err := f.start(f.advisor, sale, 2); err != nil {
		t.Fatal(err)
	}
	f.mustSave(f.advisor, sale, 2, f.validData(2))
	f.FillRequiredDocuments(sale, 2, f.advisor.ID)
	// Attempt to complete task 2 simultaneously.
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = f.complete(f.advisor, sale, 2)
		}(i)
	}
	wg.Wait()
	succeeded := 0
	refused := 0
	for _, err := range errs {
		switch {
		case err == nil:
			succeeded++
		case code(err) == "TASK_NOT_IN_PROGRESS":
			refused++
		default:
			t.Errorf("unexpected completion error: %v", err)
		}
	}
	if succeeded != 1 || refused != 1 {
		t.Fatalf("succeeded = %d, refused = %d, want one of each", succeeded, refused)
	}
	// Only one completion audit event should exist for task 2.
	taskID := f.taskID(sale, 2)
	if n := f.auditCount(taskID, "COMPLETED"); n != 1 {
		t.Errorf("task 2 completion audit events = %d, want 1", n)
	}
}
