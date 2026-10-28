package config

import (
	"fmt"
	"time"

	"altalune.id/oagent/password"
	"altalune.id/oagent/sealer"
)

const (
	minSecretLen = 32
	generateHint = "generate: openssl rand -hex 32"
)

// AuthMode selects how people sign in.
type AuthMode string

// The auth modes. Basic replaces sign-in pages with one shared HTTP Basic credential (selfhosted only).
const (
	AuthModeLocal AuthMode = "local"
	AuthModeBasic AuthMode = "basic"
)

// AuthConfig configures how people and machines sign in.
type AuthConfig struct {
	Mode       AuthMode       `yaml:"mode"       mapstructure:"mode"       validate:"required,oneof=local basic"`
	Basic      BasicConfig    `yaml:"basic"      mapstructure:"basic"`
	Genesis    GenesisConfig  `yaml:"genesis"    mapstructure:"genesis"`
	Password   PasswordConfig `yaml:"password"   mapstructure:"password"`
	SessionTTL time.Duration  `yaml:"sessionTTL" mapstructure:"sessionTTL" validate:"gte=1h,lte=720h"`
	// RememberLastUser keeps the last signed-in email in a cookie, to offer "Continue as …" on /login.
	RememberLastUser bool `yaml:"rememberLastUser" mapstructure:"rememberLastUser"`
}

// PasswordConfig turns console password sign-in on or off; the OIDC provider, when configured, is always on.
type PasswordConfig struct {
	// Enabled defaults to on in selfhosted and off in cloud, which refuses it on.
	Enabled *bool `yaml:"enabled" mapstructure:"enabled"`
}

// PasswordsOn reports whether console password sign-in is on: auth.password.enabled, else on in selfhosted and off in cloud.
func (c *Config) PasswordsOn() bool {
	if c.Auth.Password.Enabled != nil {
		return *c.Auth.Password.Enabled
	}
	return c.Mode != ModeCloud
}

// OpenSignup reports whether any verified OIDC sign-in may create an account with no org: cloud only.
func (c *Config) OpenSignup() bool { return c.Mode == ModeCloud }

// BasicConfig is the single credential accepted when auth.mode is basic.
type BasicConfig struct {
	User     string `yaml:"user"     mapstructure:"user"`
	Password string `yaml:"password" mapstructure:"password" awareness:"secret"`
}

// GenesisConfig installs the first owner from the environment on a headless selfhosted boot; with passwords off, only the email.
type GenesisConfig struct {
	Email    string `yaml:"email"    mapstructure:"email"    validate:"omitempty,email"`
	Password string `yaml:"password" mapstructure:"password" awareness:"secret"`
}

// InstallConfig configures the first-boot /install page.
type InstallConfig struct {
	Token string `yaml:"token" mapstructure:"token" awareness:"secret" validate:"omitempty,secret"`
}

func (c *Config) validateAuth() error {
	if c.Auth.Mode == AuthModeBasic {
		if c.Mode != ModeSelfhosted {
			return &AuthModeError{AuthMode: c.Auth.Mode, Mode: c.Mode}
		}
		if c.Auth.Basic.User == "" || c.Auth.Basic.Password == "" {
			return &MissingSettingError{Keys: "auth.basic.user, auth.basic.password", Reason: "auth.mode is basic"}
		}
		if err := password.Check(c.Auth.Basic.Password, password.Hints{Name: c.Auth.Basic.User}); err != nil {
			return fmt.Errorf("config: auth.basic.password: %w", err)
		}
	}
	if c.Mode == ModeCloud && !c.OIDC.Enabled() {
		return &CloudRequiresOIDCError{}
	}
	if c.Mode == ModeCloud && c.PasswordsOn() {
		return &InvalidSettingError{Key: "auth.password.enabled", Reason: "cloud signs people in through the OIDC provider only"}
	}
	if !c.PasswordsOn() && !c.OIDC.Enabled() {
		return &InvalidSettingError{Key: "auth.password.enabled", Reason: "off leaves no way to sign in; set oidc.issuer"}
	}
	if err := c.validateGenesis(); err != nil {
		return err
	}
	return c.validateSecrets()
}

func (c *Config) validateGenesis() error {
	g := c.Auth.Genesis
	switch {
	case g.Email == "" && g.Password == "":
		return nil
	case c.Mode == ModeCloud:
		return &GenesisInCloudError{}
	case !c.PasswordsOn() && g.Password != "":
		return &InvalidSettingError{Key: "auth.genesis.password", Reason: "auth.password.enabled is off, so genesis takes only the email"}
	case !c.PasswordsOn():
		return nil
	case g.Email == "" || g.Password == "":
		return &MissingSettingError{Keys: "auth.genesis.email, auth.genesis.password", Reason: "set both or neither"}
	}
	if err := password.Check(g.Password, password.Hints{Email: g.Email}); err != nil {
		return fmt.Errorf("config: auth.genesis.password: %w", err)
	}
	return nil
}

func (c *Config) validateSecrets() error {
	if c.Env == EnvDevelopment || c.Auth.Mode != AuthModeLocal {
		return nil
	}
	if c.HTTP.StateSecret == "" {
		return &MissingSettingError{Keys: "http.stateSecret", Reason: "required in production; " + generateHint}
	}
	if c.Secrets.EncryptionKey == "" {
		return &MissingSettingError{Keys: "secrets.encryptionKey", Reason: "required in production; " + generateHint}
	}
	return checkEncryptionKey(c.Secrets.EncryptionKey)
}

func checkEncryptionKey(key string) error {
	if _, err := sealer.ParseKey(key); err != nil {
		return &InvalidSettingError{Key: "secrets.encryptionKey", Reason: "must be 32 bytes as hex or base64; " + generateHint}
	}
	return nil
}
