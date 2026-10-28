package controlplane

import (
	"context"

	"connectrpc.com/connect"

	"altalune.id/oagent/internal/kernel/session"
)

func tenantFromPrincipal() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			return next(session.WithTenant(ctx, session.PrincipalFrom(ctx)), req)
		}
	}
}
