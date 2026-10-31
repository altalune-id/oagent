// Package mcp wires the MCP surface: request authentication, the scopes a caller holds and error payloads.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"slices"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/authn"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/kernel/tokens"
	"altalune.id/oagent/internal/permission"
	rootmcp "altalune.id/oagent/mcp"
	"altalune.id/oagent/reqid"
	"altalune.id/oagent/scope"
)

const (
	signInRequired = "Sign in required"
	maxPeekBytes   = 1 << 20
)

// AuthOptions configures Authenticate; a zero Resource answers a plain Bearer challenge, as when MCP OAuth is off.
type AuthOptions struct {
	Chain      authn.Chain
	KeyPrefix  string
	Required   bool
	Resource   tokens.Resource
	ToolScopes func(tool string) []string
	Logger     *slog.Logger
}

// Authenticate turns a key, provider token or Basic credential into a Principal. SECURITY: no cookies; one opaque 401; 503 on outage.
func Authenticate(o AuthOptions) func(http.Handler) http.Handler {
	log := o.Logger
	if log == nil {
		log = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Clone()
			h.Del("Cookie")
			cred, present := authn.FromHeader(h, o.KeyPrefix, nil)
			if !present {
				if o.Required {
					writeUnauthorized(w, r, o.Resource.Challenge())
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			p, err := principalFor(r.Context(), o.Chain, cred)
			if authn.IsUnavailableError(err) {
				log.ErrorContext(r.Context(), "mcp: credential check unavailable", "request_id", reqid.FromContext(r.Context()), "cause", err)
				writeUnavailable(w, r)
				return
			}
			if err != nil {
				log.DebugContext(r.Context(), "mcp: credential rejected", "request_id", reqid.FromContext(r.Context()), "cause", err)
				writeUnauthorized(w, r, o.Resource.InvalidToken())
				return
			}
			if p.Source == session.SourceToken {
				if missing := missingToolScope(r, p.TokenScopes, o.ToolScopes); missing != "" {
					writeInsufficientScope(w, r, o.Resource.InsufficientScope(missing), missing)
					return
				}
			}
			ctx := session.WithTenant(session.PrincipalInto(r.Context(), p), p)
			next.ServeHTTP(w, r.WithContext(ctx))
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

// NOTE: RFC 6750 Section 3.1 step-up: a tools/call needing a grantable scope the token lacks is refused before dispatch.
func missingToolScope(r *http.Request, granted []string, toolScopes func(string) []string) string {
	if toolScopes == nil || r.Method != http.MethodPost || r.Body == nil {
		return ""
	}
	head, err := io.ReadAll(io.LimitReader(r.Body, maxPeekBytes+1))
	r.Body = struct {
		io.Reader
		io.Closer
	}{io.MultiReader(bytes.NewReader(head), r.Body), r.Body}
	if err != nil || len(head) > maxPeekBytes {
		return ""
	}
	var call struct {
		Method string `json:"method"`
		Params struct {
			Name string `json:"name"`
		} `json:"params"`
	}
	if json.Unmarshal(head, &call) != nil || call.Method != "tools/call" {
		return ""
	}
	for _, need := range toolScopes(call.Params.Name) {
		if scope.Valid(need) && scope.Holds(need, scope.HolderPerson) && !scope.Grants(granted, need) {
			return need
		}
	}
	return ""
}

// NOTE: the body is rootmcp.ErrorPayload, the shape a failed tool call carries, so one client parser reads both.
func writeUnauthorized(w http.ResponseWriter, r *http.Request, challenge string) {
	w.Header().Set("WWW-Authenticate", challenge)
	writePayload(w, r, http.StatusUnauthorized, rootmcp.ErrorPayload{Code: string(apperror.CodeUnauthenticated), Message: signInRequired})
}

func writeInsufficientScope(w http.ResponseWriter, r *http.Request, challenge, missing string) {
	w.Header().Set("WWW-Authenticate", challenge)
	writePayload(w, r, http.StatusForbidden, rootmcp.ErrorPayload{
		Code:    string(apperror.CodeForbidden),
		Message: "Sign in again to allow " + missing,
		Meta:    map[string]string{"scope": missing},
	})
}

// NOTE: no WWW-Authenticate, so a client keeps its token through a provider or database outage.
func writeUnavailable(w http.ResponseWriter, r *http.Request) {
	ae := (&authn.UnavailableError{}).ToAppError()
	writePayload(w, r, http.StatusServiceUnavailable, rootmcp.ErrorPayload{Code: string(ae.Code()), Message: ae.Message()})
}

func writePayload(w http.ResponseWriter, r *http.Request, status int, payload rootmcp.ErrorPayload) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(AttachContext(r.Context(), payload))
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
