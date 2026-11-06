package mcpserver

import (
	"errors"
	"strconv"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/weburl"
)

// NotFoundError reports that no server matched the lookup.
type NotFoundError struct{ ID string }

func (e *NotFoundError) Error() string {
	if e.ID == "" {
		return "mcpserver: server not found"
	}
	return "mcpserver: server not found: id=" + apperror.Echo(e.ID)
}

// IsNotFoundError reports whether err's tree contains a *NotFoundError.
func IsNotFoundError(err error) bool {
	_, ok := errors.AsType[*NotFoundError](err)
	return ok
}

// AlreadyExistsError reports that the project already has a server with this name.
type AlreadyExistsError struct{ Name string }

func (e *AlreadyExistsError) Error() string {
	return "mcpserver: name already taken: " + apperror.Echo(e.Name)
}

// IsAlreadyExistsError reports whether err's tree contains an *AlreadyExistsError.
func IsAlreadyExistsError(err error) bool {
	_, ok := errors.AsType[*AlreadyExistsError](err)
	return ok
}

// URLReason is the stable cause of an InvalidURLError, sent to clients as meta.reason.
type URLReason string

// The causes of an InvalidURLError.
const (
	URLRequired       = URLReason(weburl.Required)
	URLTooLong        = URLReason(weburl.TooLong)
	URLNotUTF8        = URLReason(weburl.NotUTF8)
	URLUnparsable     = URLReason(weburl.Unparsable)
	URLHTTPSRequired  = URLReason(weburl.HTTPSRequired)
	URLHasCredentials = URLReason(weburl.HasCredentials)
	URLNoHost         = URLReason(weburl.NoHost)
)

// InvalidURLError reports that the server URL breaks its invariants.
type InvalidURLError struct{ Reason URLReason }

func (e *InvalidURLError) Error() string {
	return "mcpserver: invalid url: " + weburl.Reason(e.Reason).Text()
}

// IsInvalidURLError reports whether err's tree contains an *InvalidURLError.
func IsInvalidURLError(err error) bool {
	_, ok := errors.AsType[*InvalidURLError](err)
	return ok
}

// InUseError reports that a published agent version grants tools of the server.
type InUseError struct{}

func (*InUseError) Error() string { return "mcpserver: server is used by a published agent version" }

// IsInUseError reports whether err's tree contains an *InUseError.
func IsInUseError(err error) bool {
	_, ok := errors.AsType[*InUseError](err)
	return ok
}

// UnreachableError reports that the server could not be reached or did not answer as an MCP server.
type UnreachableError struct{ Cause error }

func (e *UnreachableError) Error() string {
	if e.Cause == nil {
		return "mcpserver: server unreachable"
	}
	return "mcpserver: server unreachable: " + e.Cause.Error()
}

func (e *UnreachableError) Unwrap() error { return e.Cause }

// IsUnreachableError reports whether err's tree contains an *UnreachableError.
func IsUnreachableError(err error) bool {
	_, ok := errors.AsType[*UnreachableError](err)
	return ok
}

// AuthRefusedError reports that the server answered the probe 401 or 403: it refused the credentials; its body is never kept.
type AuthRefusedError struct{ Status int }

func (e *AuthRefusedError) Error() string {
	return "mcpserver: server refused the credentials: status=" + strconv.Itoa(e.Status)
}

// IsAuthRefusedError reports whether err's tree contains an *AuthRefusedError.
func IsAuthRefusedError(err error) bool {
	_, ok := errors.AsType[*AuthRefusedError](err)
	return ok
}

// SecretReason is the stable cause of a SecretRequiredError, sent to clients as meta.reason.
type SecretReason string

// The causes of a SecretRequiredError.
const (
	SecretValueRequired SecretReason = "secret_required"
	SecretHeaderName    SecretReason = "header_name"
)

// SecretRequiredError reports a missing credential, or a header name that is empty, not a token or reserved.
type SecretRequiredError struct{ Reason SecretReason }

func (e *SecretRequiredError) Error() string {
	if e.Reason == SecretHeaderName {
		return "mcpserver: invalid auth header name"
	}
	return "mcpserver: auth value required"
}

// IsSecretRequiredError reports whether err's tree contains a *SecretRequiredError.
func IsSecretRequiredError(err error) bool {
	_, ok := errors.AsType[*SecretRequiredError](err)
	return ok
}

// SecretsUnavailableError reports that credentials cannot be sealed or opened because no encryption key is configured.
type SecretsUnavailableError struct{ Cause error }

func (e *SecretsUnavailableError) Error() string {
	msg := "mcpserver: secrets unavailable: configure an encryption key"
	if e.Cause == nil {
		return msg
	}
	return msg + ": " + e.Cause.Error()
}

func (e *SecretsUnavailableError) Unwrap() error { return e.Cause }

// IsSecretsUnavailableError reports whether err's tree contains a *SecretsUnavailableError.
func IsSecretsUnavailableError(err error) bool {
	_, ok := errors.AsType[*SecretsUnavailableError](err)
	return ok
}

// IsExpectedError reports whether err is a typed domain error a service returns as-is.
func IsExpectedError(err error) bool {
	return IsNotFoundError(err) || IsAlreadyExistsError(err) || IsInvalidURLError(err) || IsInUseError(err) ||
		IsUnreachableError(err) || IsAuthRefusedError(err) || IsSecretRequiredError(err) || IsSecretsUnavailableError(err)
}

// ToAppError maps NotFoundError to its own code.
func (e *NotFoundError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeMcpServerNotFound, "MCP server not found").WithCause(e)
}

// ToAppError maps AlreadyExistsError to its own code with the name.
func (e *AlreadyExistsError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeMcpServerAlreadyExists, "MCP server name already taken").
		WithMeta(map[string]string{"name": apperror.Echo(e.Name)}).WithCause(e)
}

// ToAppError maps InvalidURLError to its own code with the reason and the length limit.
func (e *InvalidURLError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeMcpServerInvalidURL, "MCP server URL "+weburl.Reason(e.Reason).Text()).
		WithMeta(map[string]string{"reason": string(e.Reason), "max": strconv.Itoa(MaxURLLen)}).WithCause(e)
}

// ToAppError maps InUseError to its own code.
func (e *InUseError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeMcpServerInUse, "MCP server is used by a published agent version").WithCause(e)
}

// ToAppError maps UnreachableError to its own code; the cause stays server-side.
func (e *UnreachableError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeMcpServerUnreachable, "MCP server could not be reached").WithCause(e)
}

// ToAppError maps AuthRefusedError to its own code.
func (e *AuthRefusedError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeMcpServerAuthRefused, "The MCP server refused the credentials").WithCause(e)
}

// ToAppError maps SecretRequiredError to its own code with the reason.
func (e *SecretRequiredError) ToAppError() *apperror.AppError {
	msg := "MCP server auth value is required"
	if e.Reason == SecretHeaderName {
		msg = "MCP server auth header name is invalid"
	}
	return apperror.New(apperror.CodeMcpServerSecretRequired, msg).
		WithMeta(map[string]string{"reason": string(e.Reason)}).WithCause(e)
}

// ToAppError maps SecretsUnavailableError to its own code.
func (e *SecretsUnavailableError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeMcpServerSecretsUnavailable, "MCP server secrets are unavailable").WithCause(e)
}
