package agent

import (
	"errors"
	"fmt"
	"strconv"

	"altalune.id/oagent/apperror"
)

// NotFoundError signals a missing agent row.
type NotFoundError struct {
	ID   string
	Slug string
}

func (e *NotFoundError) Error() string {
	switch {
	case e == nil:
		return "agent: not found"
	case e.Slug != "":
		return fmt.Sprintf("agent: not found: slug=%q", e.Slug)
	case e.ID != "":
		return fmt.Sprintf("agent: not found: id=%s", e.ID)
	default:
		return "agent: not found"
	}
}

// IsNotFoundError reports whether err wraps a *NotFoundError.
func IsNotFoundError(err error) bool {
	_, ok := errors.AsType[*NotFoundError](err)
	return ok
}

// AlreadyExistsError signals a slug collision within a project.
type AlreadyExistsError struct {
	Slug string
}

func (e *AlreadyExistsError) Error() string {
	return fmt.Sprintf("agent: slug already exists: %q", e.Slug)
}

// IsAlreadyExistsError reports whether err wraps an *AlreadyExistsError.
func IsAlreadyExistsError(err error) bool {
	_, ok := errors.AsType[*AlreadyExistsError](err)
	return ok
}

// SlugReason is the stable cause of an InvalidSlugError, sent to clients as meta.reason.
type SlugReason string

// The causes of an InvalidSlugError.
const (
	SlugLength SlugReason = "length"
	SlugFormat SlugReason = "format"
)

// InvalidSlugError signals a slug that fails the aggregate invariants.
type InvalidSlugError struct {
	Slug   string
	Reason SlugReason
}

func (e *InvalidSlugError) Error() string {
	return "agent: invalid slug: " + e.text()
}

func (e *InvalidSlugError) text() string {
	switch e.Reason {
	case SlugLength:
		return fmt.Sprintf("must be %d to %d characters", slugMinLen, slugMaxLen)
	case SlugFormat:
		return "must use lowercase letters, digits and dashes"
	default:
		return fmt.Sprintf("%q is invalid", apperror.Echo(e.Slug))
	}
}

// IsInvalidSlugError reports whether err wraps an *InvalidSlugError.
func IsInvalidSlugError(err error) bool {
	_, ok := errors.AsType[*InvalidSlugError](err)
	return ok
}

// NameReason is the stable cause of an InvalidNameError, sent to clients as meta.reason.
type NameReason string

// NameLength is the one cause of an InvalidNameError.
const NameLength NameReason = "length"

// InvalidNameError signals a name that fails the aggregate invariants.
type InvalidNameError struct {
	Reason NameReason
}

func (e *InvalidNameError) Error() string {
	return fmt.Sprintf("agent: invalid name: must be %d to %d characters", nameMinLen, nameMaxLen)
}

// IsInvalidNameError reports whether err wraps an *InvalidNameError.
func IsInvalidNameError(err error) bool {
	_, ok := errors.AsType[*InvalidNameError](err)
	return ok
}

// DescriptionReason is the stable cause of an InvalidDescriptionError, sent to clients as meta.reason.
type DescriptionReason string

// DescriptionLength is the one cause of an InvalidDescriptionError.
const DescriptionLength DescriptionReason = "length"

// InvalidDescriptionError signals a description that fails the aggregate invariants.
type InvalidDescriptionError struct {
	Reason DescriptionReason
}

func (e *InvalidDescriptionError) Error() string {
	return fmt.Sprintf("agent: invalid description: must be at most %d characters", descMaxLen)
}

// IsInvalidDescriptionError reports whether err wraps an *InvalidDescriptionError.
func IsInvalidDescriptionError(err error) bool {
	_, ok := errors.AsType[*InvalidDescriptionError](err)
	return ok
}

// NeedsRepublishError reports a config from before provider grants, which must be republished with a provider before it runs.
type NeedsRepublishError struct{}

func (*NeedsRepublishError) Error() string {
	return "agent: config needs a provider and grants; edit and publish it again"
}

// IsNeedsRepublishError reports whether err's tree contains a *NeedsRepublishError.
func IsNeedsRepublishError(err error) bool {
	_, ok := errors.AsType[*NeedsRepublishError](err)
	return ok
}

// GrantReason is the stable cause of an InvalidGrantError, sent to clients as meta.reason.
type GrantReason string

