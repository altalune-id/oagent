package transcribe_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/httpclient"
	"altalune.id/oagent/internal/run/transcribe"
)

const secretKey = "sk-secret-0123456789"

var clip = []byte("OggS fake opus bytes")

type upload struct {
	auth, model, format, filename, fileType string
	file                                    []byte
}

type fakeAPI struct {
	mu      sync.Mutex
	uploads []upload
	reply   func(n int) (int, string)
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v1/audio/transcriptions" || r.Method != http.MethodPost || r.ParseMultipartForm(1<<20) != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	u := upload{auth: r.Header.Get("Authorization"), model: r.FormValue("model"), format: r.FormValue("response_format")}
	if file, hdr, err := r.FormFile("file"); err == nil {
		u.filename, u.fileType = hdr.Filename, hdr.Header.Get("Content-Type")
		u.file, _ = io.ReadAll(file)
	}
	f.mu.Lock()
	f.uploads = append(f.uploads, u)
	n := len(f.uploads)
	f.mu.Unlock()
	status, body := http.StatusOK, `{"text":"hello there"}`
	if f.reply != nil {
		status, body = f.reply(n)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func (f *fakeAPI) seen() []upload {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]upload(nil), f.uploads...)
}

func serve(t *testing.T, h http.Handler) string {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv.URL + "/v1"
}

func newClient(t *testing.T, base string) *transcribe.Client {
	t.Helper()
	c, err := transcribe.New(transcribe.Config{BaseURL: base, APIKey: secretKey, Model: "whisper-large-v3-turbo", AllowPrivateHosts: true})
	require.NoError(t, err)
	return c
}

func requireReason(t *testing.T, err error, want transcribe.Reason) {
	t.Helper()
	e, ok := transcribe.AsError(err)
	require.True(t, ok, "got %v", err)
	assert.Equal(t, want, e.Reason, "%v", err)
	assert.NotContains(t, err.Error(), secretKey, "the key never appears in an error")
}

func TestTranscribe_SendsTheFieldsAndKey(t *testing.T) {
	f := &fakeAPI{}
	c := newClient(t, serve(t, f)+"/")
	defer c.Close()
	text, err := c.Transcribe(t.Context(), clip, "audio/ogg")
	require.NoError(t, err)
	assert.Equal(t, "hello there", text)
	assert.Equal(t, []upload{{
		auth: "Bearer " + secretKey, model: "whisper-large-v3-turbo", format: "json",
		filename: "voice.ogg", fileType: "audio/ogg", file: clip,
	}}, f.seen())
}

func TestTranscribe_EmptyTextIsReturned(t *testing.T) {
	f := &fakeAPI{reply: func(int) (int, string) { return http.StatusOK, `{"text":""}` }}
	text, err := newClient(t, serve(t, f)).Transcribe(t.Context(), clip, "audio/ogg")
	require.NoError(t, err)
	assert.Empty(t, text)
}

func TestNew_GuardedClientAndCRLF(t *testing.T) {
	f := &fakeAPI{}
	tls := httptest.NewTLSServer(f)
	t.Cleanup(tls.Close)

	guarded, err := transcribe.New(transcribe.Config{BaseURL: tls.URL + "/v1", APIKey: secretKey, Model: "m"})
	require.NoError(t, err)
	_, err = guarded.Transcribe(t.Context(), clip, "audio/ogg")
	requireReason(t, err, transcribe.Unreachable)
	assert.True(t, httpclient.IsPrivateAddressError(err), "the SSRF guard refused the loopback dial")
	assert.Empty(t, f.seen(), "nothing reached the server")

	base := tls.URL + "/v1"
	for name, cfg := range map[string]transcribe.Config{
		"CR in base URL":      {BaseURL: base + "\r", APIKey: secretKey, Model: "m"},
		"LF in key":           {BaseURL: base, APIKey: "sk\n" + secretKey, Model: "m"},
		"LF in model":         {BaseURL: base, APIKey: secretKey, Model: "m\nX-Evil: 1"},
		"no base URL":         {APIKey: secretKey, Model: "m"},
		"no model":            {BaseURL: base, APIKey: secretKey},
		"http without opt-in": {BaseURL: "http://stt.example.com/v1", APIKey: secretKey, Model: "m"},
		"not http":            {BaseURL: "ftp://stt.example.com/v1", APIKey: secretKey, Model: "m"},
		"key in the query":    {BaseURL: "https://stt.example.com/v1?key=" + secretKey, APIKey: "k", Model: "m"},
		"fragment":            {BaseURL: "https://stt.example.com/v1#" + secretKey, APIKey: "k", Model: "m"},
		"userinfo":            {BaseURL: "https://u:" + secretKey + "@stt.example.com/v1", APIKey: "k", Model: "m"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := transcribe.New(cfg)
			requireReason(t, err, transcribe.Rejected)
		})
	}
	_, err = transcribe.New(transcribe.Config{BaseURL: "http://127.0.0.1/v1", APIKey: secretKey, Model: "m", AllowPrivateHosts: true})
	require.NoError(t, err, "http is allowed only with private hosts")
}

