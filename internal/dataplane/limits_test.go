package dataplane_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/dataplane"
	"altalune.id/oagent/internal/kernel/ratelimit"
	"altalune.id/oagent/internal/kernel/ratelimit/memory"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/testutil/fakes"
)

func limited(l dataplane.Limits) func(*dataplane.Deps) {
	return func(d *dataplane.Deps) { d.Limits = l }
}

func runsPath() string { return "/api/v1/agents/AGENTpub/runs/nope" }

func TestDataPlane_PerKeyLimit(t *testing.T) {
	p := newPlane(t, 4, limited(dataplane.Limits{Limiter: memory.New(1000), PerKey: dataplane.KeyRule(2)}))
	for range 2 {
		assert.Equal(t, http.StatusNotFound, p.call(t, http.MethodGet, projectKey, runsPath()).StatusCode)
	}
	over := p.call(t, http.MethodGet, projectKey, runsPath())
	require.Equal(t, http.StatusTooManyRequests, over.StatusCode)
	assert.NotEmpty(t, over.Header.Get("Retry-After"))
	code, _ := errorCode(t, over)
	assert.Equal(t, apperror.CodeRunRateLimited, code)
	assert.Equal(t, http.StatusNotFound, p.call(t, http.MethodGet, orgKey, runsPath()).StatusCode, "another key has its own bucket")
}

func TestDataPlane_PerIPLimit(t *testing.T) {
	p := newPlane(t, 4, limited(dataplane.Limits{Limiter: memory.New(1000), PerIP: dataplane.IPRule(2), TrustedProxyHops: 1}))
	from := func(ip string) []string { return []string{"X-Forwarded-For", ip} }
	assert.Equal(t, http.StatusUnauthorized, p.call(t, http.MethodGet, "oak_wrong", runsPath(), from("203.0.113.7")...).StatusCode)
	assert.Equal(t, http.StatusNotFound, p.call(t, http.MethodGet, projectKey, runsPath(), from("203.0.113.7")...).StatusCode)
	over := p.call(t, http.MethodGet, orgKey, runsPath(), from("203.0.113.7")...)
	require.Equal(t, http.StatusTooManyRequests, over.StatusCode, "every key from one address shares its bucket, bad keys included")
	assert.NotEmpty(t, over.Header.Get("Retry-After"))
	assert.Equal(t, http.StatusNotFound, p.call(t, http.MethodGet, orgKey, runsPath(), from("198.51.100.9")...).StatusCode)
}

type brokenLimiter struct{}

func (brokenLimiter) Allow(context.Context, string, ratelimit.Rule) (ratelimit.Result, error) {
	return ratelimit.Result{}, errors.New("store down")
}

func (brokenLimiter) Reset(context.Context, string) error { return nil }

func TestDataPlane_BrokenLimiterFailsClosed(t *testing.T) {
	p := newPlane(t, 4, limited(dataplane.Limits{Limiter: brokenLimiter{}, PerIP: dataplane.IPRule(10)}),
		func(d *dataplane.Deps) { d.Unexpected = apperror.NewReporter(nil, true).Unexpected })
	resp := p.call(t, http.MethodGet, projectKey, runsPath())
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	assert.NotContains(t, resp.Body, "store down")
}

func TestDataPlane_ProjectRateIs429(t *testing.T) {
	p := newPlane(t, 4, func(d *dataplane.Deps) { d.Rate = run.RateCap{Limiter: memory.New(1000), PerMinute: 1} })
	p.settledTurn(t, projectKey)
	over := p.post(t, projectKey, "AGENTpub", body("client-thread-2", "again"))
	require.Equal(t, http.StatusTooManyRequests, over.StatusCode)
	assert.NotEmpty(t, over.Header.Get("Retry-After"))
	code, _ := errorCode(t, over)
	assert.Equal(t, apperror.CodeRunRateLimited, code)
	assert.Len(t, p.store.Messages, 2, "a refused POST inserts nothing")
}

func TestDataPlane_OnePOSTChargesOnce(t *testing.T) {
	p := newPlane(t, 4, func(d *dataplane.Deps) { d.Rate = run.RateCap{Limiter: memory.New(1000), PerMinute: 2} })
	p.rt.Replies = []fakes.RuntimeReply{{Stream: runtimeStream(t, "plain_reply")}, {Stream: runtimeStream(t, "plain_reply")}}
	for _, thread := range []string{"client-thread-1", "client-thread-2"} {
		resp := p.post(t, projectKey, "AGENTpub", body(thread, "hello"))
		require.Equal(t, http.StatusOK, resp.StatusCode, resp.Body)
		assert.Contains(t, resp.Body, `"type":"RUN_FINISHED"`, "the held run is not charged a second time")
	}
	assert.Equal(t, http.StatusTooManyRequests, p.post(t, projectKey, "AGENTpub", body("client-thread-3", "hello")).StatusCode)
}
