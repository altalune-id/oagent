// Package embed calls an OpenAI-compatible embeddings endpoint through the SSRF-guarded HTTP client.
package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"altalune.id/oagent/httpclient"
)

// The call limits.
const (
	BatchSize        = 64
	Timeout          = 30 * time.Second
	MaxAttempts      = 4
	MaxDims          = 16000
	maxResponseBytes = 32 << 20
)

// ProbeText is the fixed string Probe embeds to learn a model's vector length.
const ProbeText = "oagent embedding probe"

// Config names the endpoint and model; Dims is the expected vector length, or 0 to take the first one seen.
type Config struct {
	BaseURL           string
	APIKey            string
	Model             string
	Dims              int
	AllowPrivateHosts bool
}

// Client embeds texts with one model.
type Client struct {
	cfg      Config
	endpoint string
	http     *http.Client
}

// Result holds the vectors in input order, the tokens the provider counted and the vector length.
type Result struct {
	Vectors [][]float32
	Tokens  int64
	Dims    int
}

// New checks cfg and returns a client on the guarded HTTP client, retrying 429 and 5xx with backoff.
func New(cfg Config) (*Client, error) {
	return newClient(cfg, httpclient.RetryPolicy{})
}

func newClient(cfg Config, retry httpclient.RetryPolicy) (*Client, error) {
	if strings.ContainsAny(cfg.BaseURL, "\r\n") || strings.ContainsAny(cfg.APIKey, "\r\n") {
		return nil, &Error{Reason: Rejected, Cause: errors.New("base URL or key holds a line break")}
	}
	if cfg.BaseURL == "" || cfg.Model == "" {
		return nil, &Error{Reason: Rejected, Cause: errors.New("base URL and model are required")}
	}
	if cfg.Dims < 0 {
		return nil, &Error{Reason: WrongDims, Cause: errors.New("negative dims")}
	}
	u, err := url.Parse(cfg.BaseURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, &Error{Reason: Rejected, Cause: errors.New("base URL must be a plain URL with a host and no credentials, query or fragment")}
	}
	if u.Scheme != "https" && (u.Scheme != "http" || !cfg.AllowPrivateHosts) {
		return nil, &Error{Reason: Rejected, Cause: errors.New("base URL must be https")}
	}
	retry.MaxAttempts = MaxAttempts
	// NOTE: an embeddings call has no side effect, so replaying the POST is safe.
	retry.RetryNonIdempotent = true
	client := httpclient.New(
		httpclient.WithAllowPrivateHosts(cfg.AllowPrivateHosts),
		httpclient.WithoutProxy(),
		httpclient.WithoutRedirects(),
		httpclient.WithTimeout(Timeout),
		httpclient.WithResponseBodyLimit(maxResponseBytes),
		httpclient.WithRetry(retry),
	)
	return &Client{cfg: cfg, endpoint: u.JoinPath("embeddings").String(), http: client}, nil
}

// Probe embeds ProbeText and returns the model's vector length, whatever Config.Dims says.
func (c *Client) Probe(ctx context.Context) (int, error) {
	probe := *c
	probe.cfg.Dims = 0
	res, err := probe.Embed(ctx, []string{ProbeText})
	return res.Dims, err
}

// Embed embeds texts in batches of BatchSize, all vectors one length (Config.Dims when set); on error Result holds only the billed Tokens.
func (c *Client) Embed(ctx context.Context, texts []string) (Result, error) {
	res := Result{Vectors: make([][]float32, 0, len(texts)), Dims: c.cfg.Dims}
	for batch := range slices.Chunk(texts, BatchSize) {
		vectors, tokens, err := c.call(ctx, batch)
		if err != nil {
			return Result{Tokens: res.Tokens + tokens}, err
		}
		for _, v := range vectors {
			if res.Dims == 0 {
				res.Dims = len(v)
			}
			if len(v) > MaxDims || len(v) != res.Dims {
				return Result{Tokens: res.Tokens + tokens}, &Error{Reason: WrongDims, Cause: fmt.Errorf("vector of %d, want %d", len(v), res.Dims)}
			}
		}
		res.Vectors = append(res.Vectors, vectors...)
		res.Tokens += tokens
	}
	return res, nil
}

type request struct {
	Model          string   `json:"model"`
	Input          []string `json:"input"`
	EncodingFormat string   `json:"encoding_format"`
}

type response struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Usage struct {
		PromptTokens int64 `json:"prompt_tokens"`
	} `json:"usage"`
}

func (c *Client) call(ctx context.Context, batch []string) (vectors [][]float32, tokens int64, err error) {
	body, err := json.Marshal(request{Model: c.cfg.Model, Input: batch, EncodingFormat: "float"})
	if err != nil {
		return nil, 0, &Error{Reason: Rejected, Cause: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, 0, &Error{Reason: Rejected, Cause: err}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, 0, ctx.Err()
		}
		return nil, 0, &Error{Reason: Unreachable, Cause: err}
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case httpclient.RetryableStatus(resp.StatusCode):
		return nil, 0, &Error{Reason: Unreachable, Cause: fmt.Errorf("provider answered %d", resp.StatusCode)}
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		return nil, 0, &Error{Reason: Rejected, Cause: fmt.Errorf("provider answered %d", resp.StatusCode)}
	}
	var out response
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		if ctx.Err() != nil {
			return nil, 0, ctx.Err()
		}
		return nil, 0, &Error{Reason: Rejected, Cause: fmt.Errorf("decode response: %w", err)}
	}
	if out.Usage.PromptTokens < 0 {
		return nil, 0, &Error{Reason: Rejected, Cause: fmt.Errorf("negative token count %d", out.Usage.PromptTokens)}
	}
	if len(out.Data) != len(batch) {
		return nil, out.Usage.PromptTokens, &Error{Reason: Rejected, Cause: fmt.Errorf("%d vectors for %d texts", len(out.Data), len(batch))}
	}
	vectors = make([][]float32, len(batch))
	seen := make([]bool, len(batch))
	for _, d := range out.Data {
		if d.Index < 0 || d.Index >= len(batch) || seen[d.Index] || len(d.Embedding) == 0 {
			return nil, out.Usage.PromptTokens, &Error{Reason: Rejected, Cause: fmt.Errorf("bad or empty vector at index %d", d.Index)}
		}
		seen[d.Index] = true
		vectors[d.Index] = d.Embedding
	}
	return vectors, out.Usage.PromptTokens, nil
}