func TestTranscribe_BadAudioTypeIsRejected(t *testing.T) {
	f := &fakeAPI{}
	c := newClient(t, serve(t, f))
	for _, mime := range []string{"", "audio/ogg\r\nX-Evil: 1"} {
		_, err := c.Transcribe(t.Context(), clip, mime)
		requireReason(t, err, transcribe.Rejected)
	}
	assert.Empty(t, f.seen())
}

func TestTranscribe_RedirectsAreNeverFollowed(t *testing.T) {
	for _, status := range []int{http.StatusTemporaryRedirect, http.StatusPermanentRedirect, http.StatusFound} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var hits atomic.Int32
			second := serve(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				hits.Add(1)
				w.WriteHeader(http.StatusOK)
			}))
			first := serve(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Location", second+"/audio/transcriptions")
				w.WriteHeader(status)
			}))
			_, err := newClient(t, first).Transcribe(t.Context(), clip, "audio/ogg")
			requireReason(t, err, transcribe.Rejected)
			assert.Zero(t, hits.Load(), "the redirect target gets no request, so the key and audio never reach it")
		})
	}
}

func TestTranscribe_NeverRetries(t *testing.T) {
	for status, want := range map[int]transcribe.Reason{
		http.StatusTooManyRequests:     transcribe.RateLimited,
		http.StatusInternalServerError: transcribe.Unreachable,
		http.StatusRequestTimeout:      transcribe.Unreachable,
	} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			f := &fakeAPI{reply: func(int) (int, string) { return status, `{}` }}
			_, err := newClient(t, serve(t, f)).Transcribe(t.Context(), clip, "audio/ogg")
			requireReason(t, err, want)
			assert.Len(t, f.seen(), 1, "a replayed upload may be billed twice, so there is no second request")
		})
	}
}

func TestTranscribe_TransportErrorIsNotRetried(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
	}))
	t.Cleanup(srv.Close)
	_, err := newClient(t, srv.URL+"/v1").Transcribe(t.Context(), clip, "audio/ogg")
	requireReason(t, err, transcribe.Unreachable)
	assert.Equal(t, int32(1), hits.Load())
}

func TestTranscribe_SlowHeadersWithinTheCallLimit(t *testing.T) {
	t.Parallel()
	f := &fakeAPI{reply: func(int) (int, string) {
		time.Sleep(11 * time.Second)
		return http.StatusOK, `{"text":"slow but fine"}`
	}}
	text, err := newClient(t, serve(t, f)).Transcribe(t.Context(), clip, "audio/ogg")
	require.NoError(t, err, "headers after 11 s are within the 20 s call limit, past the transport's 10 s default")
	assert.Equal(t, "slow but fine", text)
}

func TestTranscribe_ClientErrorsAreNotRetried(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusRequestEntityTooLarge, http.StatusUnsupportedMediaType, 498, http.StatusNotImplemented} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			f := &fakeAPI{reply: func(int) (int, string) { return status, `{}` }}
			_, err := newClient(t, serve(t, f)).Transcribe(t.Context(), clip, "audio/ogg")
			requireReason(t, err, transcribe.Rejected)
			assert.Len(t, f.seen(), 1)
		})
	}
}

func TestTranscribe_NotJSONIsRejected(t *testing.T) {
	f := &fakeAPI{reply: func(int) (int, string) { return http.StatusOK, "hello there" }}
	_, err := newClient(t, serve(t, f)).Transcribe(t.Context(), clip, "audio/ogg")
	requireReason(t, err, transcribe.Rejected)
}

func TestTranscribe_OversizeBodyIsNotRetried(t *testing.T) {
	var hits atomic.Int32
	base := serve(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"`))
		chunk := []byte(strings.Repeat("a", 1<<16))
		for written := 0; written <= 2<<20; written += len(chunk) {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}))
	_, err := newClient(t, base).Transcribe(t.Context(), clip, "audio/ogg")
	requireReason(t, err, transcribe.Rejected)
	assert.True(t, httpclient.IsBodyTooLargeError(err))
	assert.Equal(t, int32(1), hits.Load())
}

func TestTranscribe_CallerCancelReturnsTheContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	f := &fakeAPI{}
	_, err := newClient(t, serve(t, f)).Transcribe(ctx, clip, "audio/ogg")
	require.ErrorIs(t, err, context.Canceled)
	_, isTranscribe := transcribe.AsError(err)
	assert.False(t, isTranscribe)

	stalled := serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":`))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	ctx, cancel = context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	_, err = newClient(t, stalled).Transcribe(ctx, clip, "audio/ogg")
	require.ErrorIs(t, err, context.DeadlineExceeded, "a cancel during the body read is the context error, not Rejected")
}

func TestError_KeepsTheCause(t *testing.T) {
	cause := fmt.Errorf("boom")
	err := &transcribe.Error{Reason: transcribe.Unreachable, Cause: cause}
	require.ErrorIs(t, err, cause)
	assert.Equal(t, "transcribe: unreachable: boom", err.Error())
	assert.Equal(t, "transcribe: rejected", (&transcribe.Error{Reason: transcribe.Rejected}).Error())
}
