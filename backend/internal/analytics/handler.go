package analytics

import (
	"endosoflow/internal/auth"
	"endosoflow/internal/platform/apperr"
	"endosoflow/internal/platform/httpx"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Handler serves the analytics routes.
type Handler struct {
	svc    *Service
	auth   *auth.Handler
	logger *slog.Logger
}

// NewHandler builds the handler.
func NewHandler(svc *Service, authHandler *auth.Handler, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, auth: authHandler, logger: logger}
}

// Mount registers the routes under the given router (already under /api/v1). The roles are checked here and
// again in the service, which also scopes what an Advisor can see.
func (h *Handler) Mount(r chi.Router) {
	staff := auth.RequireRoles(h.logger, auth.RoleSystemAdmin, auth.RoleSalesManager, auth.RoleSalesAdvisor)
	management := auth.RequireRoles(h.logger, auth.RoleSystemAdmin, auth.RoleSalesManager)
	admin := auth.RequireRoles(h.logger, auth.RoleSystemAdmin)
	r.Group(func(r chi.Router) {
		r.Use(h.auth.Authenticate)
		r.With(staff).Get("/analytics/overview", h.overview)
		r.With(staff).Get("/analytics/process", h.process)
		r.With(staff).Get("/analytics/commercial", h.commercial)
		r.With(management).Get("/analytics/team", h.team)
		r.With(admin).Get("/analytics/system", h.system)
	})
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	httpx.WriteError(w, r, h.logger, err)
}

func (h *Handler) overview(w http.ResponseWriter, r *http.Request) {
	h.serve(w, r, func(actor auth.Actor, f Filter) (any, error) { return h.svc.Overview(r.Context(), actor, f) })
}

func (h *Handler) process(w http.ResponseWriter, r *http.Request) {
	h.serve(w, r, func(actor auth.Actor, f Filter) (any, error) { return h.svc.Process(r.Context(), actor, f) })
}

func (h *Handler) commercial(w http.ResponseWriter, r *http.Request) {
	h.serve(w, r, func(actor auth.Actor, f Filter) (any, error) { return h.svc.Commercial(r.Context(), actor, f) })
}

func (h *Handler) team(w http.ResponseWriter, r *http.Request) {
	h.serve(w, r, func(actor auth.Actor, f Filter) (any, error) { return h.svc.Team(r.Context(), actor, f) })
}

func (h *Handler) system(w http.ResponseWriter, r *http.Request) {
	h.serve(w, r, func(actor auth.Actor, f Filter) (any, error) { return h.svc.System(r.Context(), actor, f) })
}

// serve parses the query, runs the call and writes the answer.
func (h *Handler) serve(w http.ResponseWriter, r *http.Request, call func(auth.Actor, Filter) (any, error)) {
	actor, ok := auth.ActorFrom(r.Context())
	if !ok {
		h.fail(w, r, apperr.Unauthorized("ACCESS_TOKEN_INVALID", "Su sesión expiró. Inicie sesión nuevamente."))
		return
	}
	f, err := parseFilter(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out, err := call(actor, f)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func parseFilter(r *http.Request) (Filter, error) {
	var f Filter
	var err error
	if f.From, err = queryDate(r, "from"); err != nil {
		return f, err
	}
	if f.To, err = queryDate(r, "to"); err != nil {
		return f, err
	}
	if f.AdvisorID, err = queryUUID(r, "advisorId"); err != nil {
		return f, err
	}
	if f.ModelID, err = queryUUID(r, "modelId"); err != nil {
		return f, err
	}
	if f.CustomerType, err = oneOf(r, "customerType", "diplomatico", "embajada", "entidad_religiosa_catolica", "entidad_religiosa_no_catolica", "organismo_internacional"); err != nil {
		return f, err
	}
	if f.Temperature, err = oneOf(r, "temperature", "hot", "warm", "cold"); err != nil {
		return f, err
	}
	groupBy, err := oneOf(r, "groupBy", "model", "version", "color", "category")
	if err != nil {
		return f, err
	}
	if groupBy != nil {
		f.GroupBy = *groupBy
	}
	return f, nil
}

func queryDate(r *http.Request, name string) (*time.Time, error) {
	text := r.URL.Query().Get(name)
	if text == "" {
		return nil, nil
	}
	d, err := time.Parse(dateLayout, text)
	if err != nil {
		return nil, apperr.BadRequest("INVALID_QUERY", "El parámetro "+name+" no es válido.")
	}
	return &d, nil
}

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

// oneOf returns nil when the parameter is absent and an error when it is not one of the allowed values.
func oneOf(r *http.Request, name string, allowed ...string) (*string, error) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return nil, nil
	}
	for _, a := range allowed {
		if v == a {
			return &v, nil
		}
	}
	return nil, apperr.BadRequest("INVALID_QUERY", "El parámetro "+name+" no es válido.")
}
