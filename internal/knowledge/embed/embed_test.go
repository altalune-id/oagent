package embed_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"altalune.id/oagent/internal/knowledge/embed"
)

const secretKey = "sk-secret-0123456789"

type embedRequest struct {
	Model          string   `json:"model"`
	Input          []string `json:"input"`
	EncodingFormat string   `json:"encoding_format"`
}

type fakeAPI struct {
	mu       sync.Mutex
	requests []embedRequest
	auth     []string
	dims     int
	reply    func(n int, req embedRequest) (int, any)
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req embedRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if r.URL.Path != "/v1/embeddings" || r.Method != http.MethodPost || dec.Decode(&req) != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.requests = append(f.requests, req)
	f.auth = append(f.auth, r.Header.Get("Authorization"))
	n := len(f.requests)
	f.mu.Unlock()
	status, body := http.StatusOK, any(f.vectors(req))
	if f.reply != nil {
		status, body = f.reply(n, req)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func (f *fakeAPI) seen() []embedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]embedRequest(nil), f.requests...)
}

func (f *fakeAPI) vectors(req embedRequest) map[string]any {
	data := make([]map[string]any, len(req.Input))
	for i := range req.Input {
		v := make([]float32, f.dims)
		v[0] = float32(len(req.Input[i]))
		data[len(req.Input)-1-i] = map[string]any{"object": "embedding", "index": i, "embedding": v}
	}
	return map[string]any{"object": "list", "data": data, "model": req.Model, "usage": map[string]int{"prompt_tokens": 3 * len(req.Input), "total_tokens": 3 * len(req.Input)}}
}

func serve(t *testing.T, h http.Handler) string {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv.URL + "/v1"
}

func newClient(t *testing.T, cfg embed.Config, retries *[]httpclient.RetryAttempt) *embed.Client {
	t.Helper()
	cfg.AllowPrivateHosts = true
	if cfg.APIKey == "" {
		cfg.APIKey = secretKey
	}
	c, err := embed.NewWithRetry(cfg, httpclient.RetryPolicy{BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond, OnRetry: func(a httpclient.RetryAttempt) {
		if retries != nil {
			*retries = append(*retries, a)
		}
	}})
	require.NoError(t, err)
	return c
}

func texts(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = strings.Repeat("x", i+1)
	}
	return out
}

func requireReason(t *testing.T, err error, want embed.Reason) {
	t.Helper()
	e, ok := embed.AsError(err)
	require.True(t, ok, "got %v", err)
	assert.Equal(t, want, e.Reason, "%v", err)
	assert.NotContains(t, err.Error(), secretKey, "the key never appears in an error")
}

func TestEmbed_GuardedClientAndCRLF(t *testing.T) {
	f := &fakeAPI{dims: 768}
	tls := httptest.NewTLSServer(f)
	t.Cleanup(tls.Close)

	guarded, err := embed.New(embed.Config{BaseURL: tls.URL + "/v1", APIKey: secretKey, Model: "m"})
	require.NoError(t, err)
	_, err = guarded.Embed(t.Context(), []string{"hi"})
	requireReason(t, err, embed.Unreachable)
	assert.True(t, httpclient.IsPrivateAddressError(err), "the SSRF guard refused the loopback dial")
	assert.Empty(t, f.seen(), "nothing reached the server")

	base := tls.URL + "/v1"
	for name, cfg := range map[string]embed.Config{
		"CR in base URL":      {BaseURL: base + "\r", APIKey: secretKey, Model: "m"},
		"LF in base URL":      {BaseURL: base + "\nX-Evil: 1", APIKey: secretKey, Model: "m"},
		"CR in key":           {BaseURL: base, APIKey: secretKey + "\r\nX-Evil: 1", Model: "m"},
		"LF in key":           {BaseURL: base, APIKey: "sk\n" + secretKey, Model: "m"},
		"no base URL":         {APIKey: secretKey, Model: "m"},
		"no model":            {BaseURL: base, APIKey: secretKey},
		"http without opt-in": {BaseURL: "http://llm.example.com/v1", APIKey: secretKey, Model: "m"},
		"not http":            {BaseURL: "ftp://llm.example.com/v1", APIKey: secretKey, Model: "m"},
		"key in the query":    {BaseURL: "https://llm.example.com/v1?key=" + secretKey, APIKey: "k", Model: "m"},
		"fragment":            {BaseURL: "https://llm.example.com/v1#" + secretKey, APIKey: "k", Model: "m"},
		"userinfo":            {BaseURL: "https://u:" + secretKey + "@llm.example.com/v1", APIKey: "k", Model: "m"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := embed.New(cfg)
			requireReason(t, err, embed.Rejected)
		})
	}
	_, err = embed.New(embed.Config{BaseURL: base, APIKey: secretKey, Model: "m", Dims: -1})
	requireReason(t, err, embed.WrongDims)
	assert.Empty(t, f.seen())
}

