// Package tasks is the task engine of a sale: the 13-step sequence, its state machine, per-task validation,
// the derived milestones and the SLA bookkeeping. This file is pure domain code with no database access, so
// the rules can be tested without one.
package tasks

import (
	"endosoflow/internal/auth"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// TaskCount is the fixed length of every sale's sequence.
const TaskCount = 13

// Status of one task.
type Status string

// The three task statuses.
const (
	StatusPending    Status = "pending"
	StatusInProgress Status = "in_progress"
	StatusCompleted  Status = "completed"
)

// Phase says how a completed task related to its SLA, computed from timestamps only.
type Phase string

// The completion phases.
const (
	PhaseEarly            Phase = "early"
	PhaseApproaching      Phase = "in_approaching"
	PhaseOverdue          Phase = "overdue"
	PhaseRecurringOverdue Phase = "recurring_overdue"
	PhaseNotApplicable    Phase = "not_applicable"
)

// milestoneOf maps each task to its milestone (a derived grouping, never stored).
var milestoneOf = [TaskCount + 1]int{0, 1, 1, 2, 2, 3, 3, 4, 4, 5, 5, 6, 6, 6}

// MilestoneOf returns the milestone number (1 to 6) of a task number (1 to 13).
func MilestoneOf(task int) int {
	if task < 1 || task > TaskCount {
		return 0
	}
	return milestoneOf[task]
}

// MilestoneCount is the number of milestones.
const MilestoneCount = 6

// MilestoneState is the derived state of a milestone.
type MilestoneState string

// The milestone states.
const (
	MilestonePending    MilestoneState = "pending"
	MilestoneInProgress MilestoneState = "in_progress"
	MilestoneCompleted  MilestoneState = "completed"
)

// CurrentTask is the single in-progress task or, when none is in progress, the lowest-numbered task that is
// not completed. It is 0 when all tasks are completed. statuses[0] is task 1.
func CurrentTask(statuses []Status) int {
	for i, s := range statuses {
		if s == StatusInProgress {
			return i + 1
		}
	}
	for i, s := range statuses {
		if s != StatusCompleted {
			return i + 1
		}
	}
	return 0
}

// CompletedCount is the number of completed tasks.
func CompletedCount(statuses []Status) int {
	n := 0
	for _, s := range statuses {
		if s == StatusCompleted {
			n++
		}
	}
	return n
}

// CheckShape verifies the invariant of an active sale: completed tasks form a prefix and at most one task is
// in progress, always the next one after that prefix. The database enforces the same rule at commit.
func CheckShape(statuses []Status) error {
	if len(statuses) != TaskCount {
		return fmt.Errorf("a sale has %d tasks, found %d", TaskCount, len(statuses))
	}
	completed := 0
	for completed < len(statuses) && statuses[completed] == StatusCompleted {
		completed++
	}
	inProgress := 0
	for i := completed; i < len(statuses); i++ {
		switch statuses[i] {
		case StatusCompleted:
			return errors.New("completed tasks must form a prefix of the sequence")
		case StatusInProgress:
			inProgress++
			if i != completed {
				return errors.New("the in-progress task must be the next task after the completed ones")
			}
		}
	}
	if inProgress > 1 {
		return errors.New("at most one task can be in progress")
	}
	return nil
}

// IsEligible reports whether a task can be started now: it is pending, the previous task is completed (task 1
// is eligible from the start) and no task is in progress.
func IsEligible(statuses []Status, task int) bool {
	if task < 1 || task > TaskCount || statuses[task-1] != StatusPending {
		return false
	}
	if task > 1 && statuses[task-2] != StatusCompleted {
		return false
	}
	for _, s := range statuses {
		if s == StatusInProgress {
			return false
		}
	}
	return true
}

// DeriveMilestones computes the state of the six milestones from the 13 task statuses. A milestone is
// completed when all its tasks are, in progress when it holds the current task, and pending otherwise.
func DeriveMilestones(statuses []Status) [MilestoneCount]MilestoneState {
	current := CurrentTask(statuses)
	var out [MilestoneCount]MilestoneState
	for m := 1; m <= MilestoneCount; m++ {
		all, holdsCurrent := true, false
		for task := 1; task <= TaskCount; task++ {
			if MilestoneOf(task) != m {
				continue
			}
			if statuses[task-1] != StatusCompleted {
				all = false
			}
			if task == current {
				holdsCurrent = true
			}
		}
		switch {
		case all:
			out[m-1] = MilestoneCompleted
		case holdsCurrent:
			out[m-1] = MilestoneInProgress
		default:
			out[m-1] = MilestonePending
		}
	}
	return out
}

// DueDate is the start time plus the task's duration.
func DueDate(started time.Time, durationHours int) time.Time {
	return started.Add(time.Duration(durationHours) * time.Hour)
}

// CompletionPhase classifies a completion against the SLA (section 5.5 of the spec). A task without a
// duration is not applicable.
func CompletionPhase(durationHours, reminderHours int, started, completed time.Time) Phase {
	if durationHours <= 0 {
		return PhaseNotApplicable
	}
	elapsed := completed.Sub(started)
	d := time.Duration(durationHours) * time.Hour
	interval := time.Duration(reminderHours) * time.Hour
	switch {
	case elapsed*10 < d*8:
		return PhaseEarly
	case elapsed <= d:
		return PhaseApproaching
	case elapsed < d+interval:
		return PhaseOverdue
	default:
		return PhaseRecurringOverdue
	}
}

// --- who may do what ---

// IsManagement is true for Admins and Managers.
func IsManagement(role auth.Role) bool {
	return role == auth.RoleSystemAdmin || role == auth.RoleSalesManager
}

// CanActOnTask: an Advisor may start, edit and complete tasks only on their own sale; Managers and Admins on
// any sale.
func CanActOnTask(role auth.Role, ownSale bool) bool {
	switch role {
	case auth.RoleSystemAdmin, auth.RoleSalesManager:
		return true
	case auth.RoleSalesAdvisor:
		return ownSale
	}
	return false
}

// CanCancel applies the cancellation rule. current is the number of the current task (1 to 13): the Advisor
// who owns the sale may cancel while it is 1 to 3, a Manager while it is 1 to 6, an Admin at any stage.
func CanCancel(role auth.Role, ownSale bool, current int) bool {
	switch role {
	case auth.RoleSystemAdmin:
		return true
	case auth.RoleSalesManager:
		return current >= 1 && current <= 6
	case auth.RoleSalesAdvisor:
		return ownSale && current >= 1 && current <= 3
	}
	return false
}

// CanReopenSale applies the reopen rule: a Manager only if the sale was cancelled while the current task was
// 1 to 6; an Admin always; an Advisor never.
func CanReopenSale(role auth.Role, cancelledAtTask int) bool {
	switch role {
	case auth.RoleSystemAdmin:
		return true
	case auth.RoleSalesManager:
		return cancelledAtTask >= 1 && cancelledAtTask <= 6
	}
	return false
}

// --- identifiers ---

var (
	vinPattern     = regexp.MustCompile(`^[A-HJ-NPR-Z0-9]{17}$`)
	platePattern   = regexp.MustCompile(`^[A-Z0-9]{6}$`)
	invoicePattern = regexp.MustCompile(`^[A-Z0-9]{4}-[0-9]{1,8}$`)
	spaces         = regexp.MustCompile(`\s+`)
)

// normalizeCode upper-cases and removes all whitespace, the form identifiers are stored in.
func normalizeCode(s string) string { return strings.ToUpper(spaces.ReplaceAllString(s, "")) }

// NormalizeSeries returns the stored form of a vehicle series number and whether it is usable.
func NormalizeSeries(s string) (string, bool) {
	n := normalizeCode(s)
	return n, n != "" && len(n) <= 40
}

// NormalizeVIN returns the stored form of a VIN: 17 characters, without I, O and Q.
func NormalizeVIN(s string) (string, bool) {
	n := normalizeCode(s)
	return n, vinPattern.MatchString(n)
}

// NormalizePlate returns the stored form of a license plate: 6 alphanumeric characters.
func NormalizePlate(s string) (string, bool) {
	n := normalizeCode(s)
	return n, platePattern.MatchString(n)
}

// NormalizeTransaction returns the stored form of a bank transaction number.
func NormalizeTransaction(s string) (string, bool) {
	n := normalizeCode(s)
	return n, n != "" && len(n) <= 40
}

// NormalizeDAM returns the stored form of a customs declaration number.
func NormalizeDAM(s string) (string, bool) {
	n := normalizeCode(s)
	return n, n != "" && len(n) <= 40
}

// NormalizeInvoiceNumber returns the stored form of an invoice number: 4 alphanumeric characters, a hyphen
// and 1 to 8 digits ("F001-000123").
func NormalizeInvoiceNumber(s string) (string, bool) {
	n := normalizeCode(s)
	return n, invoicePattern.MatchString(n)
}

// --- dates ---

// DateProblem explains why a date cannot be accepted.
type DateProblem string

// The date problems that block a save.
const (
	DateInvalid     DateProblem = "invalid"
	DateTooOld      DateProblem = "too_old"
	DateTooFarAhead DateProblem = "too_far_ahead"
)

// ParseDate reads "YYYY-MM-DD". Impossible values are hard-blocked: a date that does not exist, before the
// year 2000, or more than a year after today (almost certainly a typo). A date after today but within a year
// is accepted; the caller adds a warning for it.
func ParseDate(raw string, today time.Time) (time.Time, DateProblem, bool) {
	d, err := time.Parse("2006-01-02", strings.TrimSpace(raw))
	if err != nil {
		return time.Time{}, DateInvalid, false
	}
	if d.Year() < 2000 {
		return time.Time{}, DateTooOld, false
	}
	if d.After(today.AddDate(1, 0, 0)) {
		return time.Time{}, DateTooFarAhead, false
	}
	return d, "", true
}

// Warning is a non-blocking remark returned with a successful save.
type Warning struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// DateEntry is one recorded date and the group (position in the process) it belongs to.
type DateEntry struct {
	Field string
	Group int
	Date  time.Time
}

// Date groups, in process order. A date should not be earlier than any date of an earlier group.
const (
	GroupSaleCreated = iota
	GroupPanaPayment
	GroupToyotaPayment
	GroupInvoices
	GroupRegistration
	GroupPDSTransfer
	GroupEquipment
	GroupSurquillo
	GroupDelivery
)

// ChronologyWarnings returns a warning for every date earlier than the latest date of an earlier group, and
// for every date after today. Warnings never block a save or a completion.
func ChronologyWarnings(entries []DateEntry, today time.Time) []Warning {
	var out []Warning
	for _, e := range entries {
		if e.Group == GroupSaleCreated {
			continue
		}
		var latest time.Time
		for _, other := range entries {
			if other.Group < e.Group && other.Date.After(latest) {
				latest = other.Date
			}
		}
		if !latest.IsZero() && e.Date.Before(latest) {
			out = append(out, Warning{
				Field: e.Field, Code: "DATE_OUT_OF_ORDER",
				Message: "Esta fecha es anterior a la de un paso previo del proceso. Revísala.",
			})
		}
		if e.Date.After(today) {
			out = append(out, Warning{
				Field: e.Field, Code: "DATE_IN_THE_FUTURE",
				Message: "La fecha es posterior a hoy. Revísala.",
			})
		}
	}
	return out
}
