package auth

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/testutil/fakes"
	"altalune.id/oagent/internal/web"
	"altalune.id/oagent/internal/web/layout"
)

type basicStub struct{}

func (basicStub) Basic(_ context.Context, user, pass string) (session.Principal, error) {
	if user == "op" && pass == "pw" {
		return session.Principal{Source: session.SourceBasic, Name: "op", Operator: true}, nil
	}
	return session.Principal{}, &InvalidCredentialsError{}
}

func consoleSeen(p *session.Principal) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*p = session.PrincipalFrom(r.Context())
		w.WriteHeader(http.StatusOK)
	})
}

func basicHeader(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}

func TestConsoleGate_LocalAnonymousGoesToLogin(t *testing.T) {
	var seen session.Principal
	h := ConsoleGate("/app", nil)(consoleSeen(&seen))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app/console/o/acme?tab=keys", nil))

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/app/login?return_to=%2Fapp%2Fconsole%2Fo%2Facme%3Ftab%3Dkeys", rec.Header().Get("Location"))
}

func TestConsoleGate_PersonPasses(t *testing.T) {
	var seen session.Principal
	p := session.Principal{Source: session.SourceLocal, UserID: uuid.New()}
	h := ConsoleGate("", nil)(consoleSeen(&seen))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/console/", nil).WithContext(session.PrincipalInto(context.Background(), p)))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, p.UserID, seen.UserID)
}

func TestConsoleGate_BasicMode(t *testing.T) {
	var seen session.Principal
	h := ConsoleGate("", basicStub{})(consoleSeen(&seen))

	for name, header := range map[string]string{"none": "", "wrong": basicHeader("op", "nope"), "bearer": "Bearer oagent_x"} {
		req := httptest.NewRequest(http.MethodGet, "/console/", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code, name)
		assert.Equal(t, `Basic realm="oagent"`, rec.Header().Get("WWW-Authenticate"), name)
	}

	req := httptest.NewRequest(http.MethodGet, "/console/", nil)
	req.Header.Set("Authorization", basicHeader("op", "pw"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, seen.Operator)
	assert.Equal(t, session.SourceBasic, seen.Source)
}

func TestBasicChallenge(t *testing.T) {
	reached := false
	h := BasicChallenge(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/org.v1.OrgService/ListOrgs", nil))
	assert.False(t, reached)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, `Basic realm="oagent"`, rec.Header().Get("WWW-Authenticate"))
	assert.JSONEq(t, `{"code":"unauthenticated","message":"Sign in required"}`, rec.Body.String())

	for name, set := range map[string]func(*http.Request){
		"authorization": func(r *http.Request) { r.Header.Set("Authorization", basicHeader("op", "pw")) },
		"cookie":        func(r *http.Request) { r.Header.Set("Cookie", session.CookieName+"=x") },
	} {
		reached = false
		req := httptest.NewRequest(http.MethodPost, "/api/x", nil)
		set(req)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		assert.True(t, reached, name)
	}
}

// SECURITY: the gate covers the console only; sign-in pages stay reachable or nobody could sign in.
func TestConsoleGate_OnlyTheConsoleIsWrapped(t *testing.T) {
	kit := layout.Kit{}
	srv := web.NewServer(web.ServerOpts{
		Console:     http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }),
		Routes:      []web.Registrar{NewWeb(kit, &fakeAuth{}, CookieSessions{Store: session.NewMemoryStore(), Secret: []byte("0123456789abcdef0123456789abcdef")}, fakes.NewPublicIDs())},
		ConsoleGate: ConsoleGate("", nil),
	})

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/console/o/x/agents", nil))
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/login?return_to=%2Fconsole%2Fo%2Fx%2Fagents", rec.Header().Get("Location"))
}
