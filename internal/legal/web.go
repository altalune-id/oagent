package legal

import (
	"cmp"
	"net/http"
	"net/url"
	"time"

	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/legal/views"
	"altalune.id/oagent/internal/web/layout"
)

// WebOpts configures the legal pages; /welcome exists only with RequireAcceptance, an acceptor and Sessions.
type WebOpts struct {
	Kit               layout.Kit
	Service           *Service
	Sessions          SessionRotator
	TermsURL          string
	PrivacyURL        string
	RequireAcceptance bool
}

// Web serves /terms, /privacy and /welcome.
type Web struct{ o WebOpts }

// NewWeb binds the legal pages to their options.
func NewWeb(o WebOpts) *Web {
	if o.Service == nil {
		o.Service = NewService(nil)
	}
	return &Web{o: o}
}

// Register mounts the legal pages on mux.
func (h *Web) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET "+h.o.Kit.Path("/terms"), h.document(TermsSlug, h.o.TermsURL))
	mux.HandleFunc("GET "+h.o.Kit.Path("/privacy"), h.document(PrivacySlug, h.o.PrivacyURL))
	if !h.o.RequireAcceptance || !h.o.Service.CanAccept() || h.o.Sessions == nil {
		return
	}
	mux.HandleFunc("GET "+h.o.Kit.Path("/welcome"), h.getWelcome)
	mux.HandleFunc("POST "+h.o.Kit.Path("/welcome"), h.postWelcome)
}

// WelcomeGate sends a signed-in person to /welcome, while on is true, until they accept the terms updated at termsUpdated.
func WelcomeGate(basePath string, on bool, termsUpdated time.Time) func(http.Handler) http.Handler {
	welcome := layout.Path(basePath, "/welcome")
	skipped := []string{
		welcome, layout.Path(basePath, "/login"), layout.Path(basePath, "/logout"), layout.Path(basePath, "/install"),
		layout.Path(basePath, "/invite"), layout.Path(basePath, "/forgot-password"), layout.Path(basePath, "/reset-password"),
		layout.Path(basePath, "/terms"), layout.Path(basePath, "/privacy"), layout.Path(basePath, "/static"),
		layout.Path(basePath, "/api"), layout.Path(basePath, "/mcp"), "/healthz", "/readyz",
	}
	return func(next http.Handler) http.Handler {
		if !on {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := session.PrincipalFrom(r.Context())
			if !p.IsPerson() || !needsAcceptance(p.TermsAcceptedAt, termsUpdated) || layout.UnderAny(r.URL.Path, skipped) {
				next.ServeHTTP(w, r)
				return
			}
			http.Redirect(w, r, welcome+"?return_to="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
		})
	}
}

func (h *Web) document(slug, override string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if override != "" {
			http.Redirect(w, r, override, http.StatusFound)
			return
		}
		doc, err := h.o.Service.Document(slug)
		if err != nil {
			h.o.Kit.Fail(w, r, "web: legal page", err)
			return
		}
		data := views.DocumentData{Title: doc.Title, HTML: doc.HTML, UpdatedAt: doc.UpdatedAt}
		h.o.Kit.Render(w, r, http.StatusOK, views.Document(h.o.Kit.Layout(doc.Title), data))
	}
}

func (h *Web) getWelcome(w http.ResponseWriter, r *http.Request) {
	principal := session.PrincipalFrom(r.Context())
	returnTo := r.URL.Query().Get("return_to")
	switch {
	case !principal.IsPerson():
		h.toLogin(w, r)
	case !h.o.Service.NeedsAcceptance(principal.TermsAcceptedAt):
		to := layout.ResolveReturnTo(h.o.Kit.BasePath, returnTo)
		http.Redirect(w, r, to, http.StatusSeeOther) //nolint:gosec // G710: ResolveReturnTo sanitizes.
	default:
		h.o.Kit.Render(w, r, http.StatusOK, views.Welcome(h.o.Kit.Layout("Welcome"), h.welcomeView(returnTo, "")))
	}
}

func (h *Web) postWelcome(w http.ResponseWriter, r *http.Request) {
	principal := session.PrincipalFrom(r.Context())
	returnTo := r.PostFormValue("return_to")
	if !principal.IsPerson() {
		h.toLogin(w, r)
		return
	}
	if r.PostFormValue("accept_terms") != "1" {
		view := h.welcomeView(returnTo, "Please accept the terms to continue.")
		h.o.Kit.Render(w, r, http.StatusUnprocessableEntity, views.Welcome(h.o.Kit.Layout("Welcome"), view))
		return
	}
	at, err := h.o.Service.Accept(r.Context(), principal.UserID, SourceWelcome)
	if err != nil {
		h.o.Kit.Fail(w, r, "web: accept terms", err)
		return
	}
	principal.TermsAcceptedAt = at
	if err := h.o.Sessions.Rotate(r.Context(), w, r, principal); err != nil {
		h.o.Kit.Fail(w, r, "web: rotate session", err)
		return
	}
	to := layout.ResolveReturnTo(h.o.Kit.BasePath, returnTo)
	http.Redirect(w, r, to, http.StatusSeeOther) //nolint:gosec // G710: ResolveReturnTo sanitizes.
}

func (h *Web) welcomeView(returnTo, errMsg string) views.WelcomeData {
	return views.WelcomeData{
		TermsURL:   cmp.Or(h.o.TermsURL, h.o.Kit.Path("/terms")),
		PrivacyURL: cmp.Or(h.o.PrivacyURL, h.o.Kit.Path("/privacy")),
		ReturnTo:   layout.SanitizeReturnTo(h.o.Kit.BasePath, returnTo),
		Error:      errMsg,
	}
}

func (h *Web) toLogin(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, h.o.Kit.Path("/login")+"?return_to="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
}
