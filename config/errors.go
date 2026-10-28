package config

import (
	"errors"
	"fmt"
)

// AuthModeError reports an auth.mode the deployment mode does not allow.
type AuthModeError struct {
	AuthMode AuthMode
	Mode     Mode
}

func (e *AuthModeError) Error() string {
	return fmt.Sprintf("config: auth.mode %q is not allowed with mode %q", e.AuthMode, e.Mode)
}

// IsAuthModeError reports whether err's tree contains an *AuthModeError.
func IsAuthModeError(err error) bool {
	_, ok := errors.AsType[*AuthModeError](err)
	return ok
}

// MissingSettingError reports settings that must be present together or for a chosen option.
type MissingSettingError struct{ Keys, Reason string }

func (e *MissingSettingError) Error() string {
	return fmt.Sprintf("config: %s required: %s", e.Keys, e.Reason)
}

// IsMissingSettingError reports whether err's tree contains a *MissingSettingError.
func IsMissingSettingError(err error) bool {
	_, ok := errors.AsType[*MissingSettingError](err)
	return ok
}

// InvalidSettingError reports a setting whose value is present but unusable.
type InvalidSettingError struct{ Key, Reason string }

func (e *InvalidSettingError) Error() string {
	return fmt.Sprintf("config: %s: %s", e.Key, e.Reason)
}

// IsInvalidSettingError reports whether err's tree contains an *InvalidSettingError.
func IsInvalidSettingError(err error) bool {
	_, ok := errors.AsType[*InvalidSettingError](err)
	return ok
}

// GenesisInCloudError reports env genesis configured in cloud mode, where the first operator installs through /install.
type GenesisInCloudError struct{}

func (*GenesisInCloudError) Error() string {
	return "config: auth.genesis is selfhosted only; cloud installs through /install"
}

// IsGenesisInCloudError reports whether err's tree contains a *GenesisInCloudError.
func IsGenesisInCloudError(err error) bool {
	_, ok := errors.AsType[*GenesisInCloudError](err)
	return ok
}

// CloudRequiresOIDCError reports cloud mode without an OIDC provider, which lands in auth phase P3.
type CloudRequiresOIDCError struct{}

func (*CloudRequiresOIDCError) Error() string {
	return "config: mode cloud requires OIDC sign-in, which is not available yet (auth phase P3); use mode selfhosted"
}

// IsCloudRequiresOIDCError reports whether err's tree contains a *CloudRequiresOIDCError.
func IsCloudRequiresOIDCError(err error) bool {
	_, ok := errors.AsType[*CloudRequiresOIDCError](err)
	return ok
}

// RLSBypassError reports db.rls.allowBypass outside development: production and cloud always enforce row-level security.
type RLSBypassError struct{}

func (*RLSBypassError) Error() string {
	return "config: db.rls.allowBypass is refused in production and in cloud mode; log in as the service role instead"
}

// IsRLSBypassError reports whether err's tree contains a *RLSBypassError.
func IsRLSBypassError(err error) bool {
	_, ok := errors.AsType[*RLSBypassError](err)
	return ok
}
