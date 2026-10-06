package httpclient

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_Options(t *testing.T) {
	assert.Equal(t, DefaultTimeout, New(WithOtel(false)).Timeout)
	assert.Equal(t, time.Duration(0), New(WithoutTimeout(), WithOtel(false)).Timeout)
	assert.Equal(t, 5*time.Second, New(WithTimeout(5*time.Second), WithOtel(false)).Timeout)
	assert.Equal(t, DefaultTimeout, New(WithTimeout(0), WithOtel(false)).Timeout)
	assert.NotNil(t, New(WithAllowPrivateHosts(true), WithDialTimeout(time.Second), WithOtel(false)).Transport)
}

func TestNew_RejectsPrivateByDefault(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	t.Cleanup(ts.Close)

	resp, err := New(WithOtel(false)).Get(ts.URL)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.Error(t, err)
	assert.True(t, IsPrivateAddressError(err))
}

func TestNew_AllowsPrivateWhenOptedIn(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	t.Cleanup(ts.Close)

	resp, err := New(WithAllowPrivateHosts(true), WithOtel(false)).Get(ts.URL)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestNew_ResponseBodyLimit(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("0123456789ABCDEF"))
	}))
	t.Cleanup(ts.Close)

	resp, err := New(WithAllowPrivateHosts(true), WithOtel(false), WithResponseBodyLimit(4)).Get(ts.URL)
	require.NoError(t, err)
	_, rerr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	require.Error(t, rerr)
	assert.True(t, IsBodyTooLargeError(rerr))
}

func TestNew_Retry(t *testing.T) {
	var attempts, retries int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(ts.Close)

	c := New(WithAllowPrivateHosts(true), WithOtel(false), WithRetry(RetryPolicy{
		MaxAttempts: 4,
		BaseDelay:   time.Millisecond,
		MaxDelay:    5 * time.Millisecond,
		OnRetry:     func(RetryAttempt) { retries++ },
	}))
	resp, err := c.Get(ts.URL)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()
	assert.GreaterOrEqual(t, retries, 2)
}

func TestNew_OtelDefault(t *testing.T) {
	assert.NotNil(t, New().Transport)
}

func TestNew_RetryRespectsRetryAfter(t *testing.T) {
	var attempts int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(ts.Close)

	c := New(WithAllowPrivateHosts(true), WithOtel(false), WithRetry(RetryPolicy{
		MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond,
	}))
	resp, err := c.Get(ts.URL)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()
	assert.Equal(t, 2, attempts)
}

func TestNew_NonIdempotentNotRetried(t *testing.T) {
	var attempts int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(ts.Close)

	c := New(WithAllowPrivateHosts(true), WithOtel(false), WithRetry(RetryPolicy{
		MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 2 * time.Millisecond,
	}))
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL, strings.NewReader("x"))
	require.NoError(t, err)
	resp, err := c.Do(req)

	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, 1, attempts) // POST is not replayed without RetryNonIdempotent
}

func TestNew_RetriesTransportError(t *testing.T) {
	c := New(WithAllowPrivateHosts(true), WithOtel(false), WithRetry(RetryPolicy{
		MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: 2 * time.Millisecond,
	}))
	resp, err := c.Get("http://127.0.0.1:1")
	if resp != nil {
		_ = resp.Body.Close()
	}
	assert.Error(t, err)
}

