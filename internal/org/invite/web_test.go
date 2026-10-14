package invite_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/auth"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/org/invite"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/user"
	"altalune.id/oagent/internal/web"
	"altalune.id/oagent/internal/web/layout"
	"altalune.id/oagent/internal/web/middleware"
)

var webSecret = []byte("0123456789abcdef0123456789abcdef")

type webFixture struct {
	*fixture
	srv      http.Handler
	sessions *session.MemoryStore
	signIn   *fakeSignIn
}

type fakeSignIn struct {
	users  *user.Service
	logout []string
}

func (f *fakeSignIn) LoginLocal(ctx context.Context, c auth.Credentials) (session.Principal, error) {
	u, err := f.users.ByEmail(ctx, c.Email)
	if err != nil {
		return session.Principal{}, &auth.InvalidCredentialsError{}
	}
	return session.Principal{UserID: u.ID, Email: u.Email, Name: u.Name, Source: session.SourceLocal}, nil
}

func (f *fakeSignIn) Logout(_ context.Context, sid string) error {
	f.logout = append(f.logout, sid)
	return nil
}

type passThrough struct{}

func (passThrough) Refresh(_ context.Context, p session.Principal) (session.Principal, error) {
	return p, nil
}

type slugs map[uuid.UUID]string

func (s slugs) Slug(_ context.Context, orgID uuid.UUID) (string, error) { return s[orgID], nil }

func newWebFixture(t *testing.T, requireTerms bool) *webFixture {
	return newLimitedWebFixture(t, requireTerms, auth.Limits{})
}

func newLimitedWebFixture(t *testing.T, requireTerms bool, limits auth.Limits) *webFixture {
	f := &webFixture{fixture: newFixture(t), sessions: session.NewMemoryStore()}
	f.signIn = &fakeSignIn{users: f.users}
	cookies := auth.CookieSessions{Store: f.sessions, Secret: webSecret, Cookie: session.CookieOptions{Path: "/", Secure: true}, TTL: time.Hour}
	f.srv = web.NewServer(web.ServerOpts{
		Session: middleware.Session(middleware.SessionConfig{Store: f.sessions, Secret: webSecret, Auth: passThrough{}}),
		Routes: []web.Registrar{invite.NewWeb(invite.WebOpts{
			Kit: layout.Kit{}, Invites: f.svc, Users: f.users, Members: f.members,
			Orgs: orgNames{f.org: "Acme"}, OrgLinks: slugs{f.org: "acme"}, People: directory{people: f.people, users: f.users},
			SignIn: f.signIn, Sessions: cookies, Consent: layout.ConsentData{Required: requireTerms},
			Methods: []string{invite.MethodPassword}, StateSecret: webSecret, CookieSecure: true,
			Now: func() time.Time { return f.clock }, Limits: limits,
		})},
	})
	return f
}

func (f *webFixture) do(req *http.Request, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	for _, c := range cookies {
		if c != nil {
			req.AddCookie(c)
		}
	}
	rec := httptest.NewRecorder()
	f.srv.ServeHTTP(rec, req)
	return rec
}

func (f *webFixture) get(path string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	return f.do(httptest.NewRequest(http.MethodGet, path, nil), cookies...)
}

func (f *webFixture) post(path string, form url.Values, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return f.do(req, cookies...)
}

func (f *webFixture) open(t *testing.T, c *invite.Created) *http.Cookie {
	t.Helper()
	rec := f.get(strings.TrimPrefix(c.Link, strings.TrimSuffix(baseURL, "/")))
	require.Equal(t, http.StatusSeeOther, rec.Code, rec.Body.String())
	ck := cookieNamed(rec, "oagent_invite")
	require.NotNil(t, ck)
	return ck
}

func (f *webFixture) sessionFor(t *testing.T, email string) *http.Cookie {
	t.Helper()
	u, err := f.users.ByEmail(f.ctx, email)
	require.NoError(t, err)
	sid, err := session.NewID()
	require.NoError(t, err)
	p := session.Principal{UserID: u.ID, Email: u.Email, Source: session.SourceLocal}
	require.NoError(t, f.sessions.Save(f.ctx, sid, p, time.Now().Add(time.Hour)))
	return &http.Cookie{Name: session.CookieName, Value: session.Sign(webSecret, sid)}
}

