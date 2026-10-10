// Package capabilities derives the feature switches surfaces check instead of reading the deployment mode.
package capabilities

import "altalune.id/oagent/config"

// Capabilities is the read-only switch snapshot derived from Config.
type Capabilities struct {
	LocalLogin   bool
	BasicAuth    bool
	OrgCreation  bool
	EnvGenesis   bool
	PersonalKeys bool
}

// From derives Capabilities from a validated Config.
func From(c *config.Config) Capabilities {
	selfhosted := c.Mode == config.ModeSelfhosted
	basic := c.Auth.Mode == config.AuthModeBasic
	return Capabilities{
		LocalLogin:   selfhosted && !basic,
		BasicAuth:    selfhosted && basic,
		OrgCreation:  c.Mode == config.ModeCloud,
		EnvGenesis:   selfhosted && !basic && c.Auth.Genesis.Email != "",
		PersonalKeys: !basic,
	}
}
