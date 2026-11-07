package ingest_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/ingest"
	"altalune.id/oagent/internal/kernel/ratelimit"
	"altalune.id/oagent/internal/kernel/ratelimit/memory"
)

type fakeResolver struct {
	conns map[string]ingest.Connection
	err   error
	asked []string
}

func (f *fakeResolver) Resolve(_ context.Context, publicID string) (ingest.Connection, bool, error) {
	f.asked = append(f.asked, publicID)
	if f.err != nil {
		return ingest.Connection{}, false, f.err
	}
	c, ok := f.conns[publicID]
	return c, ok, nil
}

type fakeVerifier struct {
	err  error
	seen []byte
	conn ingest.Connection
}

func (v *fakeVerifier) Verify(_ *http.Request, body []byte, c ingest.Connection) error {
	v.seen, v.conn = body, c
	return v.err
}

type spyHandler struct {
	reached bool
	body    []byte
	conn    ingest.Connection
}

func (s *spyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.reached = true
	s.body, _ = io.ReadAll(r.Body)
	s.conn, _ = ingest.ConnectionFrom(r.Context())
	w.WriteHeader(http.StatusAccepted)
}

func resolver() *fakeResolver {
	return &fakeResolver{conns: map[string]ingest.Connection{"cnx_1": {PublicID: "cnx_1", Secrets: []string{"whsec_from_db"}, Value: "row"}}}
}

func post(t *testing.T, h http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
	return rec
}

func requireEnvelope(t *testing.T, rec *httptest.ResponseRecorder, wantCode string) {
	t.Helper()
	require.True(t, strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json"), "Content-Type = %q", rec.Header().Get("Content-Type"))
	require.NotContains(t, rec.Body.String(), "<html")
	var got struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got), "body: %s", rec.Body.String())
	require.Equal(t, wantCode, got.Code, "body=%s", rec.Body.String())
}

// SECURITY: provider identity is the whole authorization here, so every way out before Verify passes is a refusal.
func TestHandlerFailsClosed(t *testing.T) {
	tests := []struct {
		name       string
		provider   ingest.Provider
		registered bool
		path       string
		body       string
		wantStatus int
		wantCode   string
	}{
		{name: "no provider registered", path: "/hooks/openwa/cnx_1", wantStatus: http.StatusUnauthorized, wantCode: "unauthorized"},
		{
			name: "unknown provider", registered: true, provider: ingest.Provider{Resolver: resolver(), Verifier: &fakeVerifier{}, Handler: &spyHandler{}},
			path: "/hooks/nosuch/cnx_1", wantStatus: http.StatusUnauthorized, wantCode: "unauthorized",
		},
		{
			name: "subtree root names nothing", registered: true, provider: ingest.Provider{Resolver: resolver(), Verifier: &fakeVerifier{}, Handler: &spyHandler{}},
			path: "/hooks/", wantStatus: http.StatusUnauthorized, wantCode: "unauthorized",
		},
		{
			name: "no connection segment", registered: true, provider: ingest.Provider{Resolver: resolver(), Verifier: &fakeVerifier{}, Handler: &spyHandler{}},
			path: "/hooks/openwa", wantStatus: http.StatusUnauthorized, wantCode: "unauthorized",
		},
		{
			name: "extra segment", registered: true, provider: ingest.Provider{Resolver: resolver(), Verifier: &fakeVerifier{}, Handler: &spyHandler{}},
			path: "/hooks/openwa/cnx_1/events", wantStatus: http.StatusUnauthorized, wantCode: "unauthorized",
		},
		{
			name: "unknown connection", registered: true, provider: ingest.Provider{Resolver: resolver(), Verifier: &fakeVerifier{}, Handler: &spyHandler{}},
			path: "/hooks/openwa/cnx_nope", body: `{"id":"1"}`, wantStatus: http.StatusUnauthorized, wantCode: "unauthorized",
		},
		{
			name: "signature does not verify", registered: true,
			provider: ingest.Provider{Resolver: resolver(), Verifier: &fakeVerifier{err: errors.New("bad signature")}, Handler: &spyHandler{}},
			path:     "/hooks/openwa/cnx_1", body: `{"id":"1"}`, wantStatus: http.StatusUnauthorized, wantCode: "unauthorized",
		},
		{
			name: "provider with no verifier", registered: true, provider: ingest.Provider{Resolver: resolver(), Handler: &spyHandler{}},
			path: "/hooks/openwa/cnx_1", body: `{"id":"1"}`, wantStatus: http.StatusUnauthorized, wantCode: "unauthorized",
		},
		{
			name: "provider with no resolver", registered: true, provider: ingest.Provider{Verifier: &fakeVerifier{}, Handler: &spyHandler{}},
			path: "/hooks/openwa/cnx_1", body: `{"id":"1"}`, wantStatus: http.StatusUnauthorized, wantCode: "unauthorized",
		},
		{
			name: "verified delivery with nowhere to go", registered: true, provider: ingest.Provider{Resolver: resolver(), Verifier: &fakeVerifier{}},
			path: "/hooks/openwa/cnx_1", body: `{"id":"1"}`, wantStatus: http.StatusInternalServerError, wantCode: "internal",
		},
		{
			name: "resolver fails", registered: true,
			provider: ingest.Provider{Resolver: &fakeResolver{err: errors.New("db down")}, Verifier: &fakeVerifier{}, Handler: &spyHandler{}},
			path:     "/hooks/openwa/cnx_1", body: `{"id":"1"}`, wantStatus: http.StatusServiceUnavailable, wantCode: "unavailable",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			providers := map[string]ingest.Provider{}
			if tt.registered {
				providers["openwa"] = tt.provider
			}
			spy, _ := tt.provider.Handler.(*spyHandler)
			rec := post(t, ingest.NewHandler(ingest.HandlerParams{Providers: providers}), tt.path, tt.body)
			if spy != nil {
				require.False(t, spy.reached, "SECURITY: an unverified delivery reached a provider handler; status=%d", rec.Code)
			}
			require.Equal(t, tt.wantStatus, rec.Code, "body=%s", rec.Body.String())
			requireEnvelope(t, rec, tt.wantCode)
		})
	}
}

