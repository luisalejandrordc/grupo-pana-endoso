package sales

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

// Handler serves the sale routes.
type Handler struct {
	svc    *Service
	auth   *auth.Handler
	logger *slog.Logger
}

// NewHandler builds the handler.
func NewHandler(svc *Service, authHandler *auth.Handler, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, auth: authHandler, logger: logger}
}

// Mount registers the routes under the given router (already under /api/v1). They are written out in full so
// they sit next to the task routes of the same sale. The service decides what each role can do.
func (h *Handler) Mount(r chi.Router) {
	staff := auth.RequireRoles(h.logger, auth.RoleSystemAdmin, auth.RoleSalesManager, auth.RoleSalesAdvisor)
	r.Group(func(r chi.Router) {
		r.Use(h.auth.Authenticate)
		r.Use(staff)
		r.Get("/sales", h.list)
		r.Post("/sales", h.create)
		r.Get("/sales/{id}", h.get)
		r.Patch("/sales/{id}", h.updateNotes)
		r.Delete("/sales/{id}", h.delete)
		r.Get("/sales/{id}/history", h.history)
		r.Post("/sales/{id}/cancel", h.cancel)
		r.Post("/sales/{id}/reopen", h.reopen)
		r.Post("/sales/{id}/reassign", h.reassign)
	})
}

func (h *Handler) actor(r *http.Request) auth.Actor {
	a, _ := auth.ActorFrom(r.Context())
	return a
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	httpx.WriteError(w, r, h.logger, err)
}

func (h *Handler) id(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.fail(w, r, errNotFound)
		return uuid.Nil, false
	}
	return id, true
}

func queryInt(r *http.Request, name string, fallback, max int) (int, error) {
	text := r.URL.Query().Get(name)
	if text == "" {
		return fallback, nil
	}
	v, err := strconv.Atoi(text)
	if err != nil || v < 1 || (max > 0 && v > max) {
		return 0, apperr.BadRequest("INVALID_QUERY", "El parámetro "+name+" no es válido.")
	}
	return v, nil
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

func oneOf(r *http.Request, name string, allowed ...string) (string, error) {
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

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	var f ListFilter
	var err error
	if f.Page, err = queryInt(r, "page", 1, 0); err != nil {
		h.fail(w, r, err)
		return
	}
	if f.PageSize, err = queryInt(r, "pageSize", 20, 100); err != nil {
		h.fail(w, r, err)
		return
	}
	q := r.URL.Query()
	f.Search, f.Sort = q.Get("q"), q.Get("sort")
	if f.State, err = oneOf(r, "state", "active", "completed", "cancelled"); err != nil {
		h.fail(w, r, err)
		return
	}
	if f.CustomerType, err = oneOf(r, "customerType", "diplomatico", "embajada", "entidad_religiosa_catolica", "entidad_religiosa_no_catolica", "organismo_internacional"); err != nil {
		h.fail(w, r, err)
		return
	}
	if f.Temperature, err = oneOf(r, "temperature", "hot", "warm", "cold"); err != nil {
		h.fail(w, r, err)
		return
	}
	if q.Get("currentTask") != "" {
		if f.CurrentTask, err = queryInt(r, "currentTask", 0, 13); err != nil {
			h.fail(w, r, err)
			return
		}
	}
	if q.Get("milestone") != "" {
		if f.Milestone, err = queryInt(r, "milestone", 0, 6); err != nil {
			h.fail(w, r, err)
			return
		}
	}
	if f.AdvisorID, err = queryUUID(r, "advisorId"); err != nil {
		h.fail(w, r, err)
		return
	}
	if f.ModelID, err = queryUUID(r, "modelId"); err != nil {
		h.fail(w, r, err)
		return
	}
	result, err := h.svc.List(r.Context(), h.actor(r), f)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in CreateInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		h.fail(w, r, err)
		return
	}
	sale, err := h.svc.Create(r.Context(), h.actor(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, sale)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := h.id(w, r)
	if !ok {
		return
	}
	sale, err := h.svc.Get(r.Context(), h.actor(r), id)
	h.reply(w, r, sale, err)
}

func (h *Handler) reply(w http.ResponseWriter, r *http.Request, sale *Sale, err error) {
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, sale)
}

func (h *Handler) updateNotes(w http.ResponseWriter, r *http.Request) {
	id, ok := h.id(w, r)
	if !ok {
		return
	}
	var body struct {
		Version *int   `json:"version"`
		Notes   string `json:"notes"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		h.fail(w, r, err)
		return
	}
	sale, err := h.svc.UpdateNotes(r.Context(), h.actor(r), id, body.Version, body.Notes)
	h.reply(w, r, sale, err)
}

func (h *Handler) history(w http.ResponseWriter, r *http.Request) {
	id, ok := h.id(w, r)
	if !ok {
		return
	}
	page, err := queryInt(r, "page", 1, 0)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	size, err := queryInt(r, "pageSize", 50, 100)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	result, err := h.svc.History(r.Context(), h.actor(r), id, page, size)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) cancel(w http.ResponseWriter, r *http.Request) {
	id, ok := h.id(w, r)
	if !ok {
		return
	}
	var body struct {
		Category string `json:"category"`
		Reason   string `json:"reason"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		h.fail(w, r, err)
		return
	}
	sale, err := h.svc.Cancel(r.Context(), h.actor(r), id, body.Category, body.Reason)
	h.reply(w, r, sale, err)
}

func (h *Handler) reopen(w http.ResponseWriter, r *http.Request) {
	id, ok := h.id(w, r)
	if !ok {
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		h.fail(w, r, err)
		return
	}
	sale, err := h.svc.Reopen(r.Context(), h.actor(r), id, body.Reason)
	h.reply(w, r, sale, err)
}

func (h *Handler) reassign(w http.ResponseWriter, r *http.Request) {
	id, ok := h.id(w, r)
	if !ok {
		return
	}
	var body struct {
		AdvisorID *uuid.UUID `json:"advisorId"`
		Reason    string     `json:"reason"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		h.fail(w, r, err)
		return
	}
	if body.AdvisorID == nil {
		h.fail(w, r, validationError("advisorId", "REQUIRED", "Elige el asesor."))
		return
	}
	sale, err := h.svc.Reassign(r.Context(), h.actor(r), id, *body.AdvisorID, body.Reason)
	h.reply(w, r, sale, err)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := h.id(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	if err := h.svc.Delete(r.Context(), h.actor(r), id, q.Get("reason"), q.Get("confirm")); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
