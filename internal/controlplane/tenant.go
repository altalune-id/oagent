package controlplane

import (
	"context"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/kernel/tenant"
)

func tenantFromPrincipal() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if p := session.PrincipalFrom(ctx); p.ActiveOrgID != uuid.Nil {
				ctx = tenant.Into(ctx, tenant.Context{OrgID: p.ActiveOrgID, ProjectID: p.ActiveProjectID, UserID: p.UserID})
			}
			return next(ctx, req)
		}
	}
}
