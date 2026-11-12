package tasks

import (
	"encoding/json"
	"endosoflow/internal/auth"
	"endosoflow/internal/platform/apperr"
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

// NewHandler builds the handler.
func NewHandler(svc *Service, authHandler *auth.Handler, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, auth: authHandler, logger: logger}
}

// Mount registers the routes under the given router (already under /api/v1). The routes are written out in
// full rather than as a sub-router so they sit next to the sales routes without clashing. Every role that
// works with sales may call them; the service decides what each role can do.
func (h *Handler) Mount(r chi.Router) {
	staff := auth.RequireRoles(h.logger, auth.RoleSystemAdmin, auth.RoleSalesManager, auth.RoleSalesAdvisor)
	r.Group(func(r chi.Router) {
		r.Use(h.auth.Authenticate)
		r.Use(staff)
		r.Get("/sales/{id}/tasks", h.list)
		r.Get("/sales/{id}/tasks/{number}", h.get)
		r.Post("/sales/{id}/tasks/{number}/start", h.start)
		r.Post("/sales/{id}/tasks/{number}/complete", h.complete)
		r.Post("/sales/{id}/tasks/{number}/restart", h.restart)
		r.Post("/sales/{id}/tasks/{number}/reopen", h.reopen)
		r.Patch("/sales/{id}/tasks/{number}/data", h.saveData)
		r.Patch("/sales/{id}/tasks/{number}/notes", h.saveNotes)
	})
}

func (h *Handler) actor(r *http.Request) auth.Actor {
	a, _ := auth.ActorFrom(r.Context())
	return a
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	httpx.WriteError(w, r, h.logger, err)
}

func (h *Handler) ids(w http.ResponseWriter, r *http.Request) (uuid.UUID, int, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.fail(w, r, errSaleNotFound)
		return uuid.Nil, 0, false
	}
	n, err := strconv.Atoi(chi.URLParam(r, "number"))
	if err != nil || n < 1 || n > TaskCount {
		h.fail(w, r, errTaskNotFound)
		return uuid.Nil, 0, false
	}
	return id, n, true
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.fail(w, r, errSaleNotFound)
		return
	}
	items, err := h.svc.List(r.Context(), h.actor(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, n, ok := h.ids(w, r)
	if !ok {
		return
	}
	d, err := h.svc.Get(r.Context(), h.actor(r), id, n)
	h.reply(w, r, d, err)
}

func (h *Handler) reply(w http.ResponseWriter, r *http.Request, d *Detail, err error) {
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, d)
}

func (h *Handler) start(w http.ResponseWriter, r *http.Request) {
	id, n, ok := h.ids(w, r)
	if !ok {
		return
	}
	d, err := h.svc.Start(r.Context(), h.actor(r), id, n)
	h.reply(w, r, d, err)
}

func (h *Handler) complete(w http.ResponseWriter, r *http.Request) {
	id, n, ok := h.ids(w, r)
	if !ok {
		return
	}
	d, err := h.svc.Complete(r.Context(), h.actor(r), id, n)
	h.reply(w, r, d, err)
}

type reasonBody struct {
	Reason string `json:"reason"`
}

func (h *Handler) restart(w http.ResponseWriter, r *http.Request) {
	id, n, ok := h.ids(w, r)
	if !ok {
		return
	}
	var body reasonBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		h.fail(w, r, err)
		return
	}
	d, err := h.svc.Restart(r.Context(), h.actor(r), id, n, body.Reason)
	h.reply(w, r, d, err)
}

func (h *Handler) reopen(w http.ResponseWriter, r *http.Request) {
	id, n, ok := h.ids(w, r)
	if !ok {
		return
	}
	var body reasonBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		h.fail(w, r, err)
		return
	}
	d, err := h.svc.Reopen(r.Context(), h.actor(r), id, n, body.Reason)
	h.reply(w, r, d, err)
}

func (h *Handler) saveData(w http.ResponseWriter, r *http.Request) {
	id, n, ok := h.ids(w, r)
	if !ok {
		return
	}
	var body struct {
		Version *int            `json:"version"`
		Reason  string          `json:"reason"`
		Data    json.RawMessage `json:"data"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		h.fail(w, r, err)
		return
	}
	if body.Data == nil {
		h.fail(w, r, apperr.BadRequest("INVALID_JSON", "Falta el objeto data."))
		return
	}
	d, err := h.svc.SaveData(r.Context(), h.actor(r), id, n, SaveInput{Version: body.Version, Reason: body.Reason, Data: body.Data})
	h.reply(w, r, d, err)
}

func (h *Handler) saveNotes(w http.ResponseWriter, r *http.Request) {
	id, n, ok := h.ids(w, r)
	if !ok {
		return
	}
	var body NotesInput
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		h.fail(w, r, err)
		return
	}
	d, err := h.svc.SaveNotes(r.Context(), h.actor(r), id, n, body.Notes)
	h.reply(w, r, d, err)
}
