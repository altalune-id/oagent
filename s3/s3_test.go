package s3

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testKeyID  = "GK000000000000000000000001"
	testSecret = "hunter2-s3-secret-access-key"
	testBucket = "altalune-files"
)

func testConfig(endpoint string) Config {
	return Config{
		Endpoint: endpoint, Region: "garage", Bucket: testBucket,
		AccessKeyID: testKeyID, SecretAccessKey: testSecret, UsePathStyle: true, Timeout: time.Second,
	}
}

func discard() *slog.Logger { return slog.New(slog.DiscardHandler) }

type fakeStore struct {
	status atomic.Int32
	heads  atomic.Int32
	auth   atomic.Value
}

func newFakeStore(t *testing.T) (*fakeStore, *httptest.Server) {
	t.Helper()
	f := &fakeStore{}
	f.status.Store(http.StatusOK)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead && r.URL.Path == "/"+testBucket {
			f.heads.Add(1)
			f.auth.Store(r.Header.Get("Authorization"))
		}
		w.WriteHeader(int(f.status.Load()))
	}))
	t.Cleanup(srv.Close)
	return f, srv
}

func TestOpen_ChecksTheBucketAndSignsBrowserURLsForTheBrowserEndpoint(t *testing.T) {
	f, srv := newFakeStore(t)
	cfg := testConfig(srv.URL + "/")
	cfg.BrowserEndpoint = "http://localhost:3900"

	c, err := Open(t.Context(), cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	assert.Equal(t, testBucket, c.Bucket)
	assert.EqualValues(t, 1, f.heads.Load())
	assert.Contains(t, f.auth.Load(), "Credential="+testKeyID+"/", "server calls are signed with the key")
	assert.Contains(t, f.auth.Load(), "/garage/s3/aws4_request", "with the configured region")

	req, err := c.Presign.PresignPutObject(t.Context(), &awss3.PutObjectInput{
		Bucket: aws.String(c.Bucket), Key: aws.String("public/oagent/projects/p1/logo/f1.png"),
		ContentType: aws.String("image/png"), ContentLength: aws.Int64(100),
	}, awss3.WithPresignExpires(10*time.Minute))
	require.NoError(t, err)
	u, err := url.Parse(req.URL)
	require.NoError(t, err)
	assert.Equal(t, "localhost:3900", u.Host)
	assert.Equal(t, "/"+testBucket+"/public/oagent/projects/p1/logo/f1.png", u.Path)
	assert.Equal(t, "content-length;content-type;host", u.Query().Get("X-Amz-SignedHeaders"))
	assert.NotContains(t, req.URL, testSecret)
	assert.NotContains(t, req.URL, "x-amz-checksum", "no checksum the browser cannot send")
}

func TestOpen_WithoutABrowserEndpointSignsForTheEndpoint(t *testing.T) {
	_, srv := newFakeStore(t)
	c, err := Open(t.Context(), testConfig(srv.URL))
	require.NoError(t, err)
	req, err := c.Presign.PresignGetObject(t.Context(), &awss3.GetObjectInput{Bucket: aws.String(c.Bucket), Key: aws.String("k")})
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(req.URL, srv.URL+"/"+testBucket+"/k?"), req.URL)
	require.NoError(t, c.Close())
	require.NoError(t, c.Close(), "Close is idempotent")
}

func TestOpen_RefusedBucketFailsWithoutTheSecret(t *testing.T) {
	f, srv := newFakeStore(t)
	f.status.Store(http.StatusForbidden)
	_, err := Open(t.Context(), testConfig(srv.URL))
	require.Error(t, err)
	host := strings.TrimPrefix(srv.URL, "http://")
	assert.ErrorContains(t, err, `s3: cannot reach bucket "altalune-files" at `+host+" (s3.endpoint)")
	assert.NotContains(t, err.Error(), testSecret)
}

func TestOpen_SilentServerFailsWithinTimeout(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	done := make(chan struct{})
	t.Cleanup(func() {
		close(done)
		_ = ln.Close()
	})
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				<-done
				_ = conn.Close()
			}()
		}
	}()
	cfg := testConfig("http://" + ln.Addr().String())
	cfg.Timeout = 300 * time.Millisecond
	start := time.Now()
	c, err := Open(t.Context(), cfg)
	require.Error(t, err)
	assert.Nil(t, c)
	assert.Less(t, time.Since(start), 3*time.Second, "bounded by s3.timeout")
	assert.ErrorContains(t, err, "s3: cannot reach bucket")
}

func TestOpen_BadEndpointsAreNotEchoed(t *testing.T) {
	for name, tc := range map[string]struct{ endpoint, browser, key string }{
		"no scheme":            {endpoint: "garage:3900", key: "s3.endpoint"},
		"other scheme":         {endpoint: "ftp://garage:3900", key: "s3.endpoint"},
		"no host":              {endpoint: "http://", key: "s3.endpoint"},
		"unparsable":           {endpoint: "http://%zz", key: "s3.endpoint"},
		"credentials in url":   {endpoint: "http://key:" + testSecret + "@garage:3900", key: "s3.endpoint"},
		"query":                {endpoint: "http://garage:3900/?token=" + testSecret, key: "s3.endpoint"},
		"bad browser endpoint": {endpoint: "http://garage:3900", browser: "localhost:3900/" + testSecret, key: "s3.browserEndpoint"},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := testConfig(tc.endpoint)
			cfg.BrowserEndpoint = tc.browser
			_, err := Open(t.Context(), cfg)
			require.Error(t, err)
			assert.ErrorContains(t, err, tc.key)
			assert.NotContains(t, err.Error(), testSecret)
			assert.NotContains(t, err.Error(), "garage:3900")
		})
	}
}

func TestHealth_FlipsAndLogsOncePerChange(t *testing.T) {
	f, srv := newFakeStore(t)
	c, err := Open(t.Context(), testConfig(srv.URL))
	require.NoError(t, err)
	var logs bytes.Buffer
	h := NewHealth(c, time.Second, slog.New(slog.NewTextHandler(&logs, nil)))
	assert.Equal(t, "s3-health", h.Name())
	require.True(t, h.Ready())

	f.status.Store(http.StatusServiceUnavailable)
	h.probe(t.Context())
	h.probe(t.Context())
	assert.False(t, h.Ready())
	assert.Equal(t, 1, strings.Count(logs.String(), "s3: not ready"), "logged once per change, not per probe")

	f.status.Store(http.StatusOK)
	h.probe(t.Context())
	assert.True(t, h.Ready())
	assert.Contains(t, logs.String(), "s3: ready again")
}

func TestHealth_RunStopsWithItsContextAndIgnoresACancelledProbe(t *testing.T) {
	f, srv := newFakeStore(t)
	c, err := Open(t.Context(), testConfig(srv.URL))
	require.NoError(t, err)
	h := NewHealth(c, time.Second, discard())
	f.status.Store(http.StatusServiceUnavailable)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	assert.NoError(t, h.Run(ctx))
	assert.True(t, h.Ready(), "a probe cut short by shutdown does not mark the bucket down")
}