func TestEmbed_RedirectsAreNeverFollowed(t *testing.T) {
	for _, status := range []int{http.StatusTemporaryRedirect, http.StatusPermanentRedirect, http.StatusFound} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var hits atomic.Int32
			second := serve(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				hits.Add(1)
				w.WriteHeader(http.StatusOK)
			}))
			first := serve(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Location", second+"/embeddings")
				w.WriteHeader(status)
			}))
			_, err := newClient(t, embed.Config{BaseURL: first, Model: "m"}, nil).Embed(t.Context(), []string{"a"})
			requireReason(t, err, embed.Rejected)
			assert.Zero(t, hits.Load(), "the redirect target gets no request, so the key never reaches it")
		})
	}
}

func TestEmbed_SendsTheKeyAndModelAndOrdersByIndex(t *testing.T) {
	f := &fakeAPI{dims: 4}
	c := newClient(t, embed.Config{BaseURL: serve(t, f) + "/", Model: "text-embedding-3-small", Dims: 4}, nil)
	res, err := c.Embed(t.Context(), []string{"a", "bbb"})
	require.NoError(t, err)
	assert.Equal(t, []string{"Bearer " + secretKey}, f.auth)
	assert.Equal(t, []embedRequest{{Model: "text-embedding-3-small", Input: []string{"a", "bbb"}, EncodingFormat: "float"}}, f.seen())
	require.Len(t, res.Vectors, 2)
	assert.InDelta(t, 1, res.Vectors[0][0], 0, "data comes back reversed; vectors follow the input order")
	assert.InDelta(t, 3, res.Vectors[1][0], 0)
	assert.Equal(t, 4, res.Dims)
	assert.Equal(t, int64(6), res.Tokens)
}

func TestEmbed_BatchesOf64(t *testing.T) {
	f := &fakeAPI{dims: 8}
	c := newClient(t, embed.Config{BaseURL: serve(t, f), Model: "m"}, nil)
	res, err := c.Embed(t.Context(), texts(150))
	require.NoError(t, err)
	sizes := []int{}
	for _, r := range f.seen() {
		sizes = append(sizes, len(r.Input))
	}
	assert.Equal(t, []int{embed.BatchSize, embed.BatchSize, 150 - 2*embed.BatchSize}, sizes)
	require.Len(t, res.Vectors, 150)
	for i, v := range res.Vectors {
		assert.InDelta(t, float32(i+1), v[0], 0, "vector %d", i)
	}
	assert.Equal(t, int64(3*150), res.Tokens)

	empty, err := c.Embed(t.Context(), nil)
	require.NoError(t, err)
	assert.Empty(t, empty.Vectors)
	assert.Len(t, f.seen(), 3, "no texts, no request")
}

