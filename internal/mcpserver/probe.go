package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"altalune.id/oagent/httpclient"
)

//nolint:gochecknoglobals // immutable compiled pattern.
var grantableNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// ProbeTimeout bounds one tool listing, below the API server's write timeout.
const ProbeTimeout = 10 * time.Second

// MaxTools bounds the tools one server may list; more answers UnreachableError.
const MaxTools = 512

const (
	probeBodyLimit   = 1 << 20
	probeMaxRedirect = 5
	probeMaxPages    = 32

	maxDescriptionRunes = 1024
	maxSchemaBytes      = 16 << 10
	maxNameRunes        = 128
)

// Tool is one tool a server lists; ReadOnly needs a trusted server, and Grantable needs a valid prefixed name.
type Tool struct {
	Name, Description   string
	ReadOnly, Grantable bool
	InputSchema         json.RawMessage
}

var (
	errRedirectInsecure = errors.New("mcpserver: redirect to a non-https URL")
	errRedirectPrivate  = errors.New("mcpserver: redirect to a private address")
	errRedirectTooMany  = errors.New("mcpserver: too many redirects")
	errTooManyTools     = errors.New("mcpserver: server lists too many tools")
	errTooManyPages     = errors.New("mcpserver: server lists too many pages of tools")
	errNoTransport      = errors.New("mcpserver: probe client has no transport")
)

type prober struct {
	client *http.Client
	log    *slog.Logger
}

func newProber() prober { return prober{client: NewProbeClient(false)} }

// WithHTTPClient sets the client the probe calls servers with.
func (s *Service) WithHTTPClient(c *http.Client) *Service {
	s.prober.client = c
	return s
}

// WithLogger logs why a probe failed, with any credential redacted; nil logs nothing.
func (s *Service) WithLogger(l *slog.Logger) *Service {
	s.prober.log = l
	return s
}

// NewProbeClient returns the bounded client the probe uses; allowPrivate lets it dial private hosts, for development only.
func NewProbeClient(allowPrivate bool) *http.Client {
	c := httpclient.New(
		httpclient.WithTimeout(ProbeTimeout),
		httpclient.WithResponseBodyLimit(probeBodyLimit),
		httpclient.WithAllowPrivateHosts(allowPrivate),
	)
	c.CheckRedirect = checkProbeRedirect
	return c
}

// SECURITY: every redirect hop must stay on https and never name a private address literal, even where private dials are allowed.
func checkProbeRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= probeMaxRedirect {
		return errRedirectTooMany
	}
	if req.URL.Scheme != "https" {
		return errRedirectInsecure
	}
	if addr, err := netip.ParseAddr(req.URL.Hostname()); err == nil && httpclient.IsPrivateOrLocal(addr) {
		return errRedirectPrivate
	}
	return nil
}

// ListTools connects to a server and lists the tools it offers; a server that cannot be reached is an *UnreachableError.
func (s *Service) ListTools(ctx context.Context, id uuid.UUID) ([]Tool, error) {
	srv, err := s.store.ByID(ctx, id)
	if err != nil {
		return nil, s.errOrReport(ctx, "mcpserver: list tools", err)
	}
	value, err := s.credential(ctx, srv)
	if err != nil {
		return nil, s.errOrReport(ctx, "mcpserver: list tools open", secretsErr(err))
	}
	tools, err := s.probe(ctx, srv, value)
	if err != nil {
		cause := redact(err, value)
		if s.prober.log != nil {
			s.prober.log.WarnContext(ctx, "mcpserver: probe failed", "server_id", srv.PublicID, "error", cause)
		}
		if IsAuthRefusedError(err) {
			return nil, err
		}
		return nil, &UnreachableError{Cause: cause}
	}
	return tools, nil
}

// SECURITY: a server may echo the credential in an error body, so the cause never carries it.
func redact(err error, value string) error {
	if value == "" {
		return err
	}
	return errors.New(strings.ReplaceAll(err.Error(), value, "[redacted]"))
}

func (s *Service) probe(ctx context.Context, srv *Server, value string) ([]Tool, error) {
	ctx, cancel := context.WithTimeout(ctx, ProbeTimeout)
	defer cancel()
	origin, err := originOf(srv.URL)
	if err != nil {
		return nil, err
	}
	client := *s.prober.client
	auth := &authTransport{
		probe: ctx, base: client.Transport, origin: origin, kind: srv.AuthKind, header: srv.AuthHeader, value: value,
	}
	client.Transport = auth
	tools, err := s.listTools(ctx, srv, &client)
	if err != nil {
		if status := auth.refused.Load(); status != 0 {
			return nil, &AuthRefusedError{Status: int(status)}
		}
		return nil, err
	}
	return tools, nil
}

