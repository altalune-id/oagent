package invite

import (
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/auth"
	"altalune.id/oagent/internal/kernel/ratelimit"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/org/invite/views"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/user"
	"altalune.id/oagent/internal/web/layout"
)

const cookieName = "oagent_invite"

// MethodPassword is the sign-in method for a local account with a password.
const MethodPassword = "password"

// WebOpts configures the invite pages.
type WebOpts struct {
	Kit          layout.Kit
	Invites      *Service
	Users        Users
	Members      Memberships
	Orgs         Orgs
	OrgLinks     OrgLinks
	People       People
	SignIn       SignIn
	Sessions     Sessions
	Consent      layout.ConsentData
	Methods      []string
	StateSecret  []byte
	CookieSecure bool
	Now          func() time.Time
	Limits       auth.Limits
}

// LandingRule limits opening invite links and the invite page per client IP: 60 an hour.
func LandingRule() ratelimit.Rule {
	return ratelimit.Rule{Name: "invite-landing-ip", Rate: 60, Per: time.Hour, Burst: 60}
}

// SubmitRule limits joining and accepting per client IP: 20 an hour.
func SubmitRule() ratelimit.Rule {
	return ratelimit.Rule{Name: "invite-submit-ip", Rate: 20, Per: time.Hour, Burst: 20}
}

// Web serves the invite landing, join, accept and switch pages.
type Web struct {
	o      WebOpts
	cookie session.SignedCookie
}

// NewWeb binds the invite pages to their options.
func NewWeb(o WebOpts) *Web {
	if o.Now == nil {
		o.Now = time.Now
	}
	return &Web{o: o, cookie: session.SignedCookie{
		Name: cookieName, Path: o.Kit.Path("/invite"), Secret: o.StateSecret, Secure: o.CookieSecure, Now: o.Now,
	}}
}

// Register mounts the invite pages on mux.
func (h *Web) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET "+h.o.Kit.Path("/invite/{token}"), h.limited(LandingRule(), h.getLink))
	mux.HandleFunc("GET "+h.o.Kit.Path("/invite"), h.limited(LandingRule(), h.getInvite))
	mux.HandleFunc("POST "+h.o.Kit.Path("/invite/join"), h.limited(SubmitRule(), h.postJoin))
	mux.HandleFunc("POST "+h.o.Kit.Path("/invite/accept"), h.limited(SubmitRule(), h.postAccept))
	mux.HandleFunc("POST "+h.o.Kit.Path("/invite/switch"), h.postSwitch)
}

func (h *Web) limited(rule ratelimit.Rule, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h.o.Limits.LimitIP(r, rule); err != nil {
			w.Header().Set("Referrer-Policy", "no-referrer")
			auth.WriteLimited(w, r, h.o.Kit, "Invite", err)
			return
		}
		next(w, r)
	}
}

// SECURITY: the raw token must never reach the cookie, a log or a Referer.
func (h *Web) getLink(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Referrer-Policy", "no-referrer")
	inv, err := h.o.Invites.Lookup(r.Context(), r.PathValue("token"))
	if err != nil {
		h.failed(w, r, nil, err)
		return
	}
	h.cookie.Set(w, inv.PublicID+"|"+strconv.FormatInt(inv.ExpiresAt.Unix(), 10), inv.ExpiresAt)
	http.Redirect(w, r, h.o.Kit.Path("/invite"), http.StatusSeeOther)
}

func (h *Web) getInvite(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Referrer-Policy", "no-referrer")
	inv, ok := h.fromCookie(w, r)
	if !ok {
		return
	}
	v, err := h.landing(r, inv)
	if err != nil {
		h.o.Kit.Fail(w, r, "web: invite", err)
		return
	}
	h.render(w, r, http.StatusOK, v)
}

