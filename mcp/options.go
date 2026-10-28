package mcp

import (
	"context"
	"log/slog"
	"strings"
)

// ScopesFunc resolves the caller's scopes for the per-tool check.
type ScopesFunc func(ctx context.Context) []string

// ErrorMapper maps a tool failure onto the caller-visible payload; empty Code means unmapped.
type ErrorMapper func(ctx context.Context, err error) ErrorPayload

// Option configures a Server.
type Option func(*Server)

// WithScopes wires the caller's scope check; without it every tool call is denied.
func WithScopes(f ScopesFunc) Option {
	return func(s *Server) { s.scopesFrom = f }
}

// WithRegistry attaches reg's tools to the server.
func WithRegistry(reg *Registry) Option {
	return func(s *Server) { s.registry = reg }
}

// WithImplementation sets the name and version reported to MCP hosts.
func WithImplementation(name, version string) Option {
	return func(s *Server) {
		s.name = name
		s.version = version
	}
}

// WithErrorMapper wires the tool-failure mapper.
func WithErrorMapper(m ErrorMapper) Option {
	return func(s *Server) { s.mapErr = m }
}

// WithLogger sets the logger for unmapped tool failures.
func WithLogger(l *slog.Logger) Option {
	return func(s *Server) {
		if l != nil {
			s.logger = l
		}
	}
}

// WithUnmappedCode sets the code an unmapped tool failure answers with.
func WithUnmappedCode(code string) Option {
	return func(s *Server) {
		if code != "" {
			s.unmappedCode = code
		}
	}
}

// WithUI publishes ToolSpec.UI as the MCP Apps _meta.ui link.
func WithUI(enabled bool) Option {
	return func(s *Server) { s.ui = enabled }
}

// WithAllowedHosts lets a loopback request carry one of hosts as its Host, as a same-host proxy sends; other hosts stay refused.
func WithAllowedHosts(hosts ...string) Option {
	return func(s *Server) {
		for _, h := range hosts {
			if h != "" {
				s.allowedHosts = append(s.allowedHosts, strings.ToLower(h))
			}
		}
	}
}
