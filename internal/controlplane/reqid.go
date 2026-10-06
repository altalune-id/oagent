package controlplane

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	"altalune.id/oagent/reqid"
)

// RequestID extracts (or mints) an X-Request-Id and threads it through ctx and response metadata.
func RequestID() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// NOTE: mint unconditionally; reqid.Ensure would reuse the process-wide BaseContext id.
			id := reqid.FromHeader(req.Header())
			if id == "" {
				id = reqid.New()
			}
			ctx = reqid.WithContext(ctx, id)
			resp, err := next(ctx, req)
			switch {
			case resp != nil:
				resp.Header().Set(reqid.Header, id)
			case err != nil:
				if cerr, ok := errors.AsType[*connect.Error](err); ok {
					cerr.Meta().Set(reqid.Header, id)
				}
			}
			return resp, err
		}
	}
}