func (s *Service) listTools(ctx context.Context, srv *Server, client *http.Client) ([]Tool, error) {
	transport := &mcp.StreamableClientTransport{Endpoint: srv.URL, HTTPClient: client, MaxRetries: -1, DisableStandaloneSSE: true}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "oagent", Version: "1"}, nil).Connect(ctx, transport, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cs.Close() }()
	var out []Tool
	cursor := ""
	for range probeMaxPages {
		page, err := cs.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, err
		}
		for _, t := range page.Tools {
			if sanitizeText(t.Name, maxNameRunes) == "" {
				continue
			}
			if len(out) == MaxTools {
				return nil, errTooManyTools
			}
			tool, err := toTool(srv, t)
			if err != nil {
				return nil, err
			}
			out = append(out, tool)
		}
		if page.NextCursor == "" {
			return out, nil
		}
		cursor = page.NextCursor
	}
	return nil, errTooManyPages
}

func toTool(srv *Server, t *mcp.Tool) (Tool, error) {
	schema, err := json.Marshal(t.InputSchema)
	if err != nil {
		return Tool{}, err
	}
	name := sanitizeText(t.Name, maxNameRunes)
	return Tool{
		Name:        name,
		Description: sanitizeDescription(t.Description),
		ReadOnly:    srv.Trusted && t.Annotations != nil && t.Annotations.ReadOnlyHint,
		Grantable:   grantableNameRe.MatchString(srv.Name + "__" + name),
		InputSchema: sanitizeSchema(schema),
	}, nil
}

// SECURITY: remote strings keep newlines but lose control, format (bidi, zero-width, tag) and private-use characters, capped.
func sanitizeText(text string, maxRunes int) string {
	var b strings.Builder
	n := 0
	for _, r := range strings.ToValidUTF8(text, "") {
		if n == maxRunes {
			break
		}
		if r != '\n' && (unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Co)) {
			continue
		}
		b.WriteRune(r)
		n++
	}
	return b.String()
}

func sanitizeDescription(d string) string { return sanitizeText(d, maxDescriptionRunes) }

// SECURITY: schema titles and descriptions reach the model too: sanitized at every depth; an oversized schema becomes empty.
func sanitizeSchema(raw json.RawMessage) json.RawMessage {
	empty := json.RawMessage(`{"type":"object"}`)
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return empty
	}
	out, err := json.Marshal(sanitizeSchemaValue(v))
	if err != nil || len(out) > maxSchemaBytes {
		return empty
	}
	return out
}

func sanitizeSchemaValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if str, ok := child.(string); ok && (k == "description" || k == "title") {
				t[k] = sanitizeDescription(str)
				continue
			}
			t[k] = sanitizeSchemaValue(child)
		}
	case []any:
		for i, child := range t {
			t[i] = sanitizeSchemaValue(child)
		}
	}
	return v
}

func originOf(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	return origin(u), nil
}

func origin(u *url.URL) string {
	port := u.Port()
	if port == "" {
		port = map[string]string{"https": "443", "http": "80"}[u.Scheme]
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(net.JoinHostPort(u.Hostname(), port))
}

type authTransport struct {
	// refused holds the 401 or 403 status the server answered, if any.
	refused atomic.Int32
	probe   context.Context
	base    http.RoundTripper
	origin  string
	kind    AuthKind
	header  string
	value   string
}

// SECURITY: the credential goes only to the server's own origin, never to a redirect target elsewhere, and only on a clone of the request.
func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.probe.Err(); err != nil {
		return nil, err
	}
	if t.base == nil {
		return nil, errNoTransport
	}
	if origin(req.URL) != t.origin {
		return t.base.RoundTrip(req)
	}
	switch t.kind {
	case AuthBearer:
		req = req.Clone(req.Context())
		req.Header.Set("Authorization", "Bearer "+t.value)
	case AuthHeader:
		req = req.Clone(req.Context())
		req.Header.Set(t.header, t.value)
	}
	resp, err := t.base.RoundTrip(req)
	if err == nil && (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) {
		t.refused.CompareAndSwap(0, int32(resp.StatusCode))
	}
	return resp, err
}
