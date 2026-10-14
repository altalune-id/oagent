package authn

import (
	"context"
	"errors"
	"log/slog"

	"connectrpc.com/connect"

	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/reqid"
)

// PermissionTable maps a Connect procedure to the permissions it needs; the caller must hold all of them.
type PermissionTable map[string][]string

// InterceptorOptions configures Interceptor; Required refuses a request that carries no credential.
type InterceptorOptions struct {
	Chain        Chain
	KeyPrefix    string
	CookieSecret []byte
	Permissions  PermissionTable
	Required     bool
	Logger       *slog.Logger
}

// Interceptor authenticates and checks permissions. SECURITY: unmapped procedures are denied; a bad credential is never anonymous.
func Interceptor(o InterceptorOptions) connect.UnaryInterceptorFunc {
	log := o.Logger
	if log == nil {
		log = slog.Default()
	}
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			cred, present := FromHeader(req.Header(), o.KeyPrefix, o.CookieSecret)
			if !present {
				if o.Required {
					return nil, &UnauthorizedError{}
				}
				return next(ctx, req)
			}
			if cred.Kind == KindNone {
				log.DebugContext(ctx, "authn: malformed credential", "request_id", reqid.FromContext(ctx))
				return nil, &UnauthorizedError{}
			}
			p, err := o.Chain.Authenticate(ctx, cred)
			if err != nil {
				cause := err
				if ue, ok := errors.AsType[*UnauthorizedError](err); ok {
					cause = ue.Causes()
				}
				log.DebugContext(ctx, "authn: credential rejected", "request_id", reqid.FromContext(ctx), "cause", cause)
				return nil, &UnauthorizedError{}
			}
			perms := o.Permissions[req.Spec().Procedure]
			if len(perms) == 0 {
				return nil, &authz.ForbiddenError{}
			}
			for _, perm := range perms {
				if !authz.Allowed(p, perm) {
					return nil, &authz.ForbiddenError{Permission: perm}
				}
			}
			ctx = session.PrincipalInto(ctx, p)
			if cred.Kind == KindSession {
				ctx = session.SIDInto(ctx, cred.Token)
			}
			return next(ctx, req)
		}
	}
}
