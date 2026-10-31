package config

import (
	"net/url"
	"strings"
)

// OAuth reports whether MCP clients sign in through the OIDC provider: MCP is on and oidc.issuer is set.
func (c MCPConfig) OAuth(o OIDCConfig) bool { return c.Enabled && o.Enabled() }

// MountedMCP is the public URL of the MCP endpoint, or empty without http.baseURL.
func MountedMCP(h HTTPConfig) string {
	if h.BaseURL == "" {
		return ""
	}
	return strings.TrimSuffix(h.BaseURL, "/") + strings.TrimSuffix(h.BasePath, "/") + "/mcp"
}

func (c *Config) validateMCP() error {
	if !c.MCP.OAuth(c.OIDC) || c.MCP.Audience == "" {
		return nil
	}
	u, err := url.Parse(c.MCP.Audience)
	switch {
	case err != nil, !u.IsAbs(), u.Scheme != "http" && u.Scheme != "https", u.Host == "":
		return &InvalidSettingError{Key: "mcp.audience", Reason: "must be an absolute http(s) URL"}
	case strings.Contains(c.MCP.Audience, "#"):
		return &InvalidSettingError{Key: "mcp.audience", Reason: "must not carry a fragment"}
	}
	if mounted := MountedMCP(c.HTTP); !c.MCP.AudienceOverride && mounted != "" && c.MCP.Audience != mounted {
		return &InvalidSettingError{
			Key:    "mcp.audience",
			Reason: "must equal http.baseURL + http.basePath + /mcp; set mcp.audienceOverride behind a proxy that changes the public URL",
		}
	}
	return nil
}