// The causes of an InvalidGrantError.
const (
	GrantProvider            GrantReason = "provider"
	GrantModel               GrantReason = "model"
	GrantHelperModel         GrantReason = "helper_model"
	GrantServer              GrantReason = "server"
	GrantToolName            GrantReason = "tool_name"
	GrantLegacyTools         GrantReason = "legacy_tools"
	GrantBudgetUnpriced      GrantReason = "budget_unpriced"
	GrantKnowledgeSource     GrantReason = "knowledge_source"
	GrantUnapprovedTool      GrantReason = "unapproved_tool"
	GrantMemoryStore         GrantReason = "memory_store"
	GrantMemoryStoreDisabled GrantReason = "memory_store_disabled"
)

func (r GrantReason) text() string {
	switch r {
	case GrantProvider:
		return "names a model provider this project does not have"
	case GrantModel:
		return "names a model the provider does not list"
	case GrantHelperModel:
		return "names a helper model the provider does not list"
	case GrantServer:
		return "names an MCP server this project does not have"
	case GrantToolName:
		return "grants a tool whose prefixed name breaks the tool name rule"
	case GrantLegacyTools:
		return "uses the old top-level tool list; grant tools per MCP server"
	case GrantBudgetUnpriced:
		return "has a cost limit on a model without prices"
	case GrantKnowledgeSource:
		return "names a knowledge source this project does not have"
	case GrantMemoryStore:
		return "names a memory store this project does not have"
	case GrantMemoryStoreDisabled:
		return "names a memory store whose backend this deployment does not run"
	case GrantUnapprovedTool:
		return "grants a tool that runs without approval while the agent reads knowledge"
	default:
		return "is invalid"
	}
}

// InvalidGrantError reports a config reference or grant the project cannot honor; Name names the model, server or tool, if any.
type InvalidGrantError struct {
	Reason GrantReason
	Name   string
}

func (e *InvalidGrantError) Error() string {
	if e.Name == "" {
		return "agent: config " + e.Reason.text()
	}
	return "agent: config " + e.Reason.text() + ": " + apperror.Echo(e.Name)
}

// IsInvalidGrantError reports whether err's tree contains an *InvalidGrantError.
func IsInvalidGrantError(err error) bool {
	_, ok := errors.AsType[*InvalidGrantError](err)
	return ok
}

// IsExpectedError reports whether err is a typed domain error a service returns as-is.
func IsExpectedError(err error) bool {
	return IsNotFoundError(err) || IsAlreadyExistsError(err) || IsInvalidSlugError(err) ||
		IsInvalidNameError(err) || IsInvalidDescriptionError(err) || IsNeedsRepublishError(err) || IsInvalidGrantError(err)
}

// ToAppError maps NeedsRepublishError to its own code.
func (e *NeedsRepublishError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeAgentNeedsRepublish, "Agent config needs a provider; edit and publish it again").WithCause(e)
}

// ToAppError maps InvalidGrantError to its own code with the reason.
func (e *InvalidGrantError) ToAppError() *apperror.AppError {
	meta := map[string]string{"reason": string(e.Reason)}
	if e.Reason == GrantUnapprovedTool {
		meta["name"] = apperror.Echo(e.Name)
	}
	return apperror.New(apperror.CodeAgentInvalidGrant, "Agent config "+e.Reason.text()).WithMeta(meta).WithCause(e)
}

// ToAppError maps NotFoundError to the canonical NotFound envelope.
func (e *NotFoundError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeAgentNotFound, "Agent not found").WithCause(e)
}

// ToAppError maps AlreadyExistsError to the canonical AlreadyExists envelope.
func (e *AlreadyExistsError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeAgentAlreadyExists, "Agent slug already taken").
		WithMeta(map[string]string{"slug": apperror.Echo(e.Slug)}).WithCause(e)
}

// ToAppError maps InvalidSlugError to its own code with the slug, the reason and the length limits.
func (e *InvalidSlugError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeAgentInvalidSlug, "Agent slug "+e.text()).
		WithMeta(map[string]string{
			"slug": apperror.Echo(e.Slug), "reason": string(e.Reason),
			"min": strconv.Itoa(slugMinLen), "max": strconv.Itoa(slugMaxLen),
		}).WithCause(e)
}

// ToAppError maps InvalidNameError to its own code with the reason and the length limits.
func (e *InvalidNameError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeAgentInvalidName, fmt.Sprintf("Agent name must be %d to %d characters", nameMinLen, nameMaxLen)).
		WithMeta(map[string]string{"reason": string(e.Reason), "min": strconv.Itoa(nameMinLen), "max": strconv.Itoa(nameMaxLen)}).WithCause(e)
}

// ToAppError maps InvalidDescriptionError to its own code with the reason and the length limit.
func (e *InvalidDescriptionError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeAgentInvalidDescription, fmt.Sprintf("Agent description must be at most %d characters", descMaxLen)).
		WithMeta(map[string]string{"reason": string(e.Reason), "max": strconv.Itoa(descMaxLen)}).WithCause(e)
}