// SECURITY: a caller probing ids or signatures learns nothing: an unknown provider, an unknown connection and a bad signature answer the same bytes.
func TestIngest_UnknownConnectionSameAs401(t *testing.T) {
	h := ingest.NewHandler(ingest.HandlerParams{Providers: map[string]ingest.Provider{
		"openwa": {Resolver: resolver(), Verifier: &fakeVerifier{err: errors.New("digest mismatch")}, Handler: &spyHandler{}},
	}})
	responses := []*httptest.ResponseRecorder{
		post(t, h, "/hooks/nosuch/cnx_1", `{}`),
		post(t, h, "/hooks/openwa/cnx_nope", `{}`),
		post(t, h, "/hooks/openwa/cnx_1", `{}`),
	}
	for _, rec := range responses {
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.Equal(t, responses[0].Body.String(), rec.Body.String())
		assert.Equal(t, responses[0].Header(), rec.Header())
	}
}

func TestIngest_OversizeIs413(t *testing.T) {
	res, v, spy := resolver(), &fakeVerifier{}, &spyHandler{}
	h := ingest.NewHandler(ingest.HandlerParams{MaxBodyBytes: 16, Providers: map[string]ingest.Provider{"openwa": {Resolver: res, Verifier: v, Handler: spy}}})
	rec := post(t, h, "/hooks/openwa/cnx_1", strings.Repeat("x", 64))
	require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code, "body=%s", rec.Body.String())
	requireEnvelope(t, rec, "payload_too_large")
	assert.False(t, spy.reached)
	assert.Nil(t, v.seen, "an over-limit body is never handed to a verifier")
	assert.Equal(t, ingest.DefaultMaxBodyBytes, int64(1<<20), "the default cap is 1 MiB")
}

func TestIngest_SecretsFromConnectionNotBody(t *testing.T) {
	res, v, spy := resolver(), &fakeVerifier{}, &spyHandler{}
	h := ingest.NewHandler(ingest.HandlerParams{Providers: map[string]ingest.Provider{"openwa": {Resolver: res, Verifier: v, Handler: spy}}})
	const body = `{"connection":"cnx_other","secret":"whsec_attacker","secrets":["whsec_attacker"]}`
	rec := post(t, h, "/hooks/openwa/cnx_1", body)
	require.Equal(t, http.StatusAccepted, rec.Code, "body=%s", rec.Body.String())
	assert.Equal(t, []string{"cnx_1"}, res.asked, "the connection comes from the path")
	assert.Equal(t, []string{"whsec_from_db"}, v.conn.Secrets, "the verifier keys on the stored secrets, never the body's")
	assert.Equal(t, "row", spy.conn.Value, "the handler gets the resolved connection")
	assert.Equal(t, body, string(v.seen), "the verifier sees the raw body it signs over")
	assert.Equal(t, body, string(spy.body), "the handler reads the body the verifier checked")
}

