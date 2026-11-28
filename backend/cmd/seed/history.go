package main

import (
	"endosoflow/internal/auth"
	"endosoflow/internal/documents"
	"endosoflow/internal/notifications"
	"endosoflow/internal/sales"
	"endosoflow/internal/tasks"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"time"
)

// fakeClock is the time the services see while the history is built.
type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time { return c.t }

// pricing holds the amounts of one sale in cents, so the figures that the tasks record stay consistent with
// each other: the payment to Grupo Pana equals the quoted price, and the commission invoice equals what is
// left of the initial Toyota amount and commission once the final Toyota amount is paid.
type pricing struct{ toyota, commission, accessories, release, finalToyota int }

func (p pricing) quoted() int          { return p.toyota + p.commission + p.accessories + p.release }
func (p pricing) finalCommission() int { return p.toyota + p.commission - p.finalToyota }

func cents(n int) string { return fmt.Sprintf("%d.%02d", n/100, n%100) }

func newPricing(rng *rand.Rand) *pricing {
	round := func(n, step int) int { return n / step * step }
	p := &pricing{
		toyota:      round(2_400_000+rng.IntN(2_800_000), 10_000),
		commission:  round(150_000+rng.IntN(300_000), 5_000),
		accessories: round(60_000+rng.IntN(260_000), 5_000),
		release:     round(60_000+rng.IntN(60_000), 5_000),
	}
	p.finalToyota = p.toyota + round(rng.IntN(100_000), 5_000) - round(rng.IntN(60_000), 5_000)
	return p
}

var cancellationCategories = []string{
	"customer_withdrew", "payment_or_financing_failed", "unit_unavailable", "due_diligence_finding", "registration_error", "other",
}

// history creates about fifty sales through the real services with a backdated clock: they start up to eleven
// months ago, move at human speed (waiting between tasks, working for a fraction of a task's deadline or
// sometimes well beyond it) and end completed, cancelled or still in progress. The clock never passes the
// present. The choices come from a fixed random sequence, so the shape of the data is the same every time.
func (s *seeder) history() error {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	clk := &fakeClock{}
	now := time.Now()
	rng := rand.New(rand.NewPCG(2026, 11))

	h := *s
	h.clk = clk
	h.docs = documents.NewService(documents.Deps{DB: s.pool, Store: s.store, Sales: sales.Gate{}, Logger: logger, Now: clk.Now})
	notifier := notifications.Writer{}
	h.sales = sales.NewService(sales.Deps{DB: s.pool, Images: s.store, Documents: h.docs, DocumentStore: s.store, Notifier: notifier, Logger: logger, Now: clk.Now})
	h.tasks = tasks.NewService(tasks.Deps{DB: s.pool, Sales: sales.Gate{}, Documents: h.docs, Notifier: notifier, Logger: logger, Now: clk.Now})

	deadlines := map[int]int{}
	rows, err := s.pool.Query(s.ctx, `SELECT task_number, recommended_duration_hours FROM task_definitions WHERE recommended_duration_hours IS NOT NULL`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var n, hours int
		if err := rows.Scan(&n, &hours); err != nil {
			rows.Close()
			return err
		}
		deadlines[n] = hours
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	// The clock moves between steps: waiting before a task starts, working before it is completed.
	h.tick = func(kind string, task int) {
		var hours float64
		switch {
		case kind == "wait" && rng.Float64() < 0.12:
			hours = 40 + rng.Float64()*60 // a sale left sitting
		case kind == "wait":
			hours = 1 + rng.Float64()*26
		case deadlines[task] > 0 && rng.Float64() < 0.2:
			hours = float64(deadlines[task]) * (1.1 + rng.Float64()*1.6) // finished late
		case deadlines[task] > 0:
			hours = float64(deadlines[task]) * (0.25 + rng.Float64()*0.7)
		default:
			hours = 1 + rng.Float64()*24
		}
		next := clk.t.Add(time.Duration(hours * float64(time.Hour)))
		if next.After(now) {
			next = now
		}
		clk.t = next
	}

	temperatures := []string{"hot", "warm", "warm", "cold", "hot"}
	pComplete := map[string]float64{"hot": 0.70, "warm": 0.50, "cold": 0.30}
	pCancel := map[string]float64{"hot": 0.10, "warm": 0.20, "cold": 0.35}
	manager := auth.Actor{ID: s.manager.id, Role: s.manager.role}

	for i := 0; i < 50; i++ {
		age := 3 + rng.IntN(330)
		day := now.AddDate(0, 0, -age)
		clk.t = time.Date(day.Year(), day.Month(), day.Day(), 14+rng.IntN(8), rng.IntN(60), 0, 0, time.UTC) // 9:00 to 16:59 in Lima
		temp := temperatures[rng.IntN(len(temperatures))]
		advisor := s.advisors[rng.IntN(len(s.advisors))]
		car := s.cars[rng.IntN(len(s.cars))]
		client := s.clients[rng.IntN(len(s.clients))]

		// A sale needs weeks to finish and days to be cancelled, so recent ones can only be in progress.
		roll := rng.Float64()
		outcome := "progress"
		switch {
		case age >= 45 && roll < pComplete[temp]:
			outcome = "completed"
		case age >= 12 && roll < pComplete[temp]+pCancel[temp]:
			outcome = "cancelled"
		case age >= 150:
			outcome = "completed" // an old sale is never still open
		}

		h.price = newPricing(rng)
		sale, err := h.sales.Create(s.ctx, manager, sales.CreateInput{
			CustomerID: client, VersionID: car.version, ColorID: car.color, AdvisorID: &advisor.id, Temperature: temp,
		})
		if err != nil {
			return fmt.Errorf("history sale %d: create: %w", i, err)
		}
		switch outcome {
		case "completed":
			err = h.work(sale.ID, advisor, 13, false)
		case "cancelled":
			if err = h.work(sale.ID, advisor, rng.IntN(5), false); err == nil {
				h.pause("wait", 0)
				_, err = h.sales.Cancel(s.ctx, manager, sale.ID, cancellationCategories[rng.IntN(len(cancellationCategories))], "Cancelada durante el relleno del historial.")
			}
		default:
			err = h.work(sale.ID, advisor, rng.IntN(min(13, 2+age/4)), true)
		}
		if err != nil {
			return fmt.Errorf("history sale %d (%s): %w", i, outcome, err)
		}
	}
	return nil
}