func (h *Web) postJoin(w http.ResponseWriter, r *http.Request) {
	inv, ok := h.fromCookie(w, r)
	if !ok {
		return
	}
	v, err := h.landing(r, inv)
	if err != nil {
		h.o.Kit.Fail(w, r, "web: invite join", err)
		return
	}
	if v.State != views.StateJoin {
		h.render(w, r, http.StatusConflict, v)
		return
	}
	v.Name = strings.TrimSpace(r.PostFormValue("name"))
	accept := r.PostFormValue("accept_terms") == "1"
	if h.o.Consent.Required && !accept {
		v.Error = "Please accept the terms to continue."
		h.render(w, r, http.StatusUnprocessableEntity, v)
		return
	}
	pass := r.PostFormValue("password")
	_, err = h.o.Invites.Join(r.Context(), inv.ID, v.Name, pass, h.o.Consent.Required && accept)
	switch {
	case user.IsAlreadyExistsError(err):
		v.State, v.LoginURL = views.StateSignIn, h.loginURL(inv.Email)
		h.render(w, r, http.StatusConflict, v)
		return
	case err != nil:
		h.joinFailed(w, r, inv, v, err)
		return
	}
	p, err := h.o.SignIn.LoginLocal(r.Context(), auth.Credentials{Email: inv.Email, Password: pass})
	if err != nil {
		h.o.Kit.Fail(w, r, "web: sign in after join", err)
		return
	}
	if _, _, err := h.o.Sessions.Start(r.Context(), w, p); err != nil {
		h.o.Kit.Fail(w, r, "web: start session after join", err)
		return
	}
	h.done(w, r, inv)
}

// TODO(P3): require email_verified and a matching email before accept.
func (h *Web) postAccept(w http.ResponseWriter, r *http.Request) {
	inv, ok := h.fromCookie(w, r)
	if !ok {
		return
	}
	p := session.PrincipalFrom(r.Context())
	if !p.IsPerson() {
		http.Redirect(w, r, h.loginURL(inv.Email), http.StatusSeeOther)
		return
	}
	_, err := h.o.Invites.Accept(r.Context(), inv.ID, p.UserID)
	switch {
	case IsEmailMismatchError(err):
		v, lerr := h.landing(r, inv)
		if lerr != nil {
			h.o.Kit.Fail(w, r, "web: invite accept", lerr)
			return
		}
		h.render(w, r, http.StatusForbidden, v)
		return
	case err != nil:
		h.failed(w, r, inv, err)
		return
	}
	h.done(w, r, inv)
}

func (h *Web) postSwitch(w http.ResponseWriter, r *http.Request) {
	sid, _ := h.o.Sessions.Read(r)
	if sid != "" {
		if err := h.o.SignIn.Logout(r.Context(), sid); err != nil && h.o.Kit.Unexpected != nil {
			_ = h.o.Kit.Unexpected(r.Context(), "web: invite switch", err)
		}
	}
	_ = h.o.Sessions.End(r.Context(), w, "")
	http.Redirect(w, r, h.o.Kit.Path("/invite"), http.StatusSeeOther)
}

// NOTE: landing only reads; every change happens in a POST.
func (h *Web) landing(r *http.Request, inv *Invite) (views.LandingData, error) {
	ctx := r.Context()
	org, err := h.o.Orgs.Name(ctx, inv.OrgID)
	if err != nil {
		return views.LandingData{}, err
	}
	v := views.LandingData{
		Org: strings.Join(strings.Fields(org), " "), Inviter: h.o.Invites.inviterName(ctx, inv.InvitedBy),
		Role: inv.Role.Label(), Email: inv.Email, Consent: h.o.Consent,
	}
	p := session.PrincipalFrom(ctx)
	switch {
	case p.IsPerson() && !strings.EqualFold(p.Email, inv.Email):
		v.State, v.SignedInEmail = views.StateMismatch, p.Email
	case p.IsPerson():
		v.State = views.StateAccept
		_, err := h.o.Members.Find(ctx, inv.OrgID, p.UserID)
		if err == nil {
			v.State = views.StateMember
		} else if !member.IsNotFoundError(err) {
			return views.LandingData{}, err
		}
	default:
		_, err := h.o.Users.ByEmail(ctx, inv.Email)
		switch {
		case err == nil:
			v.State, v.LoginURL = views.StateSignIn, h.loginURL(inv.Email)
		case !user.IsNotFoundError(err):
			return views.LandingData{}, err
		case slices.Contains(h.o.Methods, MethodPassword):
			v.State = views.StateJoin
		default:
			v.State = views.StateNoMethod
		}
	}
	return v, nil
}

