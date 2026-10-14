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

const (
	resetCookieName = "oagent_reset"
	resetDone       = "Your password was changed. Sign in with the new one."
	resetIncomplete = "This reset link is incomplete. Open the link from your email again."
)

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
	resets   *Resets
	reset    session.SignedCookie
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

// WithResets adds the password reset pages; their cookie shares the session cookie's secret and Secure flag.
func (h *Web) WithResets(r *Resets) *Web {
	h.resets = r
	h.reset = session.SignedCookie{
		Name: resetCookieName, Path: h.kit.Path("/reset-password"), Secret: h.sessions.Secret, Secure: h.sessions.Cookie.Secure,
		Now: func() time.Time { return r.now() },
	}
	return h
}

// Register mounts GET/POST /login, POST /logout and, with resets, the password reset pages on mux.
func (h *Web) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET "+h.kit.Path("/login"), h.getLogin)
	mux.HandleFunc("POST "+h.kit.Path("/login"), h.postLogin)
	mux.HandleFunc("POST "+h.kit.Path("/logout"), h.postLogout)
	if h.resets == nil {
		return
	}
	mux.HandleFunc("GET "+h.kit.Path("/forgot-password"), h.getForgot)
	mux.HandleFunc("POST "+h.kit.Path("/forgot-password"), h.postForgot)
	mux.HandleFunc("GET "+h.kit.Path("/reset-password/{token}"), h.resetLimited(h.getResetLink))
	mux.HandleFunc("GET "+h.kit.Path("/reset-password"), h.resetLimited(h.getReset))
	mux.HandleFunc("POST "+h.kit.Path("/reset-password"), h.resetLimited(h.postReset))
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
	view := views.LoginData{ReturnTo: layout.SanitizeReturnTo(h.kit.BasePath, returnTo), ForgotURL: h.forgotURL()}
	if r.URL.Query().Get("reset") == "done" {
		view.Notice = resetDone
	}
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
		view := views.LoginData{
			Email: email, ReturnTo: layout.SanitizeReturnTo(h.kit.BasePath, returnTo), Error: loginFailed, ForgotURL: h.forgotURL(),
		}
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

func (h *Web) forgotURL() string {
	if h.resets == nil {
		return ""
	}
	return h.kit.Path("/forgot-password")
}

func (h *Web) getForgot(w http.ResponseWriter, r *http.Request) {
	h.kit.Render(w, r, http.StatusOK, views.Forgot(h.kit.Layout("Reset your password"), views.ForgotData{}))
}

// SECURITY: the same page for every email, known or not; only an existing person with a password gets mail.
func (h *Web) postForgot(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimSpace(r.PostFormValue("email"))
	if err := h.limits.LimitIP(r, ResetIPRule()); err != nil {
		WriteLimited(w, r, h.kit, "Reset your password", err)
		return
	}
	if err := h.limits.LimitEmail(r.Context(), email, ResetEmailRule()); err != nil {
		WriteLimited(w, r, h.kit, "Reset your password", err)
		return
	}
	if err := h.resets.Request(r.Context(), email); err != nil {
		h.kit.Fail(w, r, "web: reset request", err)
		return
	}
	h.kit.Render(w, r, http.StatusOK, views.Forgot(h.kit.Layout("Reset your password"), views.ForgotData{Sent: true}))
}

func (h *Web) resetLimited(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Referrer-Policy", "no-referrer")
		if err := h.limits.LimitIP(r, ResetLinkIPRule()); err != nil {
			WriteLimited(w, r, h.kit, "Reset your password", err)
			return
		}
		next(w, r)
	}
}

// SECURITY: the raw token must never reach the cookie, a log or a Referer.
func (h *Web) getResetLink(w http.ResponseWriter, r *http.Request) {
	reset, err := h.resets.Lookup(r.Context(), r.PathValue("token"))
	if err != nil {
		h.resetFailed(w, r, err)
		return
	}
	h.reset.Set(w, reset.PublicID+"|"+strconv.FormatInt(reset.ExpiresAt.Unix(), 10), reset.ExpiresAt)
	http.Redirect(w, r, h.kit.Path("/reset-password"), http.StatusSeeOther)
}

func (h *Web) getReset(w http.ResponseWriter, r *http.Request) {
	reset, ok := h.resetFromCookie(w, r)
	if !ok {
		return
	}
	h.renderReset(w, r, http.StatusOK, reset, "")
}

func (h *Web) postReset(w http.ResponseWriter, r *http.Request) {
	reset, ok := h.resetFromCookie(w, r)
	if !ok {
		return
	}
	err := h.resets.Complete(r.Context(), reset.PublicID, r.PostFormValue("password"))
	if ae, invalid := apperror.AsAppError(err); invalid && apperror.KindOf(ae.Code()) == apperror.KindInvalid {
		h.renderReset(w, r, http.StatusUnprocessableEntity, reset, ae.Message())
		return
	}
	if err != nil {
		h.resetFailed(w, r, err)
		return
	}
	h.reset.Clear(w)
	session.ClearCookie(w, h.sessions.Cookie)
	http.Redirect(w, r, h.kit.Path("/login")+"?reset=done", http.StatusSeeOther)
}

func (h *Web) resetFromCookie(w http.ResponseWriter, r *http.Request) (*Reset, bool) {
	value, ok := h.reset.Get(r)
	publicID, _, cut := strings.Cut(value, "|")
	if !ok || !cut {
		h.kit.Render(w, r, http.StatusBadRequest, layout.Error(h.kit.Layout("Reset your password"), resetIncomplete))
		return nil, false
	}
	reset, err := h.resets.ByPublicID(r.Context(), publicID)
	if err != nil {
		h.resetFailed(w, r, err)
		return nil, false
	}
	return reset, true
}

func (h *Web) renderReset(w http.ResponseWriter, r *http.Request, status int, reset *Reset, msg string) {
	email, name, err := h.resets.Person(r.Context(), reset)
	if err != nil {
		h.kit.Fail(w, r, "web: reset person", err)
		return
	}
	h.kit.Render(w, r, status, views.Reset(h.kit.Layout("Choose a new password"), views.ResetData{Email: email, Name: name, Error: msg}))
}

func (h *Web) resetFailed(w http.ResponseWriter, r *http.Request, err error) {
	e, ok := errors.AsType[*ResetNotUsableError](err)
	if !ok {
		h.kit.Fail(w, r, "web: reset", err)
		return
	}
	h.reset.Clear(w)
	h.kit.Render(w, r, http.StatusGone, views.Forgot(h.kit.Layout("Reset your password"), views.ForgotData{Error: e.Message()}))
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