func TestEmbed_BacksOffOnRetryableStatuses(t *testing.T) {
	for _, status := range []int{http.StatusRequestTimeout, http.StatusTooManyRequests, http.StatusServiceUnavailable, http.StatusBadGateway} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			f := &fakeAPI{dims: 8}
			f.reply = func(n int, req embedRequest) (int, any) {
				if n < 3 {
					return status, map[string]any{"error": map[string]string{"message": "slow down"}}
				}
				return http.StatusOK, f.vectors(req)
			}
			var retries []httpclient.RetryAttempt
			res, err := newClient(t, embed.Config{BaseURL: serve(t, f), Model: "m"}, &retries).Embed(t.Context(), []string{"a"})
			require.NoError(t, err)
			assert.Len(t, res.Vectors, 1)
			assert.Len(t, f.seen(), 3)
			require.Len(t, retries, 2)
			for _, r := range retries {
				assert.Equal(t, status, r.StatusCode)
				assert.Positive(t, r.Wait, "each retry waits")
			}
		})
	}

	for _, status := range []int{http.StatusServiceUnavailable, http.StatusRequestTimeout} {
		var hits atomic.Int32
		f := &fakeAPI{dims: 8, reply: func(int, embedRequest) (int, any) {
			hits.Add(1)
			return status, map[string]any{}
		}}
		_, err := newClient(t, embed.Config{BaseURL: serve(t, f), Model: "m"}, nil).Embed(t.Context(), []string{"a"})
		requireReason(t, err, embed.Unreachable)
		assert.Equal(t, int32(embed.MaxAttempts), hits.Load(), "%d", status)
	}
}

func TestEmbed_BadAnswersAreRejectedWithoutRetry(t *testing.T) {
	vec := []float32{1, 2}
	for name, reply := range map[string]func(int, embedRequest) (int, any){
		"401":          func(int, embedRequest) (int, any) { return http.StatusUnauthorized, map[string]any{} },
		"400":          func(int, embedRequest) (int, any) { return http.StatusBadRequest, map[string]any{} },
		"501":          func(int, embedRequest) (int, any) { return http.StatusNotImplemented, map[string]any{} },
		"not JSON":     func(int, embedRequest) (int, any) { return http.StatusOK, "nope" },
		"missing item": func(int, embedRequest) (int, any) { return http.StatusOK, map[string]any{"data": []any{}} },
		"index out of range": func(int, embedRequest) (int, any) {
			return http.StatusOK, map[string]any{"data": []map[string]any{{"index": 7, "embedding": vec}, {"index": 0, "embedding": vec}}}
		},
		"duplicate index": func(int, embedRequest) (int, any) {
			return http.StatusOK, map[string]any{"data": []map[string]any{{"index": 0, "embedding": vec}, {"index": 0, "embedding": vec}}}
		},
		"null embedding": func(int, embedRequest) (int, any) {
			return http.StatusOK, map[string]any{"data": []map[string]any{{"index": 0, "embedding": nil}, {"index": 1, "embedding": vec}}}
		},
		"empty embedding": func(int, embedRequest) (int, any) {
			return http.StatusOK, map[string]any{"data": []map[string]any{{"index": 0, "embedding": []float32{}}, {"index": 1, "embedding": vec}}}
		},
		"negative tokens": func(int, embedRequest) (int, any) {
			return http.StatusOK, map[string]any{"data": []map[string]any{{"index": 0, "embedding": vec}, {"index": 1, "embedding": vec}}, "usage": map[string]int{"prompt_tokens": -5}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := &fakeAPI{dims: 8, reply: reply}
			_, err := newClient(t, embed.Config{BaseURL: serve(t, f), Model: "m"}, nil).Embed(t.Context(), []string{"a", "b"})
			requireReason(t, err, embed.Rejected)
			assert.Len(t, f.seen(), 1)
		})
	}
}

func TestEmbed_OversizeBodyIsNotRetried(t *testing.T) {
	var hits atomic.Int32
	base := serve(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"index":0,"embedding":[`))
		chunk := []byte(strings.Repeat("0.125,", 1<<16))
		for written := 0; written <= 33<<20; written += len(chunk) {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}))
	_, err := newClient(t, embed.Config{BaseURL: base, Model: "m"}, nil).Embed(t.Context(), []string{"a"})
	requireReason(t, err, embed.Rejected)
	assert.True(t, httpclient.IsBodyTooLargeError(err))
	assert.Equal(t, int32(1), hits.Load())
}

func TestEmbed_CallerCancelReturnsTheContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	f := &fakeAPI{dims: 8}
	_, err := newClient(t, embed.Config{BaseURL: serve(t, f), Model: "m"}, nil).Embed(ctx, []string{"a"})
	require.ErrorIs(t, err, context.Canceled)
	_, isEmbed := embed.AsError(err)
	assert.False(t, isEmbed)

	stalled := serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[`))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	ctx, cancel = context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	_, err = newClient(t, embed.Config{BaseURL: stalled, Model: "m"}, nil).Embed(ctx, []string{"a"})
	require.ErrorIs(t, err, context.DeadlineExceeded, "a cancel during the body read is the context error, not Rejected")
	_, isEmbed = embed.AsError(err)
	assert.False(t, isEmbed)
}

