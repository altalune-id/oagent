package controlplane

import (
	"context"

	"connectrpc.com/connect"

	"altalune.id/oagent/apperror"
	apperrorv1 "altalune.id/oagent/gen/go/apperror/v1"
	"altalune.id/oagent/reqid"
)

// Wrap converts handler errors into the canonical connect.Error envelope.
func Wrap(unexpected apperror.UnexpectedFunc) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			resp, err := next(ctx, req)
			if err == nil {
				return resp, nil
			}
			return resp, Translate(ctx, err, unexpected)
		}
	}
}

// Translate maps err onto a connect.Error carrying the typed ErrorDetail.
func Translate(ctx context.Context, err error, unexpected apperror.UnexpectedFunc) error {
	ae, ok := apperror.AsAppError(err)
	if !ok {
		if unexpected != nil {
			ae = unexpected(ctx, "api: unexpected", err)
		} else {
			ae = apperror.New(apperror.CodeUnexpected, "unexpected error").WithCause(err)
		}
	}

	cerr := connect.NewError(connectCodeOf(ae.Code()), ae)
	if det, derr := connect.NewErrorDetail(&apperrorv1.ErrorDetail{
		Code:       string(ae.Code()),
		Meta:       ae.Meta(),
		RequestId:  reqid.FromContext(ctx),
		Violations: violationDetails(ae.Violations()),
	}); derr == nil {
		cerr.AddDetail(det)
	}
	return cerr
}

func violationDetails(vs []apperror.Violation) []*apperrorv1.Violation {
	if len(vs) == 0 {
		return nil
	}
	out := make([]*apperrorv1.Violation, 0, len(vs))
	for _, v := range vs {
		out = append(out, &apperrorv1.Violation{Field: v.Field, Reason: string(v.Reason), Param: v.Param})
	}
	return out
}
