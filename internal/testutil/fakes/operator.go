package fakes

import (
	"context"

	"connectrpc.com/connect"

	"altalune.id/oagent/internal/kernel/authn"
	"altalune.id/oagent/internal/kernel/session"
)

// OperatorToken is the bearer token OperatorAuthn accepts.
const OperatorToken = "oagent_test_operator" //nolint:gosec // G101: a fixed test token, not a credential.

// OperatorContext returns ctx acting as the system operator, which reaches every org.
func OperatorContext(ctx context.Context) context.Context {
	return session.PrincipalInto(ctx, session.System())
}

// OperatorInterceptor runs every call that carries no Principal as the system operator.
func OperatorInterceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if session.PrincipalFrom(ctx).Source == "" {
				ctx = OperatorContext(ctx)
			}
			return next(ctx, req)
		}
	}
}

// OperatorAuthn authenticates OperatorToken as the system operator.
type OperatorAuthn struct{}

// Authenticate implements authn.Authenticator.
func (OperatorAuthn) Authenticate(_ context.Context, c authn.Credential) (session.Principal, error) {
	if c.Kind == authn.KindAPIKey && c.Token == OperatorToken {
		return session.System(), nil
	}
	return session.Principal{}, &authn.UnauthorizedError{}
}
