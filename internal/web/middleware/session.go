package middleware

import (
	"context"
	"net/http"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/session"
)

// Refresher rebuilds a stored Principal's authority.
type Refresher interface {
	Refresh(ctx context.Context, p session.Principal) (session.Principal, error)
}

// SessionConfig is what Session needs to turn a cookie into a Principal.
type SessionConfig struct {
	Store      session.Store
	Secret     []byte
	Auth       Refresher
	Unexpected apperror.UnexpectedFunc
}

// Session puts the cookie's Principal (authority rebuilt) and session id on the request context; any failure leaves it anonymous.
func Session(cfg SessionConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sid, ok := session.ReadCookie(r.Header, cfg.Secret)
			if !ok {
				next.ServeHTTP(w, r)
				return
			}
			ctx := r.Context()
			p, ok, err := cfg.Store.Load(ctx, sid)
			if err != nil {
				cfg.report(ctx, "web: load session", err)
			}
			if !ok {
				next.ServeHTTP(w, r)
				return
			}
			if p, err = cfg.Auth.Refresh(ctx, p); err != nil {
				cfg.report(ctx, "web: refresh session", err)
				next.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r.WithContext(session.SIDInto(session.PrincipalInto(ctx, p), sid)))
		})
	}
}

func (cfg SessionConfig) report(ctx context.Context, msg string, err error) {
	if ae, ok := apperror.AsAppError(err); ok && apperror.KindOf(ae.Code()) == apperror.KindNotFound {
		return
	}
	if cfg.Unexpected != nil {
		_ = cfg.Unexpected(ctx, msg, err)
	}
}
