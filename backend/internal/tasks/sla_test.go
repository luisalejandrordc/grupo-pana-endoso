package tasks

import (
	"testing"
	"time"
)

func TestDueDateFor(t *testing.T) {
	start := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	got := dueDateFor(start, 72)
	if want := start.Add(72 * time.Hour); !got.Equal(want) {
		t.Fatalf("dueDateFor = %v, want %v", got, want)
	}
}

func TestCompletionPhase(t *testing.T) {
	start := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	duration, interval := 100, 24
	due := dueDateFor(start, duration)

	cases := []struct {
		name string
		at   time.Time
		want string
	}{
		{"right after the start", start, PhaseEarly},
		{"just before 80%", start.Add(80*time.Hour - time.Second), PhaseEarly},
		{"exactly 80%", start.Add(80 * time.Hour), PhaseInApproaching},
		{"between 80% and the due date", start.Add(95 * time.Hour), PhaseInApproaching},
		{"exactly at the due date", due, PhaseInApproaching},
		{"one second late", due.Add(time.Second), PhaseOverdue},
		{"just under one interval late", due.Add(24*time.Hour - time.Second), PhaseOverdue},
		{"exactly one interval late", due.Add(24 * time.Hour), PhaseRecurringOverdue},
		{"many intervals late", due.Add(10 * 24 * time.Hour), PhaseRecurringOverdue},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := completionPhase(c.at, &due, &duration, &interval); got != c.want {
				t.Fatalf("completionPhase = %q, want %q", got, c.want)
			}
		})
	}
}

func TestCompletionPhaseWithoutDuration(t *testing.T) {
	now := time.Now()
	if got := completionPhase(now, nil, nil, nil); got != PhaseNotApplicable {
		t.Fatalf("no duration: got %q", got)
	}
	hours := 10
	if got := completionPhase(now, nil, &hours, &hours); got != PhaseNotApplicable {
		t.Fatalf("no due date: got %q", got)
	}
}

// A due date that moved (a cancelled sale that was reopened) moves the 80% point with it.
func TestCompletionPhaseFollowsAShiftedDueDate(t *testing.T) {
	start := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	duration, interval := 100, 24
	due := dueDateFor(start, duration).Add(48 * time.Hour) // two days spent cancelled
	if got := completionPhase(start.Add(120*time.Hour), &due, &duration, &interval); got != PhaseEarly {
		t.Fatalf("120h in with a due date 48h later: got %q, want early", got)
	}
	if got := completionPhase(start.Add(130*time.Hour), &due, &duration, &interval); got != PhaseInApproaching {
		t.Fatalf("130h in: got %q, want in_approaching", got)
	}
}
