package auth

import (
	"context"
	"endosoflow/internal/platform/apperr"
	"endosoflow/internal/platform/httpx"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

const (
	refreshCookieName = "ef_employee_refresh"
	refreshCookiePath = "/api/v1/auth"

	// Refresh and logout use a cookie, so they also require this custom header (a form on another site
	// cannot send it) and, when the browser sends one, a matching Origin.
	csrfHeader = "X-Requested-With"
	csrfValue  = "EndosoFlow"
)

// HandlerConfig holds the settings the HTTP layer needs.
type HandlerConfig struct {
	AppBaseURL    string
	PortalBaseURL string
	CookieSecure  bool
	TrustProxy    bool
}

// Handler serves the authentication routes and provides the Authenticate middleware.
type Handler struct {
	svc             *Service
	logger          *slog.Logger
	cookieSecure    bool
	trustProxy      bool
	allowedOrigin   string
	customerOrigin  string
	loginLimiter    *httpx.RateLimiter
	recoveryLimiter *httpx.RateLimiter
}

// NewHandler builds the handler.
func NewHandler(svc *Service, logger *slog.Logger, cfg HandlerConfig) *Handler {
	return &Handler{
		svc: svc, logger: logger, cookieSecure: cfg.CookieSecure, trustProxy: cfg.TrustProxy,
		allowedOrigin: originOf(cfg.AppBaseURL), customerOrigin: originOf(cfg.PortalBaseURL),
		loginLimiter:    httpx.NewRateLimiter(10, time.Minute), // 10 login attempts per minute per address
		recoveryLimiter: httpx.NewRateLimiter(5, time.Minute),  // 5 activation or recovery calls per minute
	}
}

// originOf reduces a site address to its origin (scheme and host), which is what a browser sends in Origin.
func originOf(address string) string {
	if u, err := url.Parse(address); err == nil && u.Scheme != "" && u.Host != "" {
		return u.Scheme + "://" + u.Host
	}
	return address
}

// Mount registers the routes under the given router (already under /api/v1).
func (h *Handler) Mount(r chi.Router) {
	login := h.loginLimiter.Middleware(h.logger, h.trustProxy)
	recovery := h.recoveryLimiter.Middleware(h.logger, h.trustProxy)

	r.Route("/auth", func(r chi.Router) {
		r.With(login).Post("/employee/login", h.login)
		r.With(h.requireSameSite).Post("/refresh", h.refresh)
		r.With(h.requireSameSite).Post("/logout", h.logout)
		r.With(recovery).Post("/activate", h.activate)
		r.With(recovery).Post("/forgot-password", h.forgotPassword)
		r.With(recovery).Post("/reset-password", h.resetPassword)

		// The customer portal's own sign-in, with its own cookie and the customer portal's origin.
		r.Route("/customer", func(r chi.Router) {
			r.With(login).Post("/login", h.customerLogin)
			r.With(h.sameSite(h.customerOrigin)).Post("/refresh", h.customerRefresh)
			r.With(h.sameSite(h.customerOrigin)).Post("/logout", h.customerLogout)
			r.With(recovery).Post("/forgot-password", h.customerForgotPassword)
		})
	})
	r.Group(func(r chi.Router) {
		r.Use(h.Authenticate)
		r.Get("/me", h.me)
		r.Put("/me/password", h.changePassword)
	})
}

// ---- context and middleware --------------------------------------------------------------------

type actorKey struct{}

// ActorFrom returns the authenticated employee of the request, if any.
func ActorFrom(ctx context.Context) (Actor, bool) {
	actor, ok := ctx.Value(actorKey{}).(Actor)
	return actor, ok
}

// Authenticate requires a valid access token and puts the actor in the request context.
func (h *Handler) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || token == "" {
			httpx.WriteError(w, r, h.logger, errAccessTokenInvalid)
			return
		}
		actor, err := h.svc.Authenticate(r.Context(), token)
		if err != nil {
			httpx.WriteError(w, r, h.logger, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), actorKey{}, actor)))
	})
}

// RequireRoles allows the request only when the authenticated employee has one of the roles.
// Use it after Authenticate; without an actor it answers 401.
func RequireRoles(logger *slog.Logger, roles ...Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor, ok := ActorFrom(r.Context())
			if !ok {
				httpx.WriteError(w, r, logger, errAccessTokenInvalid)
				return
			}
			for _, role := range roles {
				if actor.Role == role {
					next.ServeHTTP(w, r)
					return
				}
			}
			httpx.WriteError(w, r, logger, apperr.Forbidden("FORBIDDEN", "No tiene permiso para realizar esta acción."))
		})
	}
}

