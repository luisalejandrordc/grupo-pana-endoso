package dashboard

import (
	"endosoflow/internal/auth"
	"endosoflow/internal/platform/apperr"
	"endosoflow/internal/platform/httpx"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Handler serves the dashboard routes.
type Handler struct {
	svc    *Service
	auth   *auth.Handler
	logger *slog.Logger
}

// NewHandler builds the handler.
func NewHandler(svc *Service, authHandler *auth.Handler, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, auth: authHandler, logger: logger}
}

// Mount registers the routes under the given router (already under /api/v1). Every tab is one read-only GET.
// The service decides what each role sees; the two tabs that are closed to Advisors are also closed here.
func (h *Handler) Mount(r chi.Router) {
	staff := auth.RequireRoles(h.logger, auth.RoleSystemAdmin, auth.RoleSalesManager, auth.RoleSalesAdvisor)
	leadership := auth.RequireRoles(h.logger, auth.RoleSystemAdmin, auth.RoleSalesManager)
	admin := auth.RequireRoles(h.logger, auth.RoleSystemAdmin)
	r.Group(func(r chi.Router) {
		r.Use(h.auth.Authenticate)
		r.With(staff).Get("/dashboard/summary", h.summary)
		r.With(staff).Get("/dashboard/process", h.process)
		r.With(staff).Get("/dashboard/market", h.market)
		r.With(staff).Get("/dashboard/finance", h.finance)
		r.With(leadership).Get("/dashboard/advisors", h.advisors)
		r.With(admin).Get("/dashboard/system", h.system)
	})
}

func (h *Handler) actor(r *http.Request) auth.Actor {
	a, _ := auth.ActorFrom(r.Context())
	return a
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	httpx.WriteError(w, r, h.logger, err)
}

var (
	customerTypes = []string{"diplomatico", "embajada", "entidad_religiosa_catolica", "entidad_religiosa_no_catolica", "organismo_internacional"}
	temperatures  = []string{"hot", "warm", "cold"}
)

func queryUUID(r *http.Request, name string) (*uuid.UUID, error) {
	text := r.URL.Query().Get(name)
	if text == "" {
		return nil, nil
	}
	id, err := uuid.Parse(text)
	if err != nil {
		return nil, apperr.BadRequest("INVALID_QUERY", "El parámetro "+name+" no es válido.")
	}
	return &id, nil
}

func oneOf(r *http.Request, name string, allowed []string) (string, error) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return "", nil
	}
	for _, a := range allowed {
		if v == a {
			return v, nil
		}
	}
	return "", apperr.BadRequest("INVALID_QUERY", "El parámetro "+name+" no es válido.")
}

// parseFilter reads the query every tab shares. The dates are checked when the period is resolved.
func parseFilter(r *http.Request) (Filter, error) {
	q := r.URL.Query()
	f := Filter{From: q.Get("from"), To: q.Get("to")}
	var err error
	if f.AdvisorID, err = queryUUID(r, "advisorId"); err != nil {
		return f, err
	}
	if f.ModelID, err = queryUUID(r, "modelId"); err != nil {
		return f, err
	}
	if f.VersionID, err = queryUUID(r, "versionId"); err != nil {
		return f, err
	}
	if f.CustomerType, err = oneOf(r, "customerType", customerTypes); err != nil {
		return f, err
	}
	if f.Temperature, err = oneOf(r, "temperature", temperatures); err != nil {
		return f, err
	}
	return f, nil
}

// serve is the whole of every route: read the filter, ask the service, write the answer.
func serve[T any](h *Handler, w http.ResponseWriter, r *http.Request, get func(*Service, auth.Actor, Filter) (*T, error)) {
	f, err := parseFilter(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	result, err := get(h.svc, h.actor(r), f)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) summary(w http.ResponseWriter, r *http.Request) {
	serve(h, w, r, func(s *Service, a auth.Actor, f Filter) (*Summary, error) { return s.Summary(r.Context(), a, f) })
}

func (h *Handler) process(w http.ResponseWriter, r *http.Request) {
	serve(h, w, r, func(s *Service, a auth.Actor, f Filter) (*Process, error) { return s.Process(r.Context(), a, f) })
}

func (h *Handler) market(w http.ResponseWriter, r *http.Request) {
	serve(h, w, r, func(s *Service, a auth.Actor, f Filter) (*Market, error) { return s.Market(r.Context(), a, f) })
}

func (h *Handler) finance(w http.ResponseWriter, r *http.Request) {
	serve(h, w, r, func(s *Service, a auth.Actor, f Filter) (*Finance, error) { return s.Finance(r.Context(), a, f) })
}

func (h *Handler) advisors(w http.ResponseWriter, r *http.Request) {
	serve(h, w, r, func(s *Service, a auth.Actor, f Filter) (*Advisors, error) { return s.Advisors(r.Context(), a, f) })
}

func (h *Handler) system(w http.ResponseWriter, r *http.Request) {
	serve(h, w, r, func(s *Service, a auth.Actor, f Filter) (*System, error) { return s.System(r.Context(), a, f) })
}
