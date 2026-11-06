package tasks

import "time"

// Completion phases, as stored in task_instances.completion_phase.
const (
	PhaseEarly            = "early"
	PhaseInApproaching    = "in_approaching"
	PhaseOverdue          = "overdue"
	PhaseRecurringOverdue = "recurring_overdue"
	PhaseNotApplicable    = "not_applicable"
)

// dueDateFor is when a task that started at startedAt is due: the start plus its duration in elapsed hours.
// Weekends and holidays never pause the clock.
func dueDateFor(startedAt time.Time, durationHours int) time.Time {
	return startedAt.Add(time.Duration(durationHours) * time.Hour)
}

// completionPhase classifies a completion against the due date (state machines spec, section 5.5).
//
//	early:             completed before 80% of the duration
//	in_approaching:    from 80% of the duration up to the due date
//	overdue:           after the due date, within one reminder interval
//	recurring_overdue: one reminder interval or more after the due date
//	not_applicable:    the task has no duration
//
// It works from the due date rather than the start, so a due date that moved (a cancelled sale that was
// reopened, or a task that was reopened) keeps the same meaning. Phase 3 reuses this function.
func completionPhase(completedAt time.Time, dueDate *time.Time, durationHours, intervalHours *int) string {
	if durationHours == nil || intervalHours == nil || dueDate == nil {
		return PhaseNotApplicable
	}
	duration := time.Duration(*durationHours) * time.Hour
	interval := time.Duration(*intervalHours) * time.Hour
	approaching := dueDate.Add(-duration / 5)
	switch {
	case completedAt.Before(approaching):
		return PhaseEarly
	case !completedAt.After(*dueDate):
		return PhaseInApproaching
	case completedAt.Before(dueDate.Add(interval)):
		return PhaseOverdue
	default:
		return PhaseRecurringOverdue
	}
}
