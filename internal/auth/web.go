package auth

import (
	"context"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/auth/views"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/user"
	"altalune.id/oagent/internal/web/layout"
)

const loginFailed = "Invalid email or password"

// SignIn signs people in and out; *Service implements it.
type SignIn interface {
	LoginLocal(ctx context.Context, c Credentials) (session.Principal, error)
	Logout(ctx context.Context, sid string) error
}

// Web serves the login and logout pages.
type Web struct {
	kit      layout.Kit
	signIn   SignIn
	sessions CookieSessions
	ids      PublicIDs
	limits   Limits
}

// NewWeb binds the login pages to the sign-in service, the browser sessions and the public id lookups.
func NewWeb(kit layout.Kit, signIn SignIn, sessions CookieSessions, ids PublicIDs) *Web {
	return &Web{kit: kit, signIn: signIn, sessions: sessions, ids: ids}
}

// WithLimits rate limits sign-in per client IP and per email.
func (h *Web) WithLimits(l Limits) *Web {
	h.limits = l
	return h
}

// Register mounts GET/POST /login and POST /logout on mux.
func (h *Web) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET "+h.kit.Path("/login"), h.getLogin)
	mux.HandleFunc("POST "+h.kit.Path("/login"), h.postLogin)
	mux.HandleFunc("POST "+h.kit.Path("/logout"), h.postLogout)
}

type loginResponse struct {
	Session   string        `json:"session"`
	ExpiresAt time.Time     `json:"expires_at"`
	Principal principalView `json:"principal"`
}

type principalView struct {
	Source          string   `json:"source"`
	UserID          string   `json:"user_id,omitempty"`
	Email           string   `json:"email,omitempty"`
	Name            string   `json:"name,omitempty"`
	AvatarURL       string   `json:"avatar_url,omitempty"`
	ActiveOrgID     string   `json:"active_org_id,omitempty"`
	ActiveProjectID string   `json:"active_project_id,omitempty"`
	Permissions     []string `json:"permissions"`
	Operator        bool     `json:"operator"`
}

func (h *Web) viewOf(ctx context.Context, p session.Principal) (principalView, error) {
	perms := p.Scopes
	if perms == nil {
		perms = []string{}
	}
	v := principalView{
		Source:      string(p.Source),
		Email:       p.Email,
		Name:        p.Name,
		AvatarURL:   p.AvatarURL,
		Permissions: perms,
		Operator:    p.Operator,
	}
	var err error
	if p.UserID != uuid.Nil {
		if v.UserID, err = h.ids.UserPublicID(ctx, p.UserID); err != nil {
			return principalView{}, err
		}
	}
	if p.ActiveOrgID != uuid.Nil {
		if v.ActiveOrgID, err = h.ids.OrgPublicID(ctx, p.ActiveOrgID); err != nil {
			return principalView{}, err
		}
	}
	if p.ActiveProjectID != uuid.Nil {
		if v.ActiveProjectID, err = h.ids.ProjectPublicID(ctx, p.ActiveProjectID); err != nil {
			return principalView{}, err
		}
	}
	return v, nil
}

func (h *Web) getLogin(w http.ResponseWriter, r *http.Request) {
	returnTo := r.URL.Query().Get("return_to")
	if session.PrincipalFrom(r.Context()).IsPerson() {
		to := layout.ResolveReturnTo(h.kit.BasePath, returnTo)
		http.Redirect(w, r, to, http.StatusSeeOther) //nolint:gosec // G710: ResolveReturnTo sanitizes.
		return
	}
	view := views.LoginData{ReturnTo: layout.SanitizeReturnTo(h.kit.BasePath, returnTo)}
	if email, err := user.NormalizeEmail(r.URL.Query().Get("email")); err == nil {
		view.Email, view.EmailLocked = email, true
		view.AnotherLogin = h.kit.Path("/login")
		if view.ReturnTo != "" {
			view.AnotherLogin += "?return_to=" + url.QueryEscape(view.ReturnTo)
		}
	}
	h.kit.Render(w, r, http.StatusOK, views.Login(h.kit.Layout("Sign in"), view))
}

