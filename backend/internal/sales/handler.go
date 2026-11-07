package sales

import (
	"endosoflow/internal/auth"
	"endosoflow/internal/customers"
	"endosoflow/internal/platform/apperr"
	"endosoflow/internal/platform/httpx"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Handler serves the sale routes. The task routes of a sale belong to the tasks module.
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
// first line of defense; the service checks again and applies the Advisor scope. The routes are written out
// in full because the tasks module shares the /sales prefix.
func (h *Handler) Mount(r chi.Router) {
	staff := auth.RequireRoles(h.logger, auth.RoleSystemAdmin, auth.RoleSalesManager, auth.RoleSalesAdvisor)
	management := auth.RequireRoles(h.logger, auth.RoleSystemAdmin, auth.RoleSalesManager)
	adminOnly := auth.RequireRoles(h.logger, auth.RoleSystemAdmin)

	r.Group(func(r chi.Router) {
		r.Use(h.auth.Authenticate)
		r.With(staff).Get("/sales", h.list)
		r.With(staff).Post("/sales", h.create)
		r.With(staff).Get("/sales/{id}", h.get)
		r.With(staff).Patch("/sales/{id}", h.update)
		r.With(adminOnly).Delete("/sales/{id}", h.delete)
		r.With(staff).Get("/sales/{id}/history", h.history)
		r.With(management).Post("/sales/{id}/reassign", h.reassign)
		r.With(staff).Post("/sales/{id}/cancel", h.cancel)
		r.With(management).Post("/sales/{id}/reopen", h.reopen)
	})
}

func (h *Handler) actor(r *http.Request) auth.Actor {
	actor, _ := auth.ActorFrom(r.Context())
	return actor
}

func (h *Handler) saleID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, h.logger, errNotFound)
		return uuid.Nil, false
	}
	return id, true
}

func invalidQuery(name string) *apperr.Error {
	return apperr.BadRequest("INVALID_QUERY", "El parámetro "+name+" no es válido.")
}

// intParam reads a positive integer query parameter, or 0 when it is absent.
func intParam(r *http.Request, name string, max int) (int, error) {
	text := r.URL.Query().Get(name)
	if text == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(text)
	if err != nil || value < 1 || (max > 0 && value > max) {
		return 0, invalidQuery(name)
	}
	return value, nil
}

func uuidParam(r *http.Request, name string) (*uuid.UUID, error) {
	text := r.URL.Query().Get(name)
	if text == "" {
		return nil, nil
	}
	id, err := uuid.Parse(text)
	if err != nil {
		return nil, invalidQuery(name)
	}
	return &id, nil
}

var (
	sortFields    = []string{"createdAt", "-createdAt", "customerName", "-customerName"}
	saleStates    = []string{StateActive, StateCompleted, StateCancelled}
	customerTypes = []string{
		customers.TypeDiplomat, customers.TypeEmbassy, customers.TypeCatholicEntity,
		customers.TypeOtherReligious, customers.TypeInternationalOrg,
	}
)

// listFilter reads and validates the query string of the workspace list.
func listFilter(r *http.Request) (ListFilter, error) {
	query := r.URL.Query()
	f := ListFilter{
		Search: query.Get("q"), State: query.Get("state"), CustomerType: query.Get("customerType"),
		Temperature: query.Get("temperature"), Sort: query.Get("sort"),
	}
	var err error
	if f.Page, err = intParam(r, "page", 0); err != nil {
		return f, err
	}
	if f.PageSize, err = intParam(r, "pageSize", 0); err != nil {
		return f, err
	}
	if f.CurrentTask, err = intParam(r, "currentTask", LastTaskNumber); err != nil {
		return f, err
	}
	if f.Milestone, err = intParam(r, "milestone", 6); err != nil {
		return f, err
	}
	if f.AdvisorID, err = uuidParam(r, "advisorId"); err != nil {
		return f, err
	}
	if f.ModelID, err = uuidParam(r, "modelId"); err != nil {
		return f, err
	}
	if f.VersionID, err = uuidParam(r, "versionId"); err != nil {
		return f, err
	}
	switch {
	case f.State != "" && !oneOf(f.State, saleStates):
		return f, invalidQuery("state")
	case f.CustomerType != "" && !oneOf(f.CustomerType, customerTypes):
		return f, invalidQuery("customerType")
	case f.Temperature != "" && !oneOf(f.Temperature, temperatures):
		return f, invalidQuery("temperature")
	case f.Sort != "" && !oneOf(f.Sort, sortFields):
		return f, invalidQuery("sort")
	}
	return f, nil
}

// LastTaskNumber is the number of the final task of a sale.
const LastTaskNumber = 13

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	filter, err := listFilter(r)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
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
	detail, err := h.svc.Create(r.Context(), h.actor(r), body)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, detail)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := h.saleID(w, r)
	if !ok {
		return
	}
	detail, err := h.svc.Get(r.Context(), h.actor(r), id)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, detail)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := h.saleID(w, r)
	if !ok {
		return
	}
	var body UpdateInput
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	detail, err := h.svc.UpdateNotes(r.Context(), h.actor(r), id, body)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, detail)
}

func (h *Handler) history(w http.ResponseWriter, r *http.Request) {
	id, ok := h.saleID(w, r)
	if !ok {
		return
	}
	page, err := intParam(r, "page", 0)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	pageSize, err := intParam(r, "pageSize", 0)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	result, err := h.svc.History(r.Context(), h.actor(r), id, page, pageSize)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) reassign(w http.ResponseWriter, r *http.Request) {
	id, ok := h.saleID(w, r)
	if !ok {
		return
	}
	var body ReassignInput
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	detail, err := h.svc.Reassign(r.Context(), h.actor(r), id, body)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, detail)
}

func (h *Handler) cancel(w http.ResponseWriter, r *http.Request) {
	id, ok := h.saleID(w, r)
	if !ok {
		return
	}
	var body CancelInput
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	detail, err := h.svc.Cancel(r.Context(), h.actor(r), id, body)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, detail)
}

func (h *Handler) reopen(w http.ResponseWriter, r *http.Request) {
	id, ok := h.saleID(w, r)
	if !ok {
		return
	}
	var body ReasonInput
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	detail, err := h.svc.Reopen(r.Context(), h.actor(r), id, body)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, detail)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := h.saleID(w, r)
	if !ok {
		return
	}
	var body DeleteInput
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	if err := h.svc.Delete(r.Context(), h.actor(r), id, body); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