func (f *webFixture) account(t *testing.T, email string) uuid.UUID {
	t.Helper()
	u, err := f.users.Create(f.ctx, user.CreateRequest{Email: email, Name: "Existing", Password: "a sturdy passphrase"})
	require.NoError(t, err)
	return u.ID
}

func cookieNamed(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func joinForm(name, pass string) url.Values {
	return url.Values{"name": {name}, "password": {pass}, "accept_terms": {"1"}}
}

func TestWeb_LinkSetsTheCookieAndLeavesTheURL(t *testing.T) {
	f := newWebFixture(t, false)
	c := f.create("bob@x.io", member.RoleMember)
	writes := f.store.Writes
	rec := f.get(strings.TrimPrefix(c.Link, "https://oagent.example.com"))

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/invite", rec.Header().Get("Location"))
	assert.Equal(t, "no-referrer", rec.Header().Get("Referrer-Policy"))
	ck := cookieNamed(rec, "oagent_invite")
	require.NotNil(t, ck)
	assert.True(t, ck.HttpOnly)
	assert.True(t, ck.Secure)
	assert.Equal(t, http.SameSiteLaxMode, ck.SameSite)
	assert.Equal(t, "/invite", ck.Path)
	assert.InDelta(t, invite.TTL.Seconds(), ck.MaxAge, 2, "the cookie lives until the invite expires")
	assert.NotContains(t, ck.Value, rawToken(t, c.Link), "the token never goes into the cookie")
	assert.NotContains(t, ck.Value, c.Invite.ID.String(), "nor the UUID")
	assert.Contains(t, ck.Value, c.Invite.PublicID)
	assert.Equal(t, writes, f.store.Writes, "a GET writes nothing")
}

func TestWeb_DeadLinks(t *testing.T) {
	f := newWebFixture(t, false)
	used := f.create("used@x.io", member.RoleMember)
	_, err := f.svc.Accept(f.ctx, used.Invite.ID, f.newcomer("used@x.io"))
	require.NoError(t, err)
	revoked := f.create("revoked@x.io", member.RoleMember)
	require.NoError(t, f.svc.Revoke(f.ctx, f.owner, f.org, revoked.Invite.ID))
	expired := f.create("late@x.io", member.RoleMember)

	cases := map[string]struct {
		path string
		want string
	}{
		"unknown": {"/invite/nope", "This invite link is not valid."},
		"used":    {strings.TrimPrefix(used.Link, "https://oagent.example.com"), "This invite was already accepted."},
		"revoked": {strings.TrimPrefix(revoked.Link, "https://oagent.example.com"), "This invite was cancelled."},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			rec := f.get(tc.path)
			assert.Equal(t, http.StatusGone, rec.Code)
			assert.Contains(t, rec.Body.String(), tc.want)
			ck := cookieNamed(rec, "oagent_invite")
			assert.True(t, ck == nil || ck.MaxAge < 0, "no invite cookie")
			if name == "used" {
				assert.Contains(t, rec.Body.String(), `href="/login"`, "signed out: a way in")
			} else {
				assert.NotContains(t, rec.Body.String(), `href="/login"`)
			}
		})
	}
	f.clock = f.clock.Add(invite.TTL)
	rec := f.get(strings.TrimPrefix(expired.Link, "https://oagent.example.com"))
	assert.Equal(t, http.StatusGone, rec.Code)
	assert.Contains(t, rec.Body.String(), "This invite has expired.")
}

func TestWeb_NoCookie(t *testing.T) {
	f := newWebFixture(t, false)
	for _, ck := range []*http.Cookie{nil, {Name: "oagent_invite", Value: "forged"}} {
		rec := f.get("/invite", ck)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "This invite link is incomplete. Open the link from your email again.")
	}
}

