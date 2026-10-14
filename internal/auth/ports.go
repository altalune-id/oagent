package auth

import (
	"context"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/user"
	"altalune.id/oagent/mailer"
)

// UserReader finds a user by email: ByEmail takes a normalized address, FindByEmail one an operator typed.
type UserReader interface {
	ByEmail(ctx context.Context, email string) (*user.User, error)
	FindByEmail(ctx context.Context, raw string) (*user.User, error)
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

// ProjectGetter loads a project by id.
type ProjectGetter interface {
	Get(ctx context.Context, id uuid.UUID) (*project.Project, error)
}

// PublicIDs turns the ids a Principal holds into the public ids a surface shows.
type PublicIDs interface {
	UserPublicID(ctx context.Context, id uuid.UUID) (string, error)
	OrgPublicID(ctx context.Context, id uuid.UUID) (string, error)
	ProjectPublicID(ctx context.Context, id uuid.UUID) (string, error)
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

// ProfileWriter loads a person and changes their name.
type ProfileWriter interface {
	ByID(ctx context.Context, id uuid.UUID) (*user.User, error)
	SetName(ctx context.Context, id uuid.UUID, name string) (*user.User, error)
}

// Deps are the ports a Service reads from; the profile changes use Profiles, Transactor and Mail.
type Deps struct {
	Users      UserReader
	Members    MemberReader
	Orgs       SystemOrgReader
	Projects   ProjectLister
	Sessions   session.Store
	Passwords  PasswordSetter
	Basic      BasicCredentials
	Profiles   ProfileWriter
	Transactor db.Transactor
	Mail       MailQueue
	From       string
	BaseURL    string
}

// ResetUsers finds the person a reset is for.
type ResetUsers interface {
	ByEmail(ctx context.Context, email string) (*user.User, error)
	ByID(ctx context.Context, id uuid.UUID) (*user.User, error)
}

// UserSessions ends a person's sessions; it joins the transaction on ctx.
type UserSessions interface {
	DeleteByUser(ctx context.Context, userID uuid.UUID) error
	DeleteByUserExcept(ctx context.Context, userID uuid.UUID, keepSID string) error
}

// MailQueue queues an email; it joins the transaction on ctx.
type MailQueue interface {
	Enqueue(ctx context.Context, m mailer.Message) (uuid.UUID, error)
}
