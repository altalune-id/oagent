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

// OrgGetter loads an org by id and says whether this deployment lets callers create orgs.
type OrgGetter interface {
	Get(ctx context.Context, id uuid.UUID) (*org.Org, error)
	CreationAllowed() bool
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
	Identities Identities
	Invites    OIDCInvites
	OrgCreator OrgCreator
	// Issuer is the normalized oidc.issuer every account link is stored under.
	Issuer string
	// OpenSignup lets a verified OIDC sign-in with no account create one (cloud); otherwise it needs an account already.
	OpenSignup bool
}

// Seat is a usable invite an OIDC sign-in may claim: who it is for and which org it joins.
type Seat struct {
	ID    uuid.UUID
	OrgID uuid.UUID
	Email string
}

// OIDCInvites finds and uses up the invites an OIDC sign-in may claim; boot adapts invite.Service to it.
type OIDCInvites interface {
	// Seat returns the usable invite with publicID, or nil when it is unknown, used, revoked or expired.
	Seat(ctx context.Context, publicID string) (*Seat, error)
	// Claim uses seat up for userID and adds them to its org; false when it is no longer usable or not for their email.
	Claim(ctx context.Context, seat *Seat, userID uuid.UUID) (bool, error)
}

// OrgCreator creates an org; with CreatedBy set, its creator owns it, once per person (org.Service and boot's after-create hook).
type OrgCreator interface {
	Create(ctx context.Context, req org.CreateRequest) (*org.Org, error)
}

// Identities finds, links and creates the accounts an OIDC sign-in names; user.Service is it.
type Identities interface {
	ByIDP(ctx context.Context, issuer, subject string) (*user.User, error)
	LinkIDP(ctx context.Context, id uuid.UUID, issuer, subject string) (bool, error)
	CreateLinked(ctx context.Context, email, name, issuer, subject string) (*user.User, error)
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