func TestBasePathIsHonoured(t *testing.T) {
	spy := &spyHandler{}
	h := ingest.NewHandler(ingest.HandlerParams{BasePath: "/app/hooks", Providers: map[string]ingest.Provider{
		"openwa": {Resolver: resolver(), Verifier: &fakeVerifier{}, Handler: spy},
	}})
	require.Equal(t, http.StatusAccepted, post(t, h, "/app/hooks/openwa/cnx_1", "{}").Code)
	require.True(t, spy.reached)
	spy.reached = false
	require.Equal(t, http.StatusUnauthorized, post(t, h, "/app/hooks/nosuch/cnx_1", "{}").Code)
	require.False(t, spy.reached)
}

func TestIngest_OnlyPOSTDelivers(t *testing.T) {
	spy := &spyHandler{}
	h := ingest.NewHandler(ingest.HandlerParams{Providers: map[string]ingest.Provider{"openwa": {Resolver: resolver(), Verifier: &fakeVerifier{}, Handler: spy}}})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/hooks/openwa/cnx_1", nil))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.False(t, spy.reached)
}

func TestVerificationFailureIsLoggedNotReturned(t *testing.T) {
	var logged bytes.Buffer
	h := ingest.NewHandler(ingest.HandlerParams{
		Log: slog.New(slog.NewTextHandler(&logged, nil)),
		Providers: map[string]ingest.Provider{
			"openwa": {Resolver: resolver(), Verifier: &fakeVerifier{err: errors.New("signature digest mismatch")}, Handler: &spyHandler{}},
		},
	})
	rec := post(t, h, "/hooks/openwa/cnx_1", "{}")
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, logged.String(), "signature digest mismatch", "an operator can debug a failing signature")
	assert.Contains(t, logged.String(), "cnx_1")
	assert.NotContains(t, logged.String(), "whsec_from_db", "the secret never reaches the log")
	assert.NotContains(t, rec.Body.String(), "signature digest mismatch")
}

func TestWriteError_RateLimitedCarriesRetryAfter(t *testing.T) {
	rec := httptest.NewRecorder()
	ingest.WriteError(rec, &ingest.RateLimitedError{RetryAfterSeconds: 7})
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Equal(t, "7", rec.Header().Get("Retry-After"))
	assert.JSONEq(t, `{"code":"rate_limited","message":"rate_limited"}`, rec.Body.String())
	assert.True(t, ingest.IsRateLimitedError(&ingest.RateLimitedError{}))
	assert.Equal(t, "ingest: rate limited", (&ingest.RateLimitedError{}).Error())
}

type failingLimiter struct{ ratelimit.Limiter }

func (failingLimiter) Allow(context.Context, string, ratelimit.Rule) (ratelimit.Result, error) {
	return ratelimit.Result{}, errors.New("store down")
}

