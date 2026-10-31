package tokens

import "time"

// Config is one resource server at the OIDC provider; boot maps it from oidc.issuer and mcp.audience and mcp.clockSkew.
type Config struct {
	Issuer    string
	Audience  string
	ClockSkew time.Duration
}