func TestRetryPolicy(t *testing.T) {
	assert.False(t, RetryPolicy{MaxAttempts: 1}.enabled())
	assert.True(t, RetryPolicy{MaxAttempts: 2}.enabled())

	p := RetryPolicy{MaxAttempts: 3}.withDefaults()
	assert.Equal(t, DefaultRetryBaseDelay, p.BaseDelay)
	assert.Equal(t, DefaultRetryMaxDelay, p.MaxDelay)

	p = RetryPolicy{MaxAttempts: 3, BaseDelay: 10 * time.Millisecond, MaxDelay: time.Second}
	assert.Equal(t, 10*time.Millisecond, p.clamp(time.Millisecond))
	assert.Equal(t, time.Second, p.clamp(time.Hour))
	assert.Greater(t, p.waitFor(0, 0), time.Duration(0))
	assert.Equal(t, p.clamp(50*time.Millisecond), p.waitFor(0, 50*time.Millisecond))
	assert.Greater(t, p.worstCaseWaitFor(0, 0), time.Duration(0))
	assert.Equal(t, p.clamp(50*time.Millisecond), p.worstCaseWaitFor(0, 50*time.Millisecond))
}

func TestParseRetryAfter(t *testing.T) {
	assert.Equal(t, time.Duration(0), ParseRetryAfter(""))
	assert.Equal(t, 5*time.Second, ParseRetryAfter("5"))
	assert.Equal(t, time.Duration(0), ParseRetryAfter("bogus"))
	assert.Greater(t, ParseRetryAfter(time.Now().Add(time.Minute).UTC().Format(http.TimeFormat)), time.Duration(0))
}

func TestIsPrivateOrLocal(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1":            true,
		"10.0.0.1":             true,
		"192.168.1.1":          true,
		"::1":                  true,
		"0.0.0.0":              true,
		"169.254.1.1":          true,
		"224.0.0.1":            true,
		"192.0.2.1":            true,
		"8.8.8.8":              false,
		"1.1.1.1":              false,
		"2606:4700:4700::1111": false,
	}
	for s, want := range cases {
		assert.Equal(t, want, IsPrivateOrLocal(netip.MustParseAddr(s)), s)
	}
	assert.True(t, IsPrivateOrLocal(netip.Addr{}))
}

func TestRejectPrivate(t *testing.T) {
	assert.Error(t, rejectPrivate(context.Background(), "tcp", "127.0.0.1:80", nil))
	assert.NoError(t, rejectPrivate(context.Background(), "tcp", "8.8.8.8:80", nil))
	assert.Error(t, rejectPrivate(context.Background(), "tcp", "not-an-addr", nil))
}

func TestInternals(t *testing.T) {
	drain(nil)
	drain(&http.Response{})
	require.NoError(t, sleep(context.Background(), time.Millisecond))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.Error(t, sleep(ctx, time.Second))

	assert.True(t, affordable(context.Background(), time.Hour))
	dctx, dcancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer dcancel()
	assert.False(t, affordable(dctx, time.Hour))

	assert.True(t, permanentTransportError(&net.DNSError{IsNotFound: true}))
	assert.False(t, retryableResult(nil, context.Canceled))
	assert.False(t, retryableResult(nil, nil))
	assert.True(t, methodReplayable(RetryPolicy{RetryNonIdempotent: true}, http.MethodPost))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.GetBody = func() (io.ReadCloser, error) { return nil, assert.AnError }
	_, err := rewind(req)
	assert.Error(t, err)
}

func TestErrorTypes(t *testing.T) {
	assert.True(t, IsPrivateAddressError(&PrivateAddressError{Address: "x"}))
	assert.False(t, IsPrivateAddressError(nil))
	assert.Contains(t, (&PrivateAddressError{Address: "x"}).Error(), "no usable IP")
	assert.Contains(t, (&PrivateAddressError{Address: "x", Addr: netip.MustParseAddr("10.0.0.1")}).Error(), "private")

	assert.True(t, IsBodyTooLargeError(&BodyTooLargeError{Limit: 1}))
	assert.False(t, IsBodyTooLargeError(nil))
	assert.Contains(t, (&BodyTooLargeError{Limit: 1}).Error(), "exceeds")

	assert.True(t, IsUnhealthyStatusError(&UnhealthyStatusError{StatusCode: 500}))
	assert.False(t, IsUnhealthyStatusError(nil))
	assert.Contains(t, (&UnhealthyStatusError{StatusCode: 500}).Error(), "unhealthy")
}
