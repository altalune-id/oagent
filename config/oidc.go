package config

import (
	"net/url"
	"slices"
	"strings"
)

// OIDCConfig is the one OpenID Connect provider people sign in through; an empty Issuer turns it off.
type OIDCConfig struct {
	Issuer       string   `yaml:"issuer"       mapstructure:"issuer"       validate:"omitempty,url"`
	ClientID     string   `yaml:"clientID"     mapstructure:"clientID"`
	ClientSecret string   `yaml:"clientSecret" mapstructure:"clientSecret" awareness:"secret"`
	Scopes       []string `yaml:"scopes"       mapstructure:"scopes"`
	ButtonLabel  string   `yaml:"buttonLabel"  mapstructure:"buttonLabel"`
}

// Enabled reports whether a provider is configured.
func (c OIDCConfig) Enabled() bool { return c.Issuer != "" }

// NormalizedIssuer is Issuer without a trailing slash, the key every account link is stored under.
func (c OIDCConfig) NormalizedIssuer() string { return strings.TrimSuffix(c.Issuer, "/") }

// NormalizeScopes trims each scope, as an env list like "openid, email" leaves spaces.
func NormalizeScopes(scopes []string) []string {
	out := make([]string, 0, len(scopes))
	for _, s := range scopes {
		out = append(out, strings.TrimSpace(s))
	}
	return out
}

func (c *Config) validateOIDC() error {
	o := c.OIDC
	if !o.Enabled() {
		return nil
	}
	if o.ClientID == "" {
		return &MissingSettingError{Keys: "oidc.clientID", Reason: "oidc.issuer is set"}
	}
	scopes := NormalizeScopes(o.Scopes)
	if slices.Contains(scopes, "") {
		return &InvalidSettingError{Key: "oidc.scopes", Reason: "must not hold an empty scope"}
	}
	if !slices.Contains(scopes, "openid") || !slices.Contains(scopes, "email") {
		return &InvalidSettingError{Key: "oidc.scopes", Reason: "must include openid and email"}
	}
	if strings.TrimSpace(o.ButtonLabel) == "" {
		return &MissingSettingError{Keys: "oidc.buttonLabel", Reason: "oidc.issuer is set"}
	}
	if c.Env == EnvDevelopment {
		return nil
	}
	if u, err := url.Parse(o.Issuer); err != nil || u.Scheme != "https" {
		return &InvalidSettingError{Key: "oidc.issuer", Reason: "must be https in production"}
	}
	if c.HTTP.BaseURL == "" {
		return &MissingSettingError{Keys: "http.baseURL", Reason: "the OIDC redirect URI is built from it"}
	}
	return nil
}
