package auth

import (
	"context"
	"encoding/json"
	"mime"
	"net/http"
	"strings"
	"time"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/auth/views"
	"altalune.id/oagent/internal/kernel/session"
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
}

// NewWeb binds the login pages to the sign-in service and the browser sessions.
func NewWeb(kit layout.Kit, signIn SignIn, sessions CookieSessions) *Web {
	return &Web{kit: kit, signIn: signIn, sessions: sessions}
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

func viewOf(p session.Principal) principalView {
	perms := p.Scopes
	if perms == nil {
		perms = []string{}
	}
	return principalView{
		Source:          string(p.Source),
		UserID:          idString(p.UserID),
		Email:           p.Email,
		Name:            p.Name,
		AvatarURL:       p.AvatarURL,
		ActiveOrgID:     idString(p.ActiveOrgID),
		ActiveProjectID: idString(p.ActiveProjectID),
		Permissions:     perms,
		Operator:        p.Operator,
	}
}

func (h *Web) getLogin(w http.ResponseWriter, r *http.Request) {
	returnTo := r.URL.Query().Get("return_to")
	if session.PrincipalFrom(r.Context()).IsPerson() {
		to := layout.ResolveReturnTo(h.kit.BasePath, returnTo)
		http.Redirect(w, r, to, http.StatusSeeOther) //nolint:gosec // G710: ResolveReturnTo sanitizes.
		return
	}
	view := views.LoginData{ReturnTo: layout.SanitizeReturnTo(h.kit.BasePath, returnTo)}
	h.kit.Render(w, r, http.StatusOK, views.Login(h.kit.Layout("Sign in"), view))
}

func (h *Web) postLogin(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimSpace(r.PostFormValue("email"))
	returnTo := r.PostFormValue("return_to")
	wantsJSON := acceptsJSON(r)

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
	signed, expires, err := h.sessions.Start(r.Context(), w, principal)
	if err != nil {
		h.kit.Fail(w, r, "web: start session", err)
		return
	}
	if wantsJSON {
		writeJSON(w, http.StatusOK, loginResponse{Session: signed, ExpiresAt: expires, Principal: viewOf(principal)})
		return
	}
	to := layout.ResolveReturnTo(h.kit.BasePath, returnTo)
	http.Redirect(w, r, to, http.StatusSeeOther) //nolint:gosec // G710: ResolveReturnTo sanitizes.
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