func (h *Web) fromCookie(w http.ResponseWriter, r *http.Request) (*Invite, bool) {
	publicID, issued, ok := h.readCookie(r)
	if !ok {
		h.incomplete(w, r)
		return nil, false
	}
	inv, err := h.o.Invites.ByPublicID(r.Context(), publicID)
	if IsNotFoundError(err) {
		err = &NotUsableError{Reason: ReasonNotFound}
	}
	// SECURITY: a cookie from a link issued before a resend is stale.
	if err == nil && inv.ExpiresAt.Unix() != issued && inv.live() {
		h.cookie.Clear(w)
		h.incomplete(w, r)
		return nil, false
	}
	if err == nil {
		err = inv.Usable(h.o.Now())
	}
	if err != nil {
		h.failed(w, r, inv, err)
		return nil, false
	}
	return inv, true
}

func (h *Web) incomplete(w http.ResponseWriter, r *http.Request) {
	msg := "This invite link is incomplete. Open the link from your email again."
	h.o.Kit.Render(w, r, http.StatusBadRequest, layout.Error(h.o.Kit.Layout("Invite"), msg))
}

func (h *Web) failed(w http.ResponseWriter, r *http.Request, inv *Invite, err error) {
	reason := ReasonOf(err)
	if reason == "" {
		h.o.Kit.Fail(w, r, "web: invite", err)
		return
	}
	h.cookie.Clear(w)
	v := views.GoneData{Message: goneMessage(reason)}
	p := session.PrincipalFrom(r.Context())
	v.SignIn = reason == ReasonUsed && !p.IsPerson()
	if inv != nil && reason == ReasonUsed && p.IsPerson() && inv.AcceptedBy == p.UserID {
		if slug, serr := h.o.OrgLinks.Slug(r.Context(), inv.OrgID); serr == nil {
			v.OpenURL = h.consoleURL(slug)
			v.OpenOrg, _ = h.o.Orgs.Name(r.Context(), inv.OrgID)
		}
	}
	h.o.Kit.Render(w, r, http.StatusGone, views.Gone(h.o.Kit.Layout("Invite"), v))
}

func goneMessage(r Reason) string {
	switch r {
	case ReasonExpired:
		return "This invite has expired. Ask the person who invited you to send a new one."
	case ReasonRevoked:
		return "This invite was cancelled. Ask the person who invited you if you still need access."
	case ReasonUsed:
		return "This invite was already accepted."
	}
	return "This invite link is not valid. Check that you copied the whole link from your email."
}

func (h *Web) joinFailed(w http.ResponseWriter, r *http.Request, inv *Invite, v views.LandingData, err error) {
	if ReasonOf(err) != "" {
		h.failed(w, r, inv, err)
		return
	}
	ae, ok := apperror.AsAppError(err)
	if !ok || apperror.KindOf(ae.Code()) != apperror.KindInvalid {
		h.o.Kit.Fail(w, r, "web: invite join", err)
		return
	}
	v.Error = ae.Message()
	h.render(w, r, http.StatusUnprocessableEntity, v)
}

func (h *Web) done(w http.ResponseWriter, r *http.Request, inv *Invite) {
	h.cookie.Clear(w)
	slug, err := h.o.OrgLinks.Slug(r.Context(), inv.OrgID)
	if err != nil {
		h.o.Kit.Fail(w, r, "web: invite org", err)
		return
	}
	http.Redirect(w, r, h.consoleURL(slug), http.StatusSeeOther) //nolint:gosec // G710: a path under the console, slug escaped.
}

func (h *Web) render(w http.ResponseWriter, r *http.Request, status int, v views.LandingData) {
	h.o.Kit.Render(w, r, status, views.Landing(h.o.Kit.Layout("Join "+v.Org), v))
}

func (h *Web) consoleURL(slug string) string {
	return h.o.Kit.Path("/console") + "/o/" + url.PathEscape(slug)
}

func (h *Web) loginURL(email string) string {
	q := url.Values{"return_to": {h.o.Kit.Path("/invite")}, "email": {email}}
	return h.o.Kit.Path("/login") + "?" + q.Encode()
}

func (h *Web) readCookie(r *http.Request) (publicID string, issued int64, ok bool) {
	value, ok := h.cookie.Get(r)
	if !ok {
		return "", 0, false
	}
	publicID, exp, ok := strings.Cut(value, "|")
	if !ok {
		return "", 0, false
	}
	issued, err := strconv.ParseInt(exp, 10, 64)
	if err != nil {
		return "", 0, false
	}
	return publicID, issued, true
}