func TestEmbed_WrongLengthFails(t *testing.T) {
	f := &fakeAPI{dims: 1024}
	_, err := newClient(t, embed.Config{BaseURL: serve(t, f), Model: "m", Dims: 1536}, nil).Embed(t.Context(), []string{"a"})
	requireReason(t, err, embed.WrongDims)

	f.reply = func(_ int, req embedRequest) (int, any) {
		body := f.vectors(req)
		body["data"].([]map[string]any)[0]["embedding"] = []float32{1, 2}
		return http.StatusOK, body
	}
	_, err = newClient(t, embed.Config{BaseURL: serve(t, f), Model: "m"}, nil).Embed(t.Context(), []string{"a", "b"})
	requireReason(t, err, embed.WrongDims)

	huge := &fakeAPI{dims: embed.MaxDims + 1}
	_, err = newClient(t, embed.Config{BaseURL: serve(t, huge), Model: "m"}, nil).Embed(t.Context(), []string{"a"})
	requireReason(t, err, embed.WrongDims)

	shrinking := &fakeAPI{dims: 8}
	shrinking.reply = func(n int, req embedRequest) (int, any) {
		if n == 2 {
			shrinking.dims = 4
		}
		return http.StatusOK, shrinking.vectors(req)
	}
	_, err = newClient(t, embed.Config{BaseURL: serve(t, shrinking), Model: "m"}, nil).Embed(t.Context(), texts(embed.BatchSize+1))
	requireReason(t, err, embed.WrongDims)
}

func TestProbe_ReturnsTheLengthWhateverDimsSays(t *testing.T) {
	f := &fakeAPI{dims: 768}
	n, err := newClient(t, embed.Config{BaseURL: serve(t, f), Model: "m", Dims: 1536}, nil).Probe(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 768, n)
	assert.Equal(t, []string{embed.ProbeText}, f.seen()[0].Input)
}

func TestError_KeepsTheCause(t *testing.T) {
	cause := errors.New("boom")
	err := error(&embed.Error{Reason: embed.Unreachable, Cause: cause})
	require.ErrorIs(t, err, cause)
	assert.Equal(t, "embed: unreachable: boom", err.Error())
	assert.Equal(t, "embed: rejected", (&embed.Error{Reason: embed.Rejected}).Error())
}

func TestEmbed_BilledTokensReturnedWithTheError(t *testing.T) {
	f := &fakeAPI{dims: 8}
	f.reply = func(n int, req embedRequest) (int, any) {
		body := f.vectors(req)
		if n == 2 {
			body["data"].([]map[string]any)[0]["embedding"] = []float32{1, 2}
		}
		return http.StatusOK, body
	}
	res, err := newClient(t, embed.Config{BaseURL: serve(t, f), Model: "m"}, nil).Embed(t.Context(), texts(embed.BatchSize+2))
	requireReason(t, err, embed.WrongDims)
	assert.Equal(t, int64(3*(embed.BatchSize+2)), res.Tokens, "both batches were billed, the failing one too")
	assert.Empty(t, res.Vectors)

	bad := &fakeAPI{dims: 8, reply: func(_ int, req embedRequest) (int, any) {
		return http.StatusOK, map[string]any{"data": []map[string]any{{"index": 0, "embedding": nil}}, "usage": map[string]int{"prompt_tokens": 9}}
	}}
	res, err = newClient(t, embed.Config{BaseURL: serve(t, bad), Model: "m"}, nil).Embed(t.Context(), []string{"a"})
	requireReason(t, err, embed.Rejected)
	assert.Equal(t, int64(9), res.Tokens)
}