func (h *Web) postLogin(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimSpace(r.PostFormValue("email"))
	returnTo := r.PostFormValue("return_to")
	wantsJSON := acceptsJSON(r)

	if err := h.limitLogin(r, email); err != nil {
		h.loginLimited(w, r, err, wantsJSON, views.LoginData{Email: email, ReturnTo: layout.SanitizeReturnTo(h.kit.BasePath, returnTo)})
		return
	}
	principal, err := h.signIn.LoginLocal(r.Context(), Credentials{Email: email, Password: r.PostFormValue("password")})
	switch {
	case IsInvalidCredentialsError(err) && wantsJSON:
		writeJSON(w, http.StatusUnauthorized, map[string]string{"code": string(apperror.CodeAuthInvalidCredentials), "message": loginFailed})
		return
	case IsInvalidCredentialsError(err):
		view := views.LoginData{Email: email, ReturnTo: layout.SanitizeReturnTo(h.kit.BasePath, returnTo), Error: loginFailed}
		h.kit.Render(w, r, http.StatusUnauthorized, views.Login(h.kit.Layout("Sign in"), view))
		return
	case err != nil:
		h.kit.Fail(w, r, "web: login", err)
		return
	}
	if err := h.limits.ForgetEmail(r.Context(), email, LoginEmailRule()); err != nil && h.kit.Unexpected != nil {
		_ = h.kit.Unexpected(r.Context(), "web: rate limit reset", err)
	}
	var view principalView
	if wantsJSON {
		if view, err = h.viewOf(r.Context(), principal); err != nil {
			h.kit.Fail(w, r, "web: login view", err)
			return
		}
	}
	signed, expires, err := h.sessions.Start(r.Context(), w, principal)
	if err != nil {
		h.kit.Fail(w, r, "web: start session", err)
		return
	}
	if wantsJSON {
		writeJSON(w, http.StatusOK, loginResponse{Session: signed, ExpiresAt: expires, Principal: view})
		return
	}
	to := layout.ResolveReturnTo(h.kit.BasePath, returnTo)
	http.Redirect(w, r, to, http.StatusSeeOther) //nolint:gosec // G710: ResolveReturnTo sanitizes.
}

// SECURITY: both keys count before the password check; a success clears the email key, so only failures stay counted per email.
func (h *Web) limitLogin(r *http.Request, email string) error {
	if err := h.limits.LimitIP(r, LoginIPRule()); err != nil {
		return err
	}
	return h.limits.LimitEmail(r.Context(), email, LoginEmailRule())
}

func (h *Web) loginLimited(w http.ResponseWriter, r *http.Request, err error, wantsJSON bool, view views.LoginData) {
	rl, limited := errors.AsType[*RateLimitedError](err)
	w.Header().Set("Cache-Control", "no-store")
	switch {
	case limited && wantsJSON:
		w.Header().Set("Retry-After", strconv.Itoa(rl.RetryAfterSeconds()))
		writeJSON(w, http.StatusTooManyRequests, map[string]any{
			"code": string(apperror.CodeAuthRateLimited), "message": rl.Message(), "retry_after": rl.RetryAfterSeconds(),
		})
	case limited:
		w.Header().Set("Retry-After", strconv.Itoa(rl.RetryAfterSeconds()))
		view.Error = rl.Message()
		h.kit.Render(w, r, http.StatusTooManyRequests, views.Login(h.kit.Layout("Sign in"), view))
	case wantsJSON:
		limiterFailed(w, r, h.kit, err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"code": string(apperror.CodeUnexpected), "message": limiterDown})
	default:
		limiterFailed(w, r, h.kit, err)
		view.Error = limiterDown
		h.kit.Render(w, r, http.StatusServiceUnavailable, views.Login(h.kit.Layout("Sign in"), view))
	}
}

func (h *Web) postLogout(w http.ResponseWriter, r *http.Request) {
	if sid, ok := h.sessions.Read(r); ok {
		if err := h.signIn.Logout(r.Context(), sid); err != nil && h.kit.Unexpected != nil {
			_ = h.kit.Unexpected(r.Context(), "web: logout", err)
		}
	}
	_ = h.sessions.End(r.Context(), w, "")
	http.Redirect(w, r, h.kit.Path("/login"), http.StatusSeeOther)
}

func acceptsJSON(r *http.Request) bool {
	for part := range strings.SplitSeq(r.Header.Get("Accept"), ",") {
		if mt, _, err := mime.ParseMediaType(strings.TrimSpace(part)); err == nil && mt == "application/json" {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
