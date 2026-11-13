// Package mcpserver models the remote MCP servers a project's agents may call tools on.
package mcpserver

import (
	"strings"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/nanoid"
	"altalune.id/oagent/weburl"
)

// MaxURLLen bounds a server URL, in bytes.
const MaxURLLen = weburl.MaxLen

// AuthKind says how oagent authenticates to a server.
type AuthKind string

// The ways oagent authenticates to a server.
const (
	AuthNone   AuthKind = "none"
	AuthBearer AuthKind = "bearer"
	AuthHeader AuthKind = "header"
)

// Input is the editable part of a server; Update ignores Name, the tool prefix, which never changes.
type Input struct {
	Name       string
	URL        string
	AuthKind   AuthKind
	AuthHeader string
	TimeoutMS  int32
	Trusted    bool
}

// Server is the aggregate root: one remote MCP endpoint and its credential, scoped to a project.
type Server struct {
	ID, OrgID, ProjectID uuid.UUID
	PublicID, Name       string
	URL                  string
	AuthKind             AuthKind
	// AuthHeader is the header name when AuthKind is AuthHeader, empty for the other kinds.
	AuthHeader           string
	AuthSealed           []byte
	TimeoutMS            int32
	Trusted              bool
	CreatedAt, UpdatedAt time.Time
}

// Ref names a server and the tenant it belongs to, before any row is read.
type Ref struct {
	ID        uuid.UUID
	OrgID     uuid.UUID
	ProjectID uuid.UUID
}

// ValidateOption relaxes a server validation rule.
type ValidateOption func(*validateOpts)

type validateOpts struct{ allowInsecureURL bool }

// AllowInsecureURL permits http:// server URLs; self-hosted dev only.
func AllowInsecureURL() ValidateOption { return func(o *validateOpts) { o.allowInsecureURL = true } }

func resolveValidateOpts(opts []ValidateOption) validateOpts {
	var o validateOpts
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// New enforces creation invariants and returns a server without a sealed credential.
func New(orgID, projectID uuid.UUID, in Input, now time.Time, opts ...ValidateOption) (*Server, error) {
	u, header, err := validate(in, resolveValidateOpts(opts))
	if err != nil {
		return nil, err
	}
	timeout, err := timeoutOf(in.TimeoutMS, RecommendedTimeoutMS)
	if err != nil {
		return nil, err
	}
	publicID, err := nanoid.New(21)
	if err != nil {
		return nil, err
	}
	now = now.UTC()
	return &Server{
		ID:         uuid.Must(uuid.NewV7()),
		PublicID:   publicID,
		OrgID:      orgID,
		ProjectID:  projectID,
		Name:       strings.TrimSpace(in.Name),
		URL:        u,
		AuthKind:   in.AuthKind,
		AuthHeader: header,
		TimeoutMS:  timeout,
		Trusted:    in.Trusted,
		CreatedAt:  now,
		UpdatedAt:  now,
	}, nil
}

// Update re-validates the editable fields but the name, keeps the sealed credential, and reports whether the URL changed.
func (s *Server) Update(in Input, now time.Time, opts ...ValidateOption) (urlChanged bool, err error) {
	u, header, err := validate(in, resolveValidateOpts(opts))
	if err != nil {
		return false, err
	}
	timeout, err := timeoutOf(in.TimeoutMS, s.TimeoutMS)
	if err != nil {
		return false, err
	}
	urlChanged = u != s.URL
	s.URL = u
	s.AuthKind = in.AuthKind
	s.AuthHeader = header
	if in.AuthKind == AuthNone {
		s.AuthSealed = nil
	}
	s.TimeoutMS = timeout
	s.Trusted = in.Trusted
	s.UpdatedAt = now.UTC()
	return urlChanged, nil
}

func timeoutOf(ms, unset int32) (int32, error) {
	switch {
	case ms == 0:
		return unset, nil
	case ms < MinTimeoutMS || ms > MaxTimeoutMS:
		return 0, apperror.Invalid("timeout_ms", apperror.FieldInvalid)
	}
	return ms, nil
}

func validate(in Input, o validateOpts) (u, header string, err error) {
	u, reason := weburl.Check(in.URL, o.allowInsecureURL)
	if reason != "" {
		return "", "", &InvalidURLError{Reason: URLReason(reason)}
	}
	if in.AuthKind != AuthHeader {
		return u, "", nil
	}
	header = strings.TrimSpace(in.AuthHeader)
	if !validHeaderName(header) {
		return "", "", &SecretRequiredError{Reason: SecretHeaderName}
	}
	return u, header, nil
}

// SECURITY: a header that frames, routes or negotiates the request would let a tenant break the connection oagent makes.
func validHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if !isTokenChar(r) {
			return false
		}
	}
	for _, reserved := range ReservedHeaders {
		if strings.EqualFold(name, reserved) {
			return false
		}
	}
	return true
}

// ReservedHeaders are the header names a server's auth may never set, compared case-insensitively.
var ReservedHeaders = []string{ //nolint:gochecknoglobals // a fixed list the runtime mirrors.
	"Host", "Content-Length", "Transfer-Encoding", "Connection", "Content-Type", "Accept", "Mcp-Session-Id",
	"MCP-Protocol-Version", "TE", "Trailer", "Upgrade", "Keep-Alive", "Proxy-Authorization", "Proxy-Connection", "Expect",
}

// NOTE: tchar from RFC 9110 section 5.6.2, https://www.rfc-editor.org/rfc/rfc9110#section-5.6.2
func isTokenChar(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	}
	return strings.ContainsRune("!#$%&'*+-.^_`|~", r)
}
