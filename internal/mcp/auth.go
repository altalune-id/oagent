// Package mcp wires the MCP surface: request authentication, the scopes a caller holds and error payloads.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"slices"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/authn"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/permission"
	rootmcp "altalune.id/oagent/mcp"
	"altalune.id/oagent/reqid"
)

const signInRequired = "Sign in required"

// AuthOptions configures Authenticate; ResourceMetadataURL is empty until MCP OAuth (P4).
type AuthOptions struct {
	Chain               authn.Chain
	KeyPrefix           string
	Required            bool
	ResourceMetadataURL string
	Logger              *slog.Logger
}

// Authenticate turns a Bearer key or Basic credential into a Principal. SECURITY: cookies are ignored and every failure is one opaque 401.
func Authenticate(o AuthOptions) func(http.Handler) http.Handler {
	log := o.Logger
	if log == nil {
		log = slog.Default()
	}
	challenge := WWWAuthenticate(o.ResourceMetadataURL)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Clone()
			h.Del("Cookie")
			cred, present := authn.FromHeader(h, o.KeyPrefix, nil)
			if !present {
				if o.Required {
					writeUnauthorized(w, r, challenge)
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			p, err := principalFor(r.Context(), o.Chain, cred)
			if err != nil {
				log.DebugContext(r.Context(), "mcp: credential rejected", "request_id", reqid.FromContext(r.Context()), "cause", err)
				writeUnauthorized(w, r, challenge)
				return
			}
			next.ServeHTTP(w, r.WithContext(session.PrincipalInto(r.Context(), p)))
		})
	}
}

func principalFor(ctx context.Context, chain authn.Chain, cred authn.Credential) (session.Principal, error) {
	if cred.Kind == authn.KindNone {
		return session.Principal{}, errors.New("malformed credential")
	}
	p, err := chain.Authenticate(ctx, cred)
	if ue, ok := errors.AsType[*authn.UnauthorizedError](err); ok {
		return p, ue.Causes()
	}
	return p, err
}

// WWWAuthenticate returns the challenge header value: `Bearer` or `Bearer resource_metadata="<url>"`.
func WWWAuthenticate(resourceMetadataURL string) string {
	if resourceMetadataURL == "" {
		return "Bearer"
	}
	return fmt.Sprintf("Bearer resource_metadata=%q", resourceMetadataURL)
}

// NOTE: the body is rootmcp.ErrorPayload, the shape a failed tool call carries, so one client parser reads both.
func writeUnauthorized(w http.ResponseWriter, r *http.Request, challenge string) {
	w.Header().Set("WWW-Authenticate", challenge)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(AttachContext(r.Context(), rootmcp.ErrorPayload{
		Code:    string(apperror.CodeUnauthenticated),
		Message: signInRequired,
	}))
}

// ScopesFromContext lists the permissions of permission.Table the caller holds; no Principal holds none.
func ScopesFromContext(ctx context.Context) []string {
	p := session.PrincipalFrom(ctx)
	if p.Source == "" {
		return nil
	}
	held := map[string]struct{}{}
	for _, perms := range permission.Table() {
		for _, perm := range perms {
			if authz.Allowed(p, perm) {
				held[perm] = struct{}{}
			}
		}
	}
	return slices.Sorted(maps.Keys(held))
}