func TestWeb_NewPersonJoinsInOneStep(t *testing.T) {
	f := newWebFixture(t, true)
	c := f.create("bob@x.io", member.RoleAdmin)
	ck := f.open(t, c)

	page := f.get("/invite", ck)
	require.Equal(t, http.StatusOK, page.Code)
	body := page.Body.String()
	assert.Contains(t, body, "Olga invited you to join Acme as Admin.")
	assert.Contains(t, body, `name="email" value="bob@x.io"`)
	assert.Contains(t, body, `readonly`)
	assert.Contains(t, body, `name="name"`)
	assert.Contains(t, body, `data-password-field data-email-input="email" data-name-input="name"`)
	assert.Contains(t, body, `name="accept_terms" value="1" required`)
	assert.Contains(t, body, `action="/invite/join"`)

	rec := f.post("/invite/join", joinForm("Bob", "a sturdy passphrase"), ck)
	require.Equal(t, http.StatusSeeOther, rec.Code, rec.Body.String())
	assert.Equal(t, "/console/o/acme", rec.Header().Get("Location"))
	assert.NotNil(t, cookieNamed(rec, session.CookieName), "signed in")
	gone := cookieNamed(rec, "oagent_invite")
	require.NotNil(t, gone)
	assert.Negative(t, gone.MaxAge, "the invite cookie is cleared")

	u, err := f.users.ByEmail(f.ctx, "bob@x.io")
	require.NoError(t, err)
	assert.Equal(t, "Bob", u.Name)
	assert.NotNil(t, u.TermsAcceptedAt)
	m, err := f.members.Find(f.ctx, f.org, u.ID)
	require.NoError(t, err)
	assert.Equal(t, member.RoleAdmin, m.Role)
}

func TestWeb_JoinNeedsTheBoxWhenRequired(t *testing.T) {
	f := newWebFixture(t, true)
	ck := f.open(t, f.create("bob@x.io", member.RoleMember))
	form := joinForm("Bob", "a sturdy passphrase")
	form.Del("accept_terms")
	rec := f.post("/invite/join", form, ck)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	assert.Contains(t, rec.Body.String(), "Please accept the terms to continue.")
	_, err := f.users.ByEmail(f.ctx, "bob@x.io")
	assert.True(t, user.IsNotFoundError(err))
}

func TestWeb_JoinWithoutTermsWhenNotRequired(t *testing.T) {
	f := newWebFixture(t, false)
	ck := f.open(t, f.create("bob@x.io", member.RoleMember))
	assert.NotContains(t, f.get("/invite", ck).Body.String(), "accept_terms")
	rec := f.post("/invite/join", joinForm("Bob", "a sturdy passphrase"), ck)
	require.Equal(t, http.StatusSeeOther, rec.Code)
	u, err := f.users.ByEmail(f.ctx, "bob@x.io")
	require.NoError(t, err)
	assert.Nil(t, u.TermsAcceptedAt, "acceptance is recorded only when asked for")
}

func TestWeb_JoinWeakPassword(t *testing.T) {
	f := newWebFixture(t, true)
	ck := f.open(t, f.create("bob@x.io", member.RoleMember))
	rec := f.post("/invite/join", joinForm("Bob", "tiny-pass"), ck)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "Password must be 12 to 256 characters")
	assert.NotContains(t, body, "tiny-pass", "the password is not echoed")
	assert.Contains(t, body, `value="Bob"`, "the name is kept")
	_, err := f.users.ByEmail(f.ctx, "bob@x.io")
	assert.True(t, user.IsNotFoundError(err))
}

func TestWeb_JoinWhenTheEmailNowHasAnAccount(t *testing.T) {
	f := newWebFixture(t, true)
	ck := f.open(t, f.create("bob@x.io", member.RoleMember))
	assert.Contains(t, f.get("/invite", ck).Body.String(), `action="/invite/join"`)
	existing := f.account(t, "bob@x.io")

	rec := f.post("/invite/join", joinForm("Bob", "a sturdy passphrase"), ck)
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), "Sign in to join")
	u, err := f.users.ByEmail(f.ctx, "bob@x.io")
	require.NoError(t, err)
	assert.Equal(t, existing, u.ID, "never a second user")
	_, err = f.members.Find(f.ctx, f.org, existing)
	assert.True(t, member.IsNotFoundError(err), "nothing joined")
}