func (h *Handler) requireSameSite(next http.Handler) http.Handler {
	return h.sameSite(h.allowedOrigin)(next)
}

// sameSite protects the cookie routes of one portal: the custom header (which a form on another site cannot send)
// and, when the browser sends one, an Origin equal to that portal's.
func (h *Handler) sameSite(allowedOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get(csrfHeader) != csrfValue {
				httpx.WriteError(w, r, h.logger, apperr.Forbidden("CSRF_REJECTED", "Solicitud no permitida."))
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" && origin != allowedOrigin {
				httpx.WriteError(w, r, h.logger, apperr.Forbidden("CSRF_REJECTED", "Solicitud no permitida."))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ---- cookies -----------------------------------------------------------------------------------

func (h *Handler) setRefreshCookie(w http.ResponseWriter, session *Session) {
	cookie := &http.Cookie{
		Name: refreshCookieName, Value: session.RefreshToken, Path: refreshCookiePath,
		HttpOnly: true, Secure: h.cookieSecure, SameSite: http.SameSiteStrictMode,
	}
	if session.RememberMe {
		// Persistent cookie. Without remember-me it stays a session cookie that ends with the browser.
		cookie.MaxAge = int(session.RefreshExpiresAt.Sub(h.svc.now()).Seconds())
	}
	http.SetCookie(w, cookie)
}

func (h *Handler) clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: refreshCookieName, Value: "", Path: refreshCookiePath, MaxAge: -1,
		HttpOnly: true, Secure: h.cookieSecure, SameSite: http.SameSiteStrictMode,
	})
}

func (h *Handler) refreshCookieValue(r *http.Request) string {
	cookie, err := r.Cookie(refreshCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func (h *Handler) client(r *http.Request) ClientInfo {
	return ClientInfo{DeviceInfo: r.UserAgent(), IP: httpx.ClientIP(r, h.trustProxy)}
}

// ---- handlers ----------------------------------------------------------------------------------

type sessionResponse struct {
	AccessToken string    `json:"accessToken"`
	ExpiresAt   time.Time `json:"expiresAt"`
	Employee    Profile   `json:"employee"`
}

func writeSession(w http.ResponseWriter, session *Session) {
	httpx.WriteJSON(w, http.StatusOK, sessionResponse{
		AccessToken: session.AccessToken, ExpiresAt: session.AccessExpiresAt, Employee: session.Employee,
	})
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DocumentType   string `json:"documentType"`
		DocumentNumber string `json:"documentNumber"`
		Password       string `json:"password"`
		RememberMe     bool   `json:"rememberMe"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	session, err := h.svc.Login(r.Context(), LoginRequest{
		DocumentType: body.DocumentType, DocumentNumber: body.DocumentNumber,
		Password: body.Password, RememberMe: body.RememberMe,
	}, h.client(r))
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	h.setRefreshCookie(w, session)
	writeSession(w, session)
}

func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	session, err := h.svc.Refresh(r.Context(), h.refreshCookieValue(r))
	if err != nil {
		if apperr.FromError(err).Code == errSessionInvalid.Code {
			h.clearRefreshCookie(w)
		}
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	if session.RefreshToken != "" { // empty during the grace window: keep the cookie the other tab just received
		h.setRefreshCookie(w, session)
	}
	writeSession(w, session)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Logout(r.Context(), h.refreshCookieValue(r), h.client(r)); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	h.clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) activate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	account, err := h.svc.Activate(r.Context(), body.Token, body.Password, h.client(r))
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]AccountType{"accountType": account})
}

func (h *Handler) forgotPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DocumentType   string `json:"documentType"`
		DocumentNumber string `json:"documentNumber"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	if err := h.svc.ForgotPassword(r.Context(), body.DocumentType, body.DocumentNumber, h.client(r)); err != nil {
		// Even a server problem is hidden from the caller, so the response never depends on the account.
		h.logger.Error("forgot password failed", "error", err)
	}
	httpx.WriteJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

func (h *Handler) resetPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	if err := h.svc.ResetPassword(r.Context(), body.Token, body.Password, h.client(r)); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	actor, _ := ActorFrom(r.Context())
	profile, err := h.svc.Me(r.Context(), actor.ID)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, profile)
}

func (h *Handler) changePassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	actor, _ := ActorFrom(r.Context())
	if err := h.svc.ChangePassword(r.Context(), actor.ID, body.CurrentPassword, body.NewPassword, h.client(r)); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	h.clearRefreshCookie(w) // every session ended, including this one
	w.WriteHeader(http.StatusNoContent)
}
