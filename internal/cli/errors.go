package cli

import (
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/render"
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

const unauthenticatedHint = "not signed in or credential rejected; run `oagent auth login` or pass --token"

func errorInfo(err error) (msg, code string) {
	if ae, ok := apperror.AsAppError(err); ok {
		return ae.Message(), string(ae.Code())
	}
	if cerr, ok := errors.AsType[*connect.Error](err); ok {
		if cerr.Code() == connect.CodeUnauthenticated {
			c, _ := connectErrorCode(err)
			return unauthenticatedHint, string(c)
		}
		if c, cok := connectErrorCode(err); cok {
			return cerr.Message(), string(c)
		}
		return cerr.Message(), ""
	}
	return err.Error(), ""
}

// ProfileHostMismatchError reports a profile signed in to another server than the one requested. SECURITY: its session is not sent.
type ProfileHostMismatchError struct {
	Profile    string
	ProfileURL string
	URL        string
}

func (e *ProfileHostMismatchError) Error() string {
	return fmt.Sprintf("profile %s is signed in to %s, not %s; run `oagent auth login --url %s --profile %s`",
		e.Profile, e.ProfileURL, e.URL, e.URL, e.Profile)
}

// ToAppError maps the mismatch to Unauthenticated, so it exits like a rejected credential.
func (e *ProfileHostMismatchError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeUnauthenticated, e.Error())
}

// IsProfileHostMismatchError reports whether err is a *ProfileHostMismatchError.
func IsProfileHostMismatchError(err error) bool {
	_, ok := errors.AsType[*ProfileHostMismatchError](err)
	return ok
}

// SessionExpiredError reports a profile whose session has expired; it is not sent.
type SessionExpiredError struct{ Profile string }

func (e *SessionExpiredError) Error() string {
	return fmt.Sprintf("session for profile %s expired; run `oagent auth login --profile %s`", e.Profile, e.Profile)
}

// ToAppError maps the expiry to Unauthenticated.
func (e *SessionExpiredError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeUnauthenticated, e.Error())
}

// IsSessionExpiredError reports whether err is a *SessionExpiredError.
func IsSessionExpiredError(err error) bool {
	_, ok := errors.AsType[*SessionExpiredError](err)
	return ok
}

// SessionFileError reports a session file that cannot be read or written.
type SessionFileError struct {
	Path string
	Err  error
}

func (e *SessionFileError) Error() string {
	return fmt.Sprintf("cli: session file %s: %v", e.Path, e.Err)
}

func (e *SessionFileError) Unwrap() error { return e.Err }

// IsSessionFileError reports whether err is a *SessionFileError.
func IsSessionFileError(err error) bool {
	_, ok := errors.AsType[*SessionFileError](err)
	return ok
}
