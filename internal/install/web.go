package install

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/auth"
	"altalune.id/oagent/internal/install/views"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/user"
	"altalune.id/oagent/internal/web/layout"
)

const (
	setupCookieName = "oagent_setup"
	setupCookieTTL  = 30 * time.Minute
)

// Installer runs the one-time install; *Service implements it.
type Installer interface {
	Install(ctx context.Context, r Request) (*user.User, error)
}

// WebOpts configures the install layout.
type WebOpts struct {
	Kit       layout.Kit
	Installer Installer
	SignIn    SignIn
	Sessions  SessionStarter
	Token     Token
	Required  *atomic.Bool
	Secret    []byte
	Secure    bool
	Consent   layout.ConsentData
}

// Web serves the install layout.
type Web struct{ o WebOpts }

// NewWeb binds the install page to its options.
func NewWeb(o WebOpts) *Web { return &Web{o: o} }

// Register mounts GET/POST /install on mux; without an Installer and Required it mounts nothing.
func (h *Web) Register(mux *http.ServeMux) {
	if h.o.Installer == nil || h.o.Required == nil {
		return
	}
	mux.HandleFunc("GET "+h.o.Kit.Path("/install"), h.getInstall)
	mux.HandleFunc("POST "+h.o.Kit.Path("/install"), h.postInstall)
}

// InstallGate sends every request to /install while required is true; /api and /mcp get a 503 JSON error instead.
func InstallGate(basePath string, required *atomic.Bool) func(http.Handler) http.Handler {
	installPath := layout.Path(basePath, "/install")
	open := []string{installPath, layout.Path(basePath, "/static")}
	machine := []string{layout.Path(basePath, "/api"), layout.Path(basePath, "/mcp")}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := r.URL.Path
			switch {
			case required == nil || !required.Load(), p == "/healthz", p == "/readyz", layout.UnderAny(p, open):
				next.ServeHTTP(w, r)
			case layout.UnderAny(p, machine):
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]string{"code": string(apperror.CodeInstallRequired), "message": "setup required"})
			default:
				http.Redirect(w, r, installPath, http.StatusSeeOther)
			}
		})
	}
}

func (h *Web) getInstall(w http.ResponseWriter, r *http.Request) {
	if !h.o.Required.Load() {
		http.Redirect(w, r, h.o.Kit.Path("/login"), http.StatusSeeOther)
		return
	}
	if token := r.URL.Query().Get("token"); token != "" {
		if !h.o.Token.Matches(token) {
			h.locked(w, r)
			return
		}
		h.writeSetupCookie(w, setupValue(h.o.Secret, h.o.Token, time.Now()), int(setupCookieTTL.Seconds()))
		http.Redirect(w, r, h.o.Kit.Path("/install"), http.StatusSeeOther)
		return
	}
	if !h.setupUnlocked(r) {
		h.locked(w, r)
		return
	}
	h.render(w, r, http.StatusOK, views.InstallData{})
}

func (h *Web) postInstall(w http.ResponseWriter, r *http.Request) {
	if !h.o.Required.Load() {
		http.Redirect(w, r, h.o.Kit.Path("/login"), http.StatusSeeOther)
		return
	}
	if !h.setupUnlocked(r) {
		h.locked(w, r)
		return
	}
	req := Request{
		Name:     strings.TrimSpace(r.PostFormValue("name")),
		Email:    strings.TrimSpace(r.PostFormValue("email")),
		Password: r.PostFormValue("password"),
		OrgName:  strings.TrimSpace(r.PostFormValue("org_name")),
	}
	if h.o.Consent.Required {
		if r.PostFormValue("accept_terms") != "1" {
			h.render(w, r, http.StatusUnprocessableEntity, h.retry(req, "Please accept the terms to continue."))
			return
		}
		req.AcceptTerms = true
	}
	_, err := h.o.Installer.Install(r.Context(), req)
	switch {
	case IsAlreadyInstalledError(err):
		h.o.Required.Store(false)
		http.Redirect(w, r, h.o.Kit.Path("/login"), http.StatusSeeOther)
		return
	case err != nil:
		h.installFailed(w, r, req, err)
		return
	}
	h.o.Required.Store(false)
	h.writeSetupCookie(w, "", -1)
	principal, err := h.o.SignIn.LoginLocal(r.Context(), auth.Credentials{Email: req.Email, Password: req.Password})
	if err != nil {
		h.o.Kit.Fail(w, r, "web: sign in after install", err)
		return
	}
	if _, _, err := h.o.Sessions.Start(r.Context(), w, principal); err != nil {
		h.o.Kit.Fail(w, r, "web: start session after install", err)
		return
	}
	http.Redirect(w, r, h.o.Kit.Path("/console")+"/", http.StatusSeeOther)
}

func (h *Web) installFailed(w http.ResponseWriter, r *http.Request, req Request, err error) {
	ae, ok := apperror.AsAppError(err)
	if !ok {
		h.o.Kit.Fail(w, r, "web: install", err)
		return
	}
	switch apperror.KindOf(ae.Code()) {
	case apperror.KindInvalid, apperror.KindAlreadyExists:
	default:
		h.o.Kit.Fail(w, r, "web: install", err)
		return
	}
	h.render(w, r, http.StatusUnprocessableEntity, h.retry(req, ae.Message()))
}

func (h *Web) retry(req Request, msg string) views.InstallData {
	return views.InstallData{Name: req.Name, Email: req.Email, OrgName: req.OrgName, Error: msg}
}

func (h *Web) render(w http.ResponseWriter, r *http.Request, status int, v views.InstallData) {
	v.Consent = h.o.Consent
	h.o.Kit.Render(w, r, status, views.Install(h.o.Kit.Layout("Set up oagent"), v))
}

func (h *Web) locked(w http.ResponseWriter, r *http.Request) {
	msg := "First-time setup needs the one-time install URL printed in the server log."
	h.o.Kit.Render(w, r, http.StatusForbidden, layout.Error(h.o.Kit.Layout("Setup is locked"), msg))
}

func (h *Web) setupUnlocked(r *http.Request) bool {
	c, err := r.Cookie(setupCookieName)
	return err == nil && validSetup(h.o.Secret, h.o.Token, c.Value, time.Now())
}

func (h *Web) writeSetupCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secure follows http.cookieSecure.
		Name:     setupCookieName,
		Value:    value,
		Path:     h.o.Kit.Path(""),
		MaxAge:   maxAge,
		Secure:   h.o.Secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// SECURITY: the cookie carries an expiry and a hash of this boot's token, never the token, so a new token voids old cookies.
func setupValue(secret []byte, token Token, now time.Time) string {
	exp := strconv.FormatInt(now.Add(setupCookieTTL).Unix(), 10)
	return session.Sign(secret, exp+"."+tokenTag(token))
}

func validSetup(secret []byte, token Token, raw string, now time.Time) bool {
	value, err := session.Verify(secret, raw)
	if err != nil {
		return false
	}
	exp, tag, ok := strings.Cut(value, ".")
	if !ok || token.Value() == "" {
		return false
	}
	unix, err := strconv.ParseInt(exp, 10, 64)
	if err != nil || !now.Before(time.Unix(unix, 0)) {
		return false
	}
	return tag == tokenTag(token)
}

func tokenTag(token Token) string {
	sum := sha256.Sum256([]byte("oagent.setup:" + token.Value()))
	return base64.RawURLEncoding.EncodeToString(sum[:16])
}