func postFrom(t *testing.T, h http.Handler, ip, path string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"id":"1"}`))
	r.RemoteAddr = ip + ":40000"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

// SECURITY: the per-IP limit keys on the client IP only, so rotating connection ids never buys a fresh bucket.
func TestIngest_PerIPLimitSharesOneBucketAcrossIDs(t *testing.T) {
	res := resolver()
	rule := ratelimit.Rule{Name: "ingest-ip", Rate: 3, Per: time.Minute, Burst: 3}
	h := ingest.NewHandler(ingest.HandlerParams{
		Providers: map[string]ingest.Provider{"openwa": {Resolver: res, Verifier: &fakeVerifier{err: errors.New("bad")}, Handler: &spyHandler{}}},
		Limiter:   memory.New(100), IPRule: rule,
	})
	for _, id := range []string{"cnx_unknown", "cnx_unsecured", "cnx_disabled"} {
		assert.Equal(t, http.StatusUnauthorized, postFrom(t, h, "203.0.113.7", "/hooks/openwa/"+id).Code, id)
	}
	asked := len(res.asked)
	var limited []*httptest.ResponseRecorder
	for _, id := range []string{"cnx_unknown", "cnx_unsecured", "cnx_disabled", "cnx_1"} {
		rec := postFrom(t, h, "203.0.113.7", "/hooks/openwa/"+id)
		require.Equal(t, http.StatusTooManyRequests, rec.Code, id)
		requireEnvelope(t, rec, "rate_limited")
		assert.NotEmpty(t, rec.Header().Get("Retry-After"))
		limited = append(limited, rec)
	}
	for _, rec := range limited[1:] {
		assert.Equal(t, limited[0].Body.Bytes(), rec.Body.Bytes(), "every id gets the same 429")
	}
	assert.Equal(t, asked, len(res.asked), "a limited delivery is refused before any lookup")
	assert.Equal(t, http.StatusUnauthorized, postFrom(t, h, "198.51.100.9", "/hooks/openwa/cnx_unknown").Code, "another IP has its own bucket")
}

func TestIngest_PerIPLimiterFailureFailsClosed(t *testing.T) {
	spy := &spyHandler{}
	h := ingest.NewHandler(ingest.HandlerParams{
		Providers: map[string]ingest.Provider{"openwa": {Resolver: resolver(), Verifier: &fakeVerifier{}, Handler: spy}},
		Limiter:   failingLimiter{}, IPRule: ratelimit.Rule{Name: "ingest-ip", Rate: 3, Per: time.Minute, Burst: 3},
	})
	rec := postFrom(t, h, "203.0.113.7", "/hooks/openwa/cnx_1")
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.False(t, spy.reached)
}

// SECURITY: behind a trusted proxy every delivery has the proxy's address, so the key is the forwarded client, not RemoteAddr.
func TestIngest_PerIPLimitBehindAProxyKeysOnTheForwardedClient(t *testing.T) {
	rule := ratelimit.Rule{Name: "ingest-ip", Rate: 1, Per: time.Minute, Burst: 1}
	h := ingest.NewHandler(ingest.HandlerParams{
		Providers: map[string]ingest.Provider{"openwa": {Resolver: resolver(), Verifier: &fakeVerifier{err: errors.New("bad")}, Handler: &spyHandler{}}},
		Limiter:   memory.New(100), IPRule: rule, TrustedProxyHops: 1,
	})
	via := func(client string) int {
		r := httptest.NewRequest(http.MethodPost, "/hooks/openwa/cnx_1", strings.NewReader(`{}`))
		r.RemoteAddr = "10.0.0.2:443"
		r.Header.Set("X-Forwarded-For", client)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec.Code
	}
	assert.Equal(t, http.StatusUnauthorized, via("203.0.113.7"))
	assert.Equal(t, http.StatusUnauthorized, via("198.51.100.9"), "a second client behind the same proxy has its own bucket")
	assert.Equal(t, http.StatusTooManyRequests, via("203.0.113.7"), "the first client's bucket is spent")
}

// SECURITY: with no trusted proxy a client can't pick its bucket by forging X-Forwarded-For; the key is RemoteAddr.
func TestIngest_PerIPLimitWithoutAProxyIgnoresAForgedForwardedFor(t *testing.T) {
	rule := ratelimit.Rule{Name: "ingest-ip", Rate: 1, Per: time.Minute, Burst: 1}
	h := ingest.NewHandler(ingest.HandlerParams{
		Providers: map[string]ingest.Provider{"openwa": {Resolver: resolver(), Verifier: &fakeVerifier{err: errors.New("bad")}, Handler: &spyHandler{}}},
		Limiter:   memory.New(100), IPRule: rule,
	})
	forged := func(xff string) int {
		r := httptest.NewRequest(http.MethodPost, "/hooks/openwa/cnx_1", strings.NewReader(`{}`))
		r.RemoteAddr = "203.0.113.7:5000"
		r.Header.Set("X-Forwarded-For", xff)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec.Code
	}
	assert.Equal(t, http.StatusUnauthorized, forged("198.51.100.1"))
	assert.Equal(t, http.StatusTooManyRequests, forged("198.51.100.2"), "a new forged address buys no new bucket")
}
