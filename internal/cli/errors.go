package cli

import (
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"

	"altalune.id/auth-server/apperror"
	"altalune.id/auth-server/internal/render"
)

// RenderError writes err to the command's stderr in the selected output format.
func RenderError(cmd *cobra.Command, err error) {
	msg, code := errorInfo(err)
	switch render.Detect(cmd) {
	case render.FormatJSON:
		_ = render.JSON(cmd.ErrOrStderr(), errorPayload(msg, code))
	case render.FormatNDJSON:
		_ = render.NDJSON(cmd.ErrOrStderr(), []map[string]string{errorPayload(msg, code)})
	default:
		if code != "" {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "error: %s (%s)\n", msg, code)
			return
		}
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "error:", msg)
	}
}

func errorPayload(msg, code string) map[string]string {
	m := map[string]string{"error": msg}
	if code != "" {
		m["code"] = code
	}
	return m
}

// errorInfo returns the clean message and, when available, the stable apperror code.
func errorInfo(err error) (msg, code string) {
	if ae, ok := apperror.AsAppError(err); ok {
		return ae.Message(), string(ae.Code())
	}
	if cerr, ok := errors.AsType[*connect.Error](err); ok {
		if c, cok := connectErrorCode(err); cok {
			return cerr.Message(), string(c)
		}
		return cerr.Message(), ""
	}
	return err.Error(), ""
}
