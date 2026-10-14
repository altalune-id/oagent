// Package capabilities derives the feature switches surfaces check instead of reading the deployment mode.
package capabilities

// Capabilities is the read-only switch snapshot derived from a Config.
type Capabilities struct {
	LocalLogin   bool
	BasicAuth    bool
	OrgCreation  bool
	EnvGenesis   bool
	PersonalKeys bool
	// OperatorOrgSupport lets staff (an operator who is a person) manage, as an admin, the members of an org they do not belong to.
	OperatorOrgSupport bool
}

// From derives Capabilities from the deployment posture.
func From(c Config) Capabilities {
	local := c.Selfhosted && !c.BasicAuth
	return Capabilities{
		LocalLogin:         local,
		BasicAuth:          c.Selfhosted && c.BasicAuth,
		OrgCreation:        c.Cloud,
		EnvGenesis:         local && c.EnvGenesis,
		PersonalKeys:       !c.BasicAuth,
		OperatorOrgSupport: c.Cloud,
	}
}
