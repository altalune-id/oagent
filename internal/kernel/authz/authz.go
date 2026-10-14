// Package authz answers whether a Principal may perform a permission.
package authz

import (
	"context"

	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/scope"
)

// AnyAuthenticated marks a procedure every signed-in credential may call.
const AnyAuthenticated = "@authenticated"

// PersonOnly marks a procedure only a signed-in person may call: never a key, basic auth or the system.
const PersonOnly = "@person"

// Allowed reports whether p holds permission. SECURITY: an empty permission is an unmapped procedure and is denied.
func Allowed(p session.Principal, permission string) bool {
	if p.Source == "" || permission == "" {
		return false
	}
	if permission == AnyAuthenticated {
		return true
	}
	if permission == PersonOnly {
		return p.IsPerson()
	}
	if p.Source == session.SourceAPIKey && !scope.Mintable(permission) {
		return false
	}
	if p.Operator {
		return true
	}
	return scope.Grants(p.Scopes, permission)
}

// Require returns a *ForbiddenError unless the Principal in ctx holds permission.
func Require(ctx context.Context, permission string) error {
	if !Allowed(session.PrincipalFrom(ctx), permission) {
		return &ForbiddenError{Permission: permission}
	}
	return nil
}
