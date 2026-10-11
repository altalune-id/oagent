package auth

import (
	"context"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/user"
)

// UserReader finds a user by email.
type UserReader interface {
	ByEmail(ctx context.Context, email string) (*user.User, error)
}

// MemberReader lists a user's memberships, oldest first.
type MemberReader interface {
	ListByUser(ctx context.Context, userID uuid.UUID) ([]*member.Membership, error)
}

// SystemOrgReader returns the deployment's system org.
type SystemOrgReader interface {
	System(ctx context.Context) (*org.Org, error)
}

// ProjectLister lists an org's projects, oldest first.
type ProjectLister interface {
	List(ctx context.Context, orgID uuid.UUID) ([]*project.Project, error)
}

// OrgGetter loads an org by id.
type OrgGetter interface {
	Get(ctx context.Context, id uuid.UUID) (*org.Org, error)
}

// UserGetter loads a user by id.
type UserGetter interface {
	ByID(ctx context.Context, id uuid.UUID) (*user.User, error)
}

// PasswordSetter replaces a user's password after the policy check.
type PasswordSetter interface {
	SetPassword(ctx context.Context, id uuid.UUID, plain string) error
}

// CLIProfiles is the CLI's store of signed-in profiles; internal/cli implements it over cli.sessionFile.
type CLIProfiles interface {
	// Target names the selected profile and the server URL a login signs in to.
	Target(cmd *cobra.Command) (profile, url string, err error)
	// Current returns the selected profile's session, or nil when it has none.
	Current(cmd *cobra.Command) (profile string, s *CLISession, err error)
	Save(cmd *cobra.Command, s CLISession) error
	Remove(cmd *cobra.Command) error
}

// Deps are the ports a Service reads from.
type Deps struct {
	Users     UserReader
	Members   MemberReader
	Orgs      SystemOrgReader
	Projects  ProjectLister
	Sessions  session.Store
	Passwords PasswordSetter
	Basic     BasicCredentials
}
