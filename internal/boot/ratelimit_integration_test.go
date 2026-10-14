//go:build integration

package boot

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/config"
)

func loginServer(t *testing.T, rl config.RateLimitConfig) http.Handler {
	t.Helper()
	cfg := devLocalConfig()
	cfg.HTTP.StateSecret = strings.Repeat("s", 40)
	cfg.Secrets.EncryptionKey = testKeyHex
	cfg.Auth.SessionTTL = 24 * time.Hour
	cfg.Auth.Genesis = config.GenesisConfig{Email: "root@example.com", Password: "genesis-password"}
	cfg.RateLimit = rl
	svcs, cfg := migratedServices(t, cfg)
	svcs.Caps.EnvGenesis = true
	require.NoError(t, firstBoot(t.Context(), cfg, svcs, testLogger(), false))
	lims, err := buildLimiters(cfg, testLogger())
	require.NoError(t, err)
	svcs.AuthLimiter = lims.auth
	return buildHTTPHandler(httpSurfaces{routes: buildRoutes(cfg, svcs, nil, nil), session: buildSessionMiddleware(svcs, nil)})
}

func postLogin(h http.Handler, email, password, ip string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(url.Values{"email": {email}, "password": {password}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = ip + ":4711"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// Review Focus 2: with the default config, 100 failed logins for one email from one IP never give 429.
func TestLoginRateLimit_OffByDefault(t *testing.T) {
	h := loginServer(t, config.RateLimitConfig{Store: config.RateLimitStoreMemory, MaxKeys: 100_000})
	for i := range 100 {
		rec := postLogin(h, "root@example.com", "wrong-password", "192.0.2.1")
		require.Equal(t, http.StatusUnauthorized, rec.Code, "attempt %d", i+1)
	}
	assert.Equal(t, http.StatusSeeOther, postLogin(h, "root@example.com", "genesis-password", "192.0.2.1").Code)
}

// Review Focus 3: with rateLimit.auth.enabled, login is limited per email across IPs; a correct login before the limit works.
func TestLoginRateLimit_Enabled(t *testing.T) {
	h := loginServer(t, config.RateLimitConfig{
		Store: config.RateLimitStoreMemory, MaxKeys: 100_000, Auth: config.RateLimitGroupConfig{Enabled: true},
	})
	for i := range 4 {
		require.Equal(t, http.StatusUnauthorized, postLogin(h, "root@example.com", "wrong-password", "192.0.2."+strconv.Itoa(i+1)).Code)
	}
	require.Equal(t, http.StatusSeeOther, postLogin(h, "root@example.com", "genesis-password", "192.0.2.50").Code)
	for i := range 5 {
		require.Equal(t, http.StatusUnauthorized, postLogin(h, "root@example.com", "wrong-password", "198.51.100."+strconv.Itoa(i+1)).Code)
	}
	rec := postLogin(h, "root@example.com", "genesis-password", "203.0.113.1")
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	wait, err := strconv.Atoi(rec.Header().Get("Retry-After"))
	require.NoError(t, err)
	assert.True(t, wait >= 1 && wait <= 90, "Retry-After %d is within one 90 s interval of the real clock", wait)
	rec = postLogin(h, "nobody@example.com", "wrong-password", "203.0.113.1")
	assert.Equal(t, http.StatusUnauthorized, rec.Code, "another email from a fresh IP is not limited")
}
