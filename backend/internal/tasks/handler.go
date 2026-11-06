package tasks

import (
	"endosoflow/internal/auth"
	"endosoflow/internal/platform/httpx"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Handler serves the task routes of a sale.
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
// in full because the sales module owns the /sales prefix.
func (h *Handler) Mount(r chi.Router) {
	staff := auth.RequireRoles(h.logger, auth.RoleSystemAdmin, auth.RoleSalesManager, auth.RoleSalesAdvisor)
	management := auth.RequireRoles(h.logger, auth.RoleSystemAdmin, auth.RoleSalesManager)

	r.Group(func(r chi.Router) {
		r.Use(h.auth.Authenticate)
		r.With(staff).Get("/sales/{id}/tasks", h.list)
		r.With(staff).Get("/sales/{id}/tasks/{n}", h.get)
		r.With(staff).Patch("/sales/{id}/tasks/{n}/data", h.saveData)
		r.With(staff).Patch("/sales/{id}/tasks/{n}/notes", h.saveNotes)
		r.With(staff).Post("/sales/{id}/tasks/{n}/start", h.start)
		r.With(staff).Post("/sales/{id}/tasks/{n}/complete", h.complete)
		r.With(management).Post("/sales/{id}/tasks/{n}/restart", h.restart)
		r.With(management).Post("/sales/{id}/tasks/{n}/reopen", h.reopen)
	})
}

func (h *Handler) actor(r *http.Request) auth.Actor {
	actor, _ := auth.ActorFrom(r.Context())
	return actor
}

func (h *Handler) saleID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, h.logger, errSaleNotFound)
		return uuid.Nil, false
	}
	return id, true
}

// ref reads the sale id and the task number from the path.
func (h *Handler) ref(w http.ResponseWriter, r *http.Request) (uuid.UUID, int, bool) {
	id, ok := h.saleID(w, r)
	if !ok {
		return uuid.Nil, 0, false
	}
	number, err := strconv.Atoi(chi.URLParam(r, "n"))
	if err != nil {
		httpx.WriteError(w, r, h.logger, errTaskNotFound)
		return uuid.Nil, 0, false
	}
	return id, number, true
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	id, ok := h.saleID(w, r)
	if !ok {
		return
	}
	tasks, err := h.svc.List(r.Context(), h.actor(r), id)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": tasks})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, number, ok := h.ref(w, r)
	if !ok {
		return
	}
	detail, err := h.svc.Get(r.Context(), h.actor(r), id, number)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, detail)
}

func (h *Handler) saveData(w http.ResponseWriter, r *http.Request) {
	id, number, ok := h.ref(w, r)
	if !ok {
		return
	}
	var body SaveInput
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	result, err := h.svc.SaveData(r.Context(), h.actor(r), id, number, body)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) saveNotes(w http.ResponseWriter, r *http.Request) {
	id, number, ok := h.ref(w, r)
	if !ok {
		return
	}
	var body NotesInput
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	detail, err := h.svc.SaveNotes(r.Context(), h.actor(r), id, number, body.Notes)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, detail)
}

func (h *Handler) start(w http.ResponseWriter, r *http.Request) {
	id, number, ok := h.ref(w, r)
	if !ok {
		return
	}
	detail, err := h.svc.Start(r.Context(), h.actor(r), id, number)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, detail)
}

func (h *Handler) complete(w http.ResponseWriter, r *http.Request) {
	id, number, ok := h.ref(w, r)
	if !ok {
		return
	}
	detail, err := h.svc.Complete(r.Context(), h.actor(r), id, number)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, detail)
}

func (h *Handler) restart(w http.ResponseWriter, r *http.Request) {
	id, number, ok := h.ref(w, r)
	if !ok {
		return
	}
	var body ReasonInput
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	detail, err := h.svc.Restart(r.Context(), h.actor(r), id, number, body.Reason)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, detail)
}

func (h *Handler) reopen(w http.ResponseWriter, r *http.Request) {
	id, number, ok := h.ref(w, r)
	if !ok {
		return
	}
	var body ReasonInput
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	detail, err := h.svc.Reopen(r.Context(), h.actor(r), id, number, body.Reason)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, detail)
}
