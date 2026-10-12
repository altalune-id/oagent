// Package resend is the mail driver that sends through the Resend HTTP API.
package resend

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"altalune.id/oagent/httpclient"
	"altalune.id/oagent/mailer"
	"altalune.id/oagent/nanoid"
)

// Driver defaults. https://resend.com/docs/api-reference/emails/send-email
const (
	DefaultEndpoint    = "https://api.resend.com/emails"
	DefaultMaxAttempts = 3
)

const (
	userAgent      = "oagent-mailer-resend/1"
	bodyLimit      = 64 << 10
	sendTimeout    = 10 * time.Second
	idempotencyLen = 24
	snippetLimit   = 512
)

var _ mailer.Mailer = (*Sender)(nil)

// Config configures the driver; Endpoint and MaxAttempts fall back to the defaults.
type Config struct {
	APIKey      string
	Endpoint    string
	MaxAttempts int
}

// Sender sends each Message as one Resend API call.
type Sender struct {
	from         string
	apiKey       string
	endpoint     string
	allowPrivate bool
	hc           *http.Client
}

// New builds a Sender, rejecting a missing sender or API key and an unusable endpoint.
func New(from string, cfg Config) (*Sender, error) {
	endpoint := cmp.Or(cfg.Endpoint, DefaultEndpoint)
	// SECURITY: the SSRF dial filter stays on for the public API; only an operator-set endpoint may reach a private address.
	return build(from, cfg, endpoint != DefaultEndpoint)
}

func build(from string, cfg Config, allowPrivate bool) (*Sender, error) {
	if from == "" {
		return nil, &mailer.MissingConfigError{Field: "from"}
	}
	if cfg.APIKey == "" {
		return nil, &mailer.MissingConfigError{Field: "resend.apiKey"}
	}
	endpoint := cmp.Or(cfg.Endpoint, DefaultEndpoint)
	if err := checkEndpoint(endpoint); err != nil {
		return nil, err
	}
	attempts := cmp.Or(cfg.MaxAttempts, DefaultMaxAttempts)
	if attempts < 1 {
		return nil, &mailer.InvalidConfigError{Field: "resend.maxAttempts", Value: strconv.Itoa(attempts), Reason: "must be at least 1"}
	}
	hc := httpclient.New(
		httpclient.WithTimeout(sendTimeout),
		httpclient.WithResponseBodyLimit(bodyLimit),
		httpclient.WithAllowPrivateHosts(allowPrivate),
		// NOTE: replaying a POST is safe here only because every Send carries one Idempotency-Key across attempts.
		httpclient.WithRetry(httpclient.RetryPolicy{MaxAttempts: attempts, RetryNonIdempotent: true}),
	)
	return &Sender{from: from, apiKey: cfg.APIKey, endpoint: endpoint, allowPrivate: allowPrivate, hc: hc}, nil
}

// Send delivers m through the Resend API, retrying rate limits and server errors under one idempotency key.
func (r *Sender) Send(ctx context.Context, m mailer.Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	m.From = cmp.Or(m.From, r.from)
	if err := m.Validate(); err != nil {
		return err
	}
	key, ok := mailer.IdempotencyKey(ctx)
	if !ok {
		var err error
		if key, err = nanoid.New(idempotencyLen); err != nil {
			return fmt.Errorf("mailer: resend: idempotency key: %w", err)
		}
	}
	body, err := json.Marshal(payload{
		From:    m.From,
		To:      []string{m.To},
		Subject: m.Subject,
		Text:    m.TextBody,
		HTML:    m.HTMLBody,
	})
	if err != nil {
		return fmt.Errorf("mailer: resend: encode: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("mailer: resend: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Idempotency-Key", key)

	resp, err := r.hc.Do(req)
	if err != nil {
		return fmt.Errorf("mailer: resend: send: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, snippetLimit))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return &APIError{
		StatusCode: resp.StatusCode,
		RetryAfter: httpclient.ParseRetryAfter(resp.Header.Get("Retry-After")),
		Body:       bodySnippet(snippet),
	}
}

type payload struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Text    string   `json:"text,omitempty"`
	HTML    string   `json:"html,omitempty"`
}

func checkEndpoint(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return &mailer.InvalidConfigError{Field: "resend.endpoint", Value: raw, Reason: err.Error()}
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return &mailer.InvalidConfigError{Field: "resend.endpoint", Value: raw, Reason: "scheme must be http or https"}
	}
	if u.Host == "" {
		return &mailer.InvalidConfigError{Field: "resend.endpoint", Value: raw, Reason: "host required"}
	}
	return nil
}

func bodySnippet(b []byte) string {
	return strings.ToValidUTF8(string(b[:min(len(b), snippetLimit)]), "")
}
