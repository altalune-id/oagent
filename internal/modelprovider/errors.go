package modelprovider

import (
	"errors"
	"strconv"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/weburl"
)

// NotFoundError reports that no provider matched the lookup.
type NotFoundError struct{ ID string }

func (e *NotFoundError) Error() string {
	if e.ID == "" {
		return "modelprovider: provider not found"
	}
	return "modelprovider: provider not found: id=" + apperror.Echo(e.ID)
}

// IsNotFoundError reports whether err's tree contains a *NotFoundError.
func IsNotFoundError(err error) bool {
	_, ok := errors.AsType[*NotFoundError](err)
	return ok
}

// AlreadyExistsError reports that the project already has a provider with this name.
type AlreadyExistsError struct{ Name string }

func (e *AlreadyExistsError) Error() string {
	return "modelprovider: name already taken: " + apperror.Echo(e.Name)
}

// IsAlreadyExistsError reports whether err's tree contains an *AlreadyExistsError.
func IsAlreadyExistsError(err error) bool {
	_, ok := errors.AsType[*AlreadyExistsError](err)
	return ok
}

// URLReason is the stable cause of an InvalidBaseURLError, sent to clients as meta.reason.
type URLReason string

// The causes of an InvalidBaseURLError.
const (
	URLRequired       = URLReason(weburl.Required)
	URLTooLong        = URLReason(weburl.TooLong)
	URLNotUTF8        = URLReason(weburl.NotUTF8)
	URLUnparsable     = URLReason(weburl.Unparsable)
	URLHTTPSRequired  = URLReason(weburl.HTTPSRequired)
	URLHasCredentials = URLReason(weburl.HasCredentials)
	URLNoHost         = URLReason(weburl.NoHost)
)

// InvalidBaseURLError reports that the provider base URL breaks its invariants.
type InvalidBaseURLError struct{ Reason URLReason }

func (e *InvalidBaseURLError) Error() string {
	return "modelprovider: invalid base url: " + weburl.Reason(e.Reason).Text()
}

// IsInvalidBaseURLError reports whether err's tree contains an *InvalidBaseURLError.
func IsInvalidBaseURLError(err error) bool {
	_, ok := errors.AsType[*InvalidBaseURLError](err)
	return ok
}

// ModelsReason is the stable cause of an InvalidModelsError, sent to clients as meta.reason.
type ModelsReason string

// The causes of an InvalidModelsError.
const (
	ModelsRequired  ModelsReason = "required"
	ModelsTooMany   ModelsReason = "too_many"
	ModelsDuplicate ModelsReason = "duplicate"
	ModelsPrice     ModelsReason = "price"
)

func (r ModelsReason) text() string {
	switch r {
	case ModelsRequired:
		return "must list at least one model"
	case ModelsTooMany:
		return "must list at most " + strconv.Itoa(MaxModels) + " models"
	case ModelsDuplicate:
		return "must not list a model twice"
	case ModelsPrice:
		return "must not have a negative price"
	default:
		return "are invalid"
	}
}

// InvalidModelsError reports that the provider's model list breaks its invariants; Model names the offending model, if any.
type InvalidModelsError struct {
	Reason ModelsReason
	Model  string
}

func (e *InvalidModelsError) Error() string {
	if e.Model == "" {
		return "modelprovider: models " + e.Reason.text()
	}
	return "modelprovider: models " + e.Reason.text() + ": model=" + apperror.Echo(e.Model)
}

// IsInvalidModelsError reports whether err's tree contains an *InvalidModelsError.
func IsInvalidModelsError(err error) bool {
	_, ok := errors.AsType[*InvalidModelsError](err)
	return ok
}

// InUseReason is the stable cause of an InUseError, sent to clients as meta.reason.
type InUseReason string

// The causes of an InUseError.
const (
	InUseDelete       InUseReason = "delete"
	InUseModelRemoved InUseReason = "model_removed"
	InUsePriceRemoved InUseReason = "price_removed"
)

func (r InUseReason) text() string {
	switch r {
	case InUseDelete:
		return "cannot be deleted while an agent uses it"
	case InUseModelRemoved:
		return "cannot drop a model an agent uses"
	case InUsePriceRemoved:
		return "cannot drop the price of a model a capped agent uses"
	default:
		return "is in use"
	}
}

