// Package analytics serves the figures of the BI dashboard. Everything is computed on demand from the
// operational tables in one read-only snapshot per request, so the widgets of a tab always agree. It never
// writes, and it applies the caller's scope itself: an Advisor only ever sees their own sales and never
// the internal money (commission, Toyota payments, amount mismatches).
package analytics

import (
	"context"
	"endosoflow/internal/auth"
	"endosoflow/internal/platform/apperr"
	"endosoflow/internal/platform/database"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Deps are the Service's dependencies.
type Deps struct {
	DB     auth.Database
	Logger *slog.Logger
	Now    func() time.Time
}

type Service struct {
	db     auth.Database
	logger *slog.Logger
	now    func() time.Time
}

func NewService(d Deps) *Service {
	now := d.Now
	if now == nil {
		now = time.Now
	}
	return &Service{db: d.DB, logger: d.Logger, now: now}
}

// Filter is what the caller asked for.
type Filter struct {
	From, To     *time.Time
	AdvisorID    *uuid.UUID
	CustomerType *string
	ModelID      *uuid.UUID
	Temperature  *string
	GroupBy      string // commercial only: model (default), version, color or category
}

// --- who may see what ---

func isStaff(a auth.Actor) bool {
	return a.Role == auth.RoleSystemAdmin || a.Role == auth.RoleSalesManager || a.Role == auth.RoleSalesAdvisor
}

// seesInternalMoney is true for Admins and Managers.
func seesInternalMoney(a auth.Actor) bool {
	return a.Role == auth.RoleSystemAdmin || a.Role == auth.RoleSalesManager
}

var errForbidden = apperr.Forbidden("FORBIDDEN", "No tiene permiso para realizar esta acción.")

// scopeFor builds the scope of a request. An Advisor is pinned to their own sales whatever they filter.
// useFilters false ignores the sales filters (the team and system tabs).
func (s *Service) scopeFor(actor auth.Actor, f Filter, useFilters bool) (scope, error) {
	if !isStaff(actor) {
		return scope{}, errForbidden
	}
	now := s.now()
	period, err := ResolvePeriod(f.From, f.To, now)
	if err != nil {
		return scope{}, err
	}
	sc := scope{period: period, now: now}
	if actor.Role == auth.RoleSalesAdvisor {
		id := actor.ID
		sc.advisor = &id
		sc.own = true
	} else if useFilters {
		sc.advisor = f.AdvisorID
	}
	if useFilters {
		sc.customerType, sc.model, sc.temperature = f.CustomerType, f.ModelID, f.Temperature
	}
	return sc, nil
}

func (s *Service) meta(sc scope) Meta {
	scopeName := "global"
	if sc.own {
		scopeName = "own"
	}
	return Meta{
		From: sc.period.From.Format(dateLayout), To: sc.period.To.Format(dateLayout),
		PreviousFrom: sc.period.PrevFrom.Format(dateLayout), PreviousTo: sc.period.PrevTo.Format(dateLayout),
		Granularity: sc.period.Granularity, Scope: scopeName, GeneratedAt: sc.now.UTC(),
	}
}

// read runs fn in one read-only snapshot, so every figure of a response is consistent with the others.
func (s *Service) read(ctx context.Context, fn func(q querier) error) error {
	return database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SET TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY`); err != nil {
			return err
		}
		return fn(tx)
	})
}
