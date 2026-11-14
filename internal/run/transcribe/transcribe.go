// Package transcribe calls an OpenAI-compatible speech-to-text endpoint through the SSRF-guarded HTTP client.
package transcribe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"

	"altalune.id/oagent/httpclient"
)

// The call limits.
const (
	Timeout          = 20 * time.Second
	maxResponseBytes = 1 << 20
)

// Filename is the upload name; providers pick the decoder from its extension.
const Filename = "voice.ogg"

// Config names the endpoint and model.
type Config struct {
	BaseURL           string
	APIKey            string
	Model             string
	AllowPrivateHosts bool
}

// Client transcribes audio with one model.
type Client struct {
	cfg      Config
	endpoint string
	http     *http.Client
}

// New checks cfg and returns a client on the guarded HTTP client; it never retries, since a replayed upload may be billed twice.
func New(cfg Config) (*Client, error) {
	if strings.ContainsAny(cfg.BaseURL, "\r\n") || strings.ContainsAny(cfg.APIKey, "\r\n") || strings.ContainsAny(cfg.Model, "\r\n") {
		return nil, &Error{Reason: Rejected, Cause: errors.New("base URL, key or model holds a line break")}
	}
	if cfg.BaseURL == "" || cfg.Model == "" {
		return nil, &Error{Reason: Rejected, Cause: errors.New("base URL and model are required")}
	}
	u, err := url.Parse(cfg.BaseURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, &Error{Reason: Rejected, Cause: errors.New("base URL must be a plain URL with a host and no credentials, query or fragment")}
	}
	if u.Scheme != "https" && (u.Scheme != "http" || !cfg.AllowPrivateHosts) {
		return nil, &Error{Reason: Rejected, Cause: errors.New("base URL must be https")}
	}
	client := httpclient.New(
		httpclient.WithAllowPrivateHosts(cfg.AllowPrivateHosts),
		httpclient.WithoutProxy(),
		httpclient.WithoutRedirects(),
		httpclient.WithTimeout(Timeout),
		httpclient.WithResponseHeaderTimeout(Timeout),
		httpclient.WithResponseBodyLimit(maxResponseBytes),
	)
	return &Client{cfg: cfg, endpoint: u.JoinPath("audio", "transcriptions").String(), http: client}, nil
}

type response struct {
	Text string `json:"text"`
}

// Close drops the client's idle connections.
func (c *Client) Close() { c.http.CloseIdleConnections() }

// Transcribe sends one clip and returns the provider's text, which may be empty.
func (c *Client) Transcribe(ctx context.Context, audio []byte, mime string) (string, error) {
	if mime == "" || strings.ContainsAny(mime, "\r\n") {
		return "", &Error{Reason: Rejected, Cause: errors.New("audio type is empty or holds a line break")}
	}
	body, contentType, err := form(c.cfg.Model, audio, mime)
	if err != nil {
		return "", &Error{Reason: Rejected, Cause: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", &Error{Reason: Rejected, Cause: err}
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", &Error{Reason: Unreachable, Cause: err}
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return "", &Error{Reason: RateLimited, Cause: fmt.Errorf("provider answered %d", resp.StatusCode)}
	case httpclient.RetryableStatus(resp.StatusCode):
		return "", &Error{Reason: Unreachable, Cause: fmt.Errorf("provider answered %d", resp.StatusCode)}
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		return "", &Error{Reason: Rejected, Cause: fmt.Errorf("provider answered %d", resp.StatusCode)}
	}
	var out response
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", &Error{Reason: Rejected, Cause: fmt.Errorf("decode response: %w", err)}
	}
	return out.Text, nil
}

func form(model string, audio []byte, mime string) (body []byte, contentType string, err error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="file"; filename="`+Filename+`"`)
	h.Set("Content-Type", mime)
	part, err := w.CreatePart(h)
	if err != nil {
		return nil, "", err
	}
	if _, err := part.Write(audio); err != nil {
		return nil, "", err
	}
	if err := w.WriteField("model", model); err != nil {
		return nil, "", err
	}
	if err := w.WriteField("response_format", "json"); err != nil {
		return nil, "", err
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), w.FormDataContentType(), nil
}
