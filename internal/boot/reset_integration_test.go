//go:build integration

package boot

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/auth"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/mailer"
)

func claimMail(t *testing.T, svcs *Services) []mailer.Message {
	t.Helper()
	entries, err := svcs.MailQueue.ClaimDue(t.Context(), time.Now().Add(time.Minute), 10)
	require.NoError(t, err)
	out := make([]mailer.Message, 0, len(entries))
	for _, e := range entries {
		m, err := svcs.MailQueue.Open(e)
		require.NoError(t, err)
		out = append(out, m)
	}
	return out
}

func serve(h http.Handler, req *http.Request, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func formPost(path string, v url.Values) *http.Request {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(v.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func TestPasswordReset_EndToEnd(t *testing.T) {
	cfg := devLocalConfig()
	cfg.HTTP.StateSecret = strings.Repeat("s", 40)
	cfg.HTTP.BaseURL = "https://oagent.example.com"
	cfg.Secrets.EncryptionKey = testKeyHex
	cfg.Auth.SessionTTL = 24 * time.Hour
	cfg.Auth.Genesis = config.GenesisConfig{Email: "root@example.com", Password: "genesis-password"}
	cfg.Compliance.RequireAcceptance = true
	svcs, cfg := migratedServices(t, cfg)
	svcs.Caps.EnvGenesis = true
	require.NoError(t, firstBoot(t.Context(), cfg, svcs, testLogger(), false))
	h := buildHTTPHandler(httpSurfaces{
		routes: buildRoutes(cfg, svcs, nil, nil), session: buildSessionMiddleware(svcs, nil),
		requireAcceptance: true, termsUpdated: svcs.Legal.TermsUpdatedAt(),
	})
	ctx := t.Context()
	p, err := svcs.Auth.LoginLocal(ctx, auth.Credentials{Email: "root@example.com", Password: "genesis-password"})
	require.NoError(t, err)
	p.TermsAcceptedAt = time.Time{}
	require.NoError(t, svcs.Sessions.Save(ctx, "old-sid", p, time.Now().Add(time.Hour)))
	old := &http.Cookie{Name: session.CookieName, Value: session.Sign(svcs.StateSecret, "old-sid")}

	rec := serve(h, formPost("/forgot-password", url.Values{"email": {"Root@Example.com"}}))
	require.Equal(t, http.StatusOK, rec.Code)
	unknown := serve(h, formPost("/forgot-password", url.Values{"email": {"ghost@example.com"}}))
	assert.Equal(t, rec.Body.String(), unknown.Body.String())
	mail := claimMail(t, svcs)
	require.Len(t, mail, 1, "one email, for the known address only")
	i := strings.Index(mail[0].TextBody, "https://oagent.example.com/reset-password/")
	require.GreaterOrEqual(t, i, 0)
	link := strings.Fields(mail[0].TextBody[i+len("https://oagent.example.com"):])[0]

	gated := serve(h, httptest.NewRequest(http.MethodGet, "/somewhere", nil), old)
	require.Equal(t, http.StatusSeeOther, gated.Code, "this person owes the terms, so the gate is on")
	require.Contains(t, gated.Header().Get("Location"), "/welcome")
	rec = serve(h, httptest.NewRequest(http.MethodGet, link, nil), old)
	require.Equal(t, http.StatusSeeOther, rec.Code, "the welcome gate lets the reset link through")
	assert.Equal(t, "/reset-password", rec.Header().Get("Location"))
	resetCookie := rec.Result().Cookies()[0]

	rec = serve(h, formPost("/reset-password", url.Values{"password": {"a brand new passphrase"}}), resetCookie)
	require.Equal(t, http.StatusSeeOther, rec.Code, rec.Body.String())
	assert.Equal(t, "/login?reset=done", rec.Header().Get("Location"))

	_, ok, err := svcs.Sessions.Load(ctx, "old-sid")
	require.NoError(t, err)
	assert.False(t, ok, "the old session is gone")
	changed := claimMail(t, svcs)
	require.Len(t, changed, 1)
	assert.Equal(t, "Your oagent password was changed", changed[0].Subject)

	_, err = svcs.Auth.LoginLocal(ctx, auth.Credentials{Email: "root@example.com", Password: "a brand new passphrase"})
	require.NoError(t, err, "the new password signs in")
	_, err = svcs.Auth.LoginLocal(ctx, auth.Credentials{Email: "root@example.com", Password: "genesis-password"})
	assert.True(t, auth.IsInvalidCredentialsError(err))

	rec = serve(h, formPost("/reset-password", url.Values{"password": {"another passphrase here"}}), resetCookie)
	assert.Equal(t, http.StatusGone, rec.Code, "the link worked once")
}