// InUseError reports that an agent's config depends on what the change would remove; Model names the model, if any.
type InUseError struct {
	Reason InUseReason
	Model  string
}

func (e *InUseError) Error() string {
	if e.Model == "" {
		return "modelprovider: provider " + e.Reason.text()
	}
	return "modelprovider: provider " + e.Reason.text() + ": model=" + apperror.Echo(e.Model)
}

// IsInUseError reports whether err's tree contains an *InUseError.
func IsInUseError(err error) bool {
	_, ok := errors.AsType[*InUseError](err)
	return ok
}

// SecretRequiredReason is the only cause of a SecretRequiredError, sent to clients as meta.reason.
const SecretRequiredReason = "secret_required"

// SecretRequiredError reports that an API key is needed, for a new provider or a changed base URL.
type SecretRequiredError struct{}

func (*SecretRequiredError) Error() string { return "modelprovider: api key required" }

// IsSecretRequiredError reports whether err's tree contains a *SecretRequiredError.
func IsSecretRequiredError(err error) bool {
	_, ok := errors.AsType[*SecretRequiredError](err)
	return ok
}

// UnknownModelError reports that the provider lists no model with this id.
type UnknownModelError struct{ Model string }

func (e *UnknownModelError) Error() string {
	return "modelprovider: unknown model: " + apperror.Echo(e.Model)
}

// IsUnknownModelError reports whether err's tree contains an *UnknownModelError.
func IsUnknownModelError(err error) bool {
	_, ok := errors.AsType[*UnknownModelError](err)
	return ok
}

// SecretsUnavailableError reports that API keys cannot be sealed or opened because no encryption key is configured.
type SecretsUnavailableError struct{ Cause error }

func (e *SecretsUnavailableError) Error() string {
	msg := "modelprovider: secrets unavailable: configure an encryption key"
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
	return IsNotFoundError(err) || IsAlreadyExistsError(err) || IsInvalidBaseURLError(err) ||
		IsInvalidModelsError(err) || IsInUseError(err) || IsSecretRequiredError(err) ||
		IsUnknownModelError(err) || IsSecretsUnavailableError(err)
}

// ToAppError maps NotFoundError to its own code.
func (e *NotFoundError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeModelProviderNotFound, "Model provider not found").WithCause(e)
}

// ToAppError maps AlreadyExistsError to its own code with the name.
func (e *AlreadyExistsError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeModelProviderAlreadyExists, "Model provider name already taken").
		WithMeta(map[string]string{"name": apperror.Echo(e.Name)}).WithCause(e)
}

// ToAppError maps InvalidBaseURLError to its own code with the reason and the length limit.
func (e *InvalidBaseURLError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeModelProviderInvalidBaseURL, "Model provider base URL "+weburl.Reason(e.Reason).Text()).
		WithMeta(map[string]string{"reason": string(e.Reason), "max": strconv.Itoa(MaxBaseURLLen)}).
		WithCause(e)
}

// ToAppError maps InvalidModelsError to its own code with the reason and the model limit.
func (e *InvalidModelsError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeModelProviderInvalidModels, "Model provider models "+e.Reason.text()).
		WithMeta(map[string]string{"reason": string(e.Reason), "max": strconv.Itoa(MaxModels)}).
		WithCause(e)
}

// ToAppError maps InUseError to its own code with the reason.
func (e *InUseError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeModelProviderInUse, "Model provider "+e.Reason.text()).
		WithMeta(map[string]string{"reason": string(e.Reason)}).WithCause(e)
}

// ToAppError maps SecretRequiredError to its own code with its reason.
func (e *SecretRequiredError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeModelProviderSecretRequired, "Model provider API key is required").
		WithMeta(map[string]string{"reason": SecretRequiredReason}).WithCause(e)
}

// ToAppError maps UnknownModelError to its own code with the model id.
func (e *UnknownModelError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeModelProviderUnknownModel, "Model provider has no such model").
		WithMeta(map[string]string{"model": apperror.Echo(e.Model)}).WithCause(e)
}

// ToAppError maps SecretsUnavailableError to its own code.
func (e *SecretsUnavailableError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeModelProviderSecretsUnavailable, "Model provider secrets are unavailable").WithCause(e)
}
