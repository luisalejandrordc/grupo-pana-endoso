package customers

import (
	"endosoflow/internal/auth"
	"endosoflow/internal/platform/apperr"
	"endosoflow/internal/platform/httpx"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Handler serves the customer and lookup routes.
type Handler struct {
	svc    *Service
	auth   *auth.Handler
	logger *slog.Logger
}

// NewHandler builds the handler. The auth handler provides the Authenticate middleware.
func NewHandler(svc *Service, authHandler *auth.Handler, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, auth: authHandler, logger: logger}
}

// Mount registers the routes under the given router (already under /api/v1). The role gates here are the
// first line of defense; the service checks again and applies the Advisor scope.
func (h *Handler) Mount(r chi.Router) {
	staff := auth.RequireRoles(h.logger, auth.RoleSystemAdmin, auth.RoleSalesManager, auth.RoleSalesAdvisor)
	management := auth.RequireRoles(h.logger, auth.RoleSystemAdmin, auth.RoleSalesManager)
	adminOnly := auth.RequireRoles(h.logger, auth.RoleSystemAdmin)

	r.Group(func(r chi.Router) {
		r.Use(h.auth.Authenticate)

		r.With(staff).Get("/lookups/identity", h.lookupIdentity)
		r.With(adminOnly).Get("/lookups/identity/quota", h.identityQuota)

		r.Route("/customers", func(r chi.Router) {
			r.Use(staff)
			r.Get("/", h.list)
			r.Post("/", h.create)
			r.Get("/lookup", h.lookup)
			r.Get("/{id}", h.get)
			r.Patch("/{id}", h.update)
			r.With(management).Post("/{id}/deactivate", h.deactivate)
			r.With(management).Post("/{id}/reactivate", h.reactivate)
			r.With(management).Post("/{id}/resend-activation", h.resendActivation)
			r.With(adminOnly).Delete("/{id}", h.delete)
		})
	})
}

func (h *Handler) actor(r *http.Request) auth.Actor {
	actor, _ := auth.ActorFrom(r.Context())
	return actor
}

func (h *Handler) customerID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, h.logger, errNotFound)
		return uuid.Nil, false
	}
	return id, true
}

func intParam(r *http.Request, name string, fallback int) (int, error) {
	text := r.URL.Query().Get(name)
	if text == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(text)
	if err != nil || value < 1 {
		return 0, apperr.BadRequest("INVALID_QUERY", "El parámetro "+name+" no es válido.")
	}
	return value, nil
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	page, err := intParam(r, "page", 1)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	pageSize, err := intParam(r, "pageSize", 20)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	query := r.URL.Query()
	filter := ListFilter{
		Search: query.Get("q"), CustomerType: query.Get("customerType"), Status: query.Get("status"),
		Page: page, PageSize: pageSize, Sort: query.Get("sort"),
	}
	if filter.CustomerType != "" && !isCustomerType(filter.CustomerType) {
		httpx.WriteError(w, r, h.logger, apperr.BadRequest("INVALID_QUERY", "El tipo de cliente indicado no es válido."))
		return
	}
	switch filter.Status {
	case "", "all", "active", "inactive":
	default:
		httpx.WriteError(w, r, h.logger, apperr.BadRequest("INVALID_QUERY", "El estado indicado no es válido."))
		return
	}
	result, err := h.svc.List(r.Context(), h.actor(r), filter)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var body CreateInput
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	customer, err := h.svc.Create(r.Context(), h.actor(r), body)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, customer)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := h.customerID(w, r)
	if !ok {
		return
	}
	customer, err := h.svc.Get(r.Context(), h.actor(r), id)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, customer)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := h.customerID(w, r)
	if !ok {
		return
	}
	var body UpdateInput
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	customer, err := h.svc.Update(r.Context(), h.actor(r), id, body)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, customer)
}

func (h *Handler) deactivate(w http.ResponseWriter, r *http.Request) {
	id, ok := h.customerID(w, r)
	if !ok {
		return
	}
	customer, err := h.svc.Deactivate(r.Context(), h.actor(r), id)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, customer)
}

func (h *Handler) resendActivation(w http.ResponseWriter, r *http.Request) {
	id, ok := h.customerID(w, r)
	if !ok {
		return
	}
	if err := h.svc.ResendActivation(r.Context(), h.actor(r), id); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	// Accepted, not "done": the email is queued and sent within moments.
	httpx.WriteJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

func (h *Handler) reactivate(w http.ResponseWriter, r *http.Request) {
	id, ok := h.customerID(w, r)
	if !ok {
		return
	}
	customer, err := h.svc.Reactivate(r.Context(), h.actor(r), id)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, customer)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := h.customerID(w, r)
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), h.actor(r), id, r.URL.Query().Get("reason")); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) lookup(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	result, err := h.svc.Lookup(r.Context(), h.actor(r), query.Get("documentType"), query.Get("documentNumber"))
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) lookupIdentity(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	result, err := h.svc.LookupIdentity(r.Context(), h.actor(r), query.Get("documentType"), query.Get("documentNumber"))
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) identityQuota(w http.ResponseWriter, r *http.Request) {
	status, err := h.svc.IdentityQuota(r.Context(), h.actor(r))
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, status)
}
