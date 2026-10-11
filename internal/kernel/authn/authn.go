// Package authn turns the credential a request presents into a Principal and checks it against a permission table.
package authn

import (
	"context"
	"errors"

	"altalune.id/oagent/internal/kernel/session"
)

// Authenticator turns a credential into a Principal; it refuses kinds it does not handle.
type Authenticator interface {
	Authenticate(ctx context.Context, c Credential) (session.Principal, error)
}

// Chain tries each Authenticator in order and returns the first success.
type Chain []Authenticator

// Authenticate implements Authenticator. SECURITY: every failure collapses to one opaque error, whatever the cause.
func (c Chain) Authenticate(ctx context.Context, cred Credential) (session.Principal, error) {
	var causes []error
	for _, a := range c {
		p, err := a.Authenticate(ctx, cred)
		if err == nil {
			return p, nil
		}
		causes = append(causes, err)
	}
	return session.Principal{}, &UnauthorizedError{causes: errors.Join(causes...)}
}
