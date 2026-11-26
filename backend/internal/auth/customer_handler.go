package auth

import (
	"context"
	"endosoflow/internal/platform/apperr"
	"endosoflow/internal/platform/httpx"
	"net/http"
	"strings"
	"time"
)

// The customer portal's HTTP side of authentication: sign-in, session cookie, and the middleware that lets only
// customers through to /portal.

const (
	// The customer cookie has its own name and path. In local development both portals are served from localhost,
	// where a cookie does not distinguish ports, so the names must differ.
	customerRefreshCookieName = "ef_customer_refresh"
	customerRefreshCookiePath = "/api/v1/auth/customer"
)

type customerActorKey struct{}

// CustomerFrom returns the authenticated customer of the request, if any.
func CustomerFrom(ctx context.Context) (CustomerActor, bool) {
	actor, ok := ctx.Value(customerActorKey{}).(CustomerActor)
	return actor, ok
}

// AuthenticateCustomer requires a valid customer access token and puts the customer in the request context. An
// employee's token is refused: the two kinds of token are not interchangeable.
func (h *Handler) AuthenticateCustomer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || token == "" {
			httpx.WriteError(w, r, h.logger, errAccessTokenInvalid)
			return
		}
		actor, err := h.svc.AuthenticateCustomer(r.Context(), token)
		if err != nil {
			httpx.WriteError(w, r, h.logger, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), customerActorKey{}, actor)))
	})
}

func (h *Handler) setCustomerRefreshCookie(w http.ResponseWriter, session *CustomerSession) {
	cookie := &http.Cookie{
		Name: customerRefreshCookieName, Value: session.RefreshToken, Path: customerRefreshCookiePath,
		HttpOnly: true, Secure: h.cookieSecure, SameSite: http.SameSiteStrictMode,
	}
	if session.RememberMe {
		cookie.MaxAge = int(session.RefreshExpiresAt.Sub(h.svc.now()).Seconds())
	}
	http.SetCookie(w, cookie)
}

// ClearCustomerRefreshCookie removes the customer's session cookie from the browser.
func (h *Handler) ClearCustomerRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: customerRefreshCookieName, Value: "", Path: customerRefreshCookiePath, MaxAge: -1,
		HttpOnly: true, Secure: h.cookieSecure, SameSite: http.SameSiteStrictMode,
	})
}

func (h *Handler) customerRefreshCookieValue(r *http.Request) string {
	cookie, err := r.Cookie(customerRefreshCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

type customerSessionResponse struct {
	AccessToken string          `json:"accessToken"`
	ExpiresAt   time.Time       `json:"expiresAt"`
	Customer    CustomerProfile `json:"customer"`
}

func writeCustomerSession(w http.ResponseWriter, session *CustomerSession) {
	httpx.WriteJSON(w, http.StatusOK, customerSessionResponse{
		AccessToken: session.AccessToken, ExpiresAt: session.AccessExpiresAt, Customer: session.Customer,
	})
}

func (h *Handler) customerLogin(w http.ResponseWriter, r *http.Request) {
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
	session, err := h.svc.CustomerLogin(r.Context(), LoginRequest{
		DocumentType: body.DocumentType, DocumentNumber: body.DocumentNumber,
		Password: body.Password, RememberMe: body.RememberMe,
	}, h.client(r))
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	h.setCustomerRefreshCookie(w, session)
	writeCustomerSession(w, session)
}

func (h *Handler) customerRefresh(w http.ResponseWriter, r *http.Request) {
	session, err := h.svc.CustomerRefresh(r.Context(), h.customerRefreshCookieValue(r))
	if err != nil {
		if apperr.FromError(err).Code == errSessionInvalid.Code {
			h.ClearCustomerRefreshCookie(w)
		}
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	if session.RefreshToken != "" { // empty during the grace window: keep the cookie the other tab just received
		h.setCustomerRefreshCookie(w, session)
	}
	writeCustomerSession(w, session)
}

func (h *Handler) customerLogout(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.CustomerLogout(r.Context(), h.customerRefreshCookieValue(r), h.client(r)); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	h.ClearCustomerRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) customerForgotPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DocumentType   string `json:"documentType"`
		DocumentNumber string `json:"documentNumber"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	if err := h.svc.CustomerForgotPassword(r.Context(), body.DocumentType, body.DocumentNumber, h.client(r)); err != nil {
		// Even a server problem is hidden from the caller, so the response never depends on the account.
		h.logger.Error("customer forgot password failed", "error", err)
	}
	httpx.WriteJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

// CustomerMe answers the profile of the signed-in customer (GET /portal/me). Mount it behind AuthenticateCustomer.
func (h *Handler) CustomerMe(w http.ResponseWriter, r *http.Request) {
	actor, _ := CustomerFrom(r.Context())
	profile, err := h.svc.CustomerMe(r.Context(), actor.ID)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, profile)
}

// CustomerChangePassword changes the signed-in customer's password (PUT /portal/me/password). Mount it behind
// AuthenticateCustomer.
func (h *Handler) CustomerChangePassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	actor, _ := CustomerFrom(r.Context())
	if err := h.svc.ChangeCustomerPassword(r.Context(), actor.ID, body.CurrentPassword, body.NewPassword, h.client(r)); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	h.ClearCustomerRefreshCookie(w) // every session ended, including this one
	w.WriteHeader(http.StatusNoContent)
}
