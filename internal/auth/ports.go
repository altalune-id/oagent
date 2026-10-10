package auth

import (
	"context"

	"github.com/google/uuid"

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

// Deps are the ports a Service reads from.
type Deps struct {
	Users    UserReader
	Members  MemberReader
	Orgs     SystemOrgReader
	Projects ProjectLister
	Sessions session.Store
	Basic    BasicCredentials
}
