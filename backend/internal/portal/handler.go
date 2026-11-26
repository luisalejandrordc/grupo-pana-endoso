package portal

import (
	"endosoflow/internal/auth"
	"endosoflow/internal/platform/httpx"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Handler serves /portal. Its routes accept only a signed-in customer.
type Handler struct {
	svc    *Service
	auth   *auth.Handler
	logger *slog.Logger
}

// NewHandler builds the handler. The auth handler supplies the customer middleware and the two routes about the
// customer's own account (profile and password), which belong to authentication.
func NewHandler(svc *Service, authHandler *auth.Handler, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, auth: authHandler, logger: logger}
}

// noStore keeps what a customer reads out of every cache, shared or not.
func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// Mount registers the routes under /portal.
func (h *Handler) Mount(r chi.Router) {
	r.Route("/portal", func(r chi.Router) {
		r.Use(noStore)
		r.Use(h.auth.AuthenticateCustomer)
		r.Get("/me", h.auth.CustomerMe)
		r.Put("/me/password", h.auth.CustomerChangePassword)
		r.Get("/sales", h.sales)
		r.Get("/sales/{id}", h.sale)
		r.Get("/documents/{id}/download-url", h.download)
	})
}

func (h *Handler) customer(r *http.Request) auth.CustomerActor {
	actor, _ := auth.CustomerFrom(r.Context())
	return actor
}

// pathID reads a UUID from the path. A malformed one is a not-found like any other: it names nothing.
func pathID(r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	return id, err == nil
}

func (h *Handler) sales(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.Sales(r.Context(), h.customer(r).ID)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, list)
}

func (h *Handler) sale(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		httpx.WriteError(w, r, h.logger, errSaleNotFound)
		return
	}
	detail, err := h.svc.Sale(r.Context(), h.customer(r).ID, id)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, detail)
}

func (h *Handler) download(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		httpx.WriteError(w, r, h.logger, errDocumentNotFound)
		return
	}
	url, err := h.svc.DownloadURL(r.Context(), h.customer(r).ID, id)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, url)
}