func TestWeb_ExistingAccountSignedOut(t *testing.T) {
	f := newWebFixture(t, false)
	f.account(t, "bob@x.io")
	ck := f.open(t, f.create("bob@x.io", member.RoleMember))
	rec := f.get("/invite", ck)
	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "Sign in to join")
	assert.Contains(t, body, `href="/login?email=bob%40x.io&amp;return_to=%2Finvite"`)
	assert.NotContains(t, body, `action="/invite/join"`)

	accept := f.post("/invite/accept", nil, ck)
	assert.Equal(t, http.StatusSeeOther, accept.Code, "accepting needs a sign-in first")
	assert.Equal(t, "/login?email=bob%40x.io&return_to=%2Finvite", accept.Header().Get("Location"))
}

func TestWeb_SignedInAsTheInvitedEmail(t *testing.T) {
	f := newWebFixture(t, false)
	id := f.account(t, "bob@x.io")
	ck := f.open(t, f.create("bob@x.io", member.RoleMember))
	s := f.sessionFor(t, "bob@x.io")

	page := f.get("/invite", ck, s)
	assert.Contains(t, page.Body.String(), "Join Acme as Member, invited by Olga.")
	assert.Contains(t, page.Body.String(), `action="/invite/accept"`)

	rec := f.post("/invite/accept", nil, ck, s)
	require.Equal(t, http.StatusSeeOther, rec.Code, rec.Body.String())
	assert.Equal(t, "/console/o/acme", rec.Header().Get("Location"))
	m, err := f.members.Find(f.ctx, f.org, id)
	require.NoError(t, err)
	assert.Equal(t, member.RoleMember, m.Role)

	again := f.get("/invite", ck, s)
	assert.Equal(t, http.StatusGone, again.Code, "the invite is used")
	assert.Contains(t, again.Body.String(), `href="/console/o/acme"`, "used by you: Open Acme")
	assert.NotContains(t, again.Body.String(), `href="/login"`, "already signed in")
}

func TestWeb_AlreadyAMember(t *testing.T) {
	f := newWebFixture(t, false)
	c := f.create("olga@example.com", member.RoleMember)
	_, err := f.users.Create(f.ctx, user.CreateRequest{Email: "olga@example.com", Password: "a sturdy passphrase"})
	require.NoError(t, err)
	u, err := f.users.ByEmail(f.ctx, "olga@example.com")
	require.NoError(t, err)
	_, err = f.members.Add(f.ctx, f.org, u.ID, member.RoleOwner)
	require.NoError(t, err)
	ck := f.open(t, c)
	s := f.sessionFor(t, "olga@example.com")

	writes := f.store.Writes
	page := f.get("/invite", ck, s)
	assert.Contains(t, page.Body.String(), "You're already a member of Acme.")
	assert.Contains(t, page.Body.String(), "Open Acme")
	assert.Equal(t, writes, f.store.Writes, "showing the page writes nothing")

	rec := f.post("/invite/accept", nil, ck, s)
	require.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/console/o/acme", rec.Header().Get("Location"))
	m, err := f.members.Find(f.ctx, f.org, u.ID)
	require.NoError(t, err)
	assert.Equal(t, member.RoleOwner, m.Role, "the owner stays owner")
	stored, err := f.store.ByID(f.ctx, c.Invite.ID)
	require.NoError(t, err)
	assert.NotNil(t, stored.AcceptedAt, "the invite is marked used")
}

func TestWeb_SignedInAsAnotherEmail(t *testing.T) {
	f := newWebFixture(t, false)
	f.account(t, "carol@x.io")
	c := f.create("bob@x.io", member.RoleMember)
	ck := f.open(t, c)
	s := f.sessionFor(t, "carol@x.io")

	page := f.get("/invite", ck, s)
	assert.Contains(t, page.Body.String(), "This invite is for bob@x.io, but you're signed in as carol@x.io.")
	assert.Contains(t, page.Body.String(), `action="/invite/switch"`)

	refused := f.post("/invite/accept", nil, ck, s)
	assert.Equal(t, http.StatusForbidden, refused.Code)
	stored, err := f.store.ByID(f.ctx, c.Invite.ID)
	require.NoError(t, err)
	assert.Nil(t, stored.AcceptedAt, "the invite is untouched")

	rec := f.post("/invite/switch", nil, ck, s)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/invite", rec.Header().Get("Location"))
	assert.Len(t, f.signIn.logout, 1, "the session ends")
	cleared := cookieNamed(rec, session.CookieName)
	require.NotNil(t, cleared)
	assert.Negative(t, cleared.MaxAge)
	assert.Nil(t, cookieNamed(rec, "oagent_invite"), "the invite cookie stays")
}

