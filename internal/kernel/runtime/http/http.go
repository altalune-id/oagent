// Package http is the runtime driver that runs a turn on the runtime service over HTTP and NDJSON.
package http

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	nethttp "net/http"
	"net/url"
	"strings"

	"altalune.id/oagent/httpclient"
	"altalune.id/oagent/internal/kernel/runtime"
)

const (
	refusalBodyLimit = 4 << 10
	// MaxLineBytes is the longest NDJSON line the stream accepts; a snapshot rides on the terminal line.
	MaxLineBytes = 8 << 20
)

// ErrLineTooLong reports a stream line over MaxLineBytes.
var ErrLineTooLong = errors.New("runtime.http: stream line over 8 MiB")

// Client is the HTTP runtime driver.
type Client struct {
	endpoint string
	token    string
	hc       *nethttp.Client
	log      *slog.Logger
}

var _ runtime.Runtime = (*Client)(nil)

// New builds a client for the runtime at cfg.URL; the turn's context bounds each call.
func New(cfg runtime.Config) (*Client, error) {
	u, err := url.Parse(cfg.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("runtime.http: url must be an absolute http or https URL")
	}
	hc := httpclient.New(httpclient.WithAllowPrivateHosts(true), httpclient.WithoutTimeout(), httpclient.WithoutProxy())
	// SECURITY: a redirect would replay the body, which carries the provider key and MCP credentials, to another URL.
	hc.CheckRedirect = func(*nethttp.Request, []*nethttp.Request) error { return nethttp.ErrUseLastResponse }
	return &Client{endpoint: strings.TrimSuffix(u.String(), "/") + "/turn", token: cfg.Token, hc: hc, log: slog.Default()}, nil
}

// WithLogger logs refusals to l.
func (c *Client) WithLogger(l *slog.Logger) *Client {
	c.log = l
	return c
}

// Turn posts req and returns its event stream; a refusal before the stream is a typed error. SECURITY: req is never logged.
func (c *Client) Turn(ctx context.Context, req runtime.TurnRequest) (runtime.Stream, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("runtime.http: encode turn: %w", err)
	}
	httpReq, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("runtime.http: build turn: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/x-ndjson")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.hc.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("runtime.http: turn: %w", err)
	}
	if resp.StatusCode == nethttp.StatusOK {
		return &stream{body: resp.Body, r: bufio.NewReaderSize(resp.Body, 64<<10)}, nil
	}
	defer func() { _ = resp.Body.Close() }()
	err = refusal(resp)
	c.log.WarnContext(ctx, "runtime: turn refused", slog.Int("status", resp.StatusCode), slog.String("err", err.Error()))
	return nil, err
}

func refusal(resp *nethttp.Response) error {
	var body struct {
		Error string `json:"error"`
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, refusalBodyLimit))
	_ = json.Unmarshal(raw, &body)
	switch resp.StatusCode {
	case nethttp.StatusBadRequest, nethttp.StatusUnauthorized, nethttp.StatusRequestEntityTooLarge:
		return &runtime.ContractError{Status: resp.StatusCode, Code: body.Error}
	case nethttp.StatusUnprocessableEntity:
		if body.Error == "snapshot_unloadable" {
			return &runtime.SnapshotUnloadableError{}
		}
		return &runtime.ContractError{Status: resp.StatusCode, Code: body.Error}
	case nethttp.StatusServiceUnavailable:
		return &runtime.BusyError{}
	default:
		return &runtime.StatusError{Status: resp.StatusCode}
	}
}

type stream struct {
	body io.ReadCloser
	r    *bufio.Reader
	done bool
}

func (s *stream) line() ([]byte, error) {
	var line []byte
	for {
		chunk, err := s.r.ReadSlice('\n')
		if len(line)+len(chunk) > MaxLineBytes {
			return nil, ErrLineTooLong
		}
		line = append(line, chunk...)
		switch {
		case err == nil:
			return line, nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF) && len(bytes.TrimSpace(line)) > 0:
			return line, nil
		default:
			return nil, err
		}
	}
}

func (s *stream) Next() (runtime.Event, error) {
	if s.done {
		return nil, io.EOF
	}
	for {
		line, err := s.line()
		if errors.Is(err, ErrLineTooLong) {
			return nil, &runtime.ContractError{Status: nethttp.StatusOK, Cause: err}
		}
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, &runtime.NoTerminalError{}
		}
		if err != nil {
			return nil, fmt.Errorf("runtime.http: read stream: %w", err)
		}
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		e, err := runtime.DecodeEvent(bytes.TrimSpace(line))
		if err != nil {
			return nil, &runtime.ContractError{Status: nethttp.StatusOK, Cause: err}
		}
		if _, ok := e.(*runtime.Terminal); ok {
			s.done = true
		}
		return e, nil
	}
}

func (s *stream) Close() error { return s.body.Close() }
