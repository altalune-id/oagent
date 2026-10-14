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
	Mode       AuthMode      `yaml:"mode"       mapstructure:"mode"       validate:"required,oneof=local basic"`
	Basic      BasicConfig   `yaml:"basic"      mapstructure:"basic"`
	Genesis    GenesisConfig `yaml:"genesis"    mapstructure:"genesis"`
	SessionTTL time.Duration `yaml:"sessionTTL" mapstructure:"sessionTTL" validate:"gte=1h,lte=720h"`
}

// BasicConfig is the single credential accepted when auth.mode is basic.
type BasicConfig struct {
	User     string `yaml:"user"     mapstructure:"user"`
	Password string `yaml:"password" mapstructure:"password" awareness:"secret"`
}

// GenesisConfig installs the first owner from the environment on a headless selfhosted boot.
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
	g := c.Auth.Genesis
	if (g.Email == "") != (g.Password == "") {
		return &MissingSettingError{Keys: "auth.genesis.email, auth.genesis.password", Reason: "set both or neither"}
	}
	if g.Email != "" {
		if c.Mode == ModeCloud {
			return &GenesisInCloudError{}
		}
		if err := password.Check(g.Password, password.Hints{Email: g.Email}); err != nil {
			return fmt.Errorf("config: auth.genesis.password: %w", err)
		}
	}
	if err := c.validateSecrets(); err != nil {
		return err
	}
	if c.Mode == ModeCloud {
		return &CloudRequiresOIDCError{}
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