func TestWeb_DraftsNeverRender(t *testing.T) {
	f := newWebFixture(t, false)
	d, err := f.svc.Draft(f.ctx, f.owner, f.org, "dina@x.io")
	require.NoError(t, err)
	ck := &http.Cookie{Name: "oagent_invite", Value: session.Sign(webSecret, "oagent_invite="+d.PublicID+"|"+strconv.FormatInt(d.ExpiresAt.Unix(), 10))}
	rec := f.get("/invite", ck)
	assert.Equal(t, http.StatusGone, rec.Code)
	assert.Contains(t, rec.Body.String(), "This invite link is not valid.")
}

func TestWeb_CookieStillWorksDaysLater(t *testing.T) {
	f := newWebFixture(t, false)
	ck := f.open(t, f.create("bob@x.io", member.RoleMember))
	f.clock = f.clock.Add(3 * 24 * time.Hour)
	page := f.get("/invite", ck)
	require.Equal(t, http.StatusOK, page.Code)
	rec := f.post("/invite/join", joinForm("Bob", "a sturdy passphrase"), ck)
	require.Equal(t, http.StatusSeeOther, rec.Code, rec.Body.String())
	assert.Equal(t, "/console/o/acme", rec.Header().Get("Location"))
}

func TestWeb_ResendMakesAnOldCookieStale(t *testing.T) {
	f := newWebFixture(t, false)
	c := f.create("bob@x.io", member.RoleMember)
	ck := f.open(t, c)
	f.clock = f.clock.Add(time.Hour)
	_, err := f.svc.Resend(f.ctx, f.owner, f.org, c.Invite.ID)
	require.NoError(t, err)
	rec := f.get("/invite", ck)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "Open the link from your email again.")
}

func TestWeb_ExpiredWhileTheCookieLives(t *testing.T) {
	f := newWebFixture(t, false)
	ck := f.open(t, f.create("bob@x.io", member.RoleMember))
	f.clock = f.clock.Add(invite.TTL)
	rec := f.get("/invite", ck)
	assert.Equal(t, http.StatusGone, rec.Code)
	assert.Contains(t, rec.Body.String(), "This invite has expired.")
	gone := cookieNamed(rec, "oagent_invite")
	require.NotNil(t, gone)
	assert.Negative(t, gone.MaxAge, "a definitive failure clears the cookie")
}

func TestWeb_GetsNeverWrite(t *testing.T) {
	f := newWebFixture(t, false)
	c := f.create("bob@x.io", member.RoleMember)
	writes := f.store.Writes
	link := strings.TrimPrefix(c.Link, "https://oagent.example.com")
	ck := f.open(t, c)
	f.get(link)
	for range 5 {
		assert.Equal(t, http.StatusOK, f.get("/invite", ck).Code)
	}
	assert.Equal(t, writes, f.store.Writes)
	stored, err := f.store.ByID(f.ctx, c.Invite.ID)
	require.NoError(t, err)
	assert.Nil(t, stored.AcceptedAt)
}

func TestWeb_NoSignInMethod(t *testing.T) {
	f := newWebFixture(t, false)
	ck := f.open(t, f.create("bob@x.io", member.RoleMember))
	h := invite.NewWeb(invite.WebOpts{
		Invites: f.svc, Users: f.users, Members: f.members, Orgs: orgNames{f.org: "Acme"}, OrgLinks: slugs{f.org: "acme"},
		StateSecret: webSecret, Now: func() time.Time { return f.clock },
	})
	mux := http.NewServeMux()
	h.Register(mux)
	req := httptest.NewRequest(http.MethodGet, "/invite", nil)
	req.AddCookie(ck)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "this server has no way to sign in yet")
	assert.NotContains(t, rec.Body.String(), `action="/invite/join"`)
}
