package install

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/auth"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/legal"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/user"
)

// UserCreator creates the first owner.
type UserCreator interface {
	Create(ctx context.Context, req user.CreateRequest) (*user.User, error)
}

// Terms records that a person accepted the current terms; it joins the transaction on ctx.
type Terms interface {
	Accept(ctx context.Context, userID uuid.UUID, source legal.Source) (time.Time, error)
}

// SystemOrgs finds or creates the deployment's system org.
type SystemOrgs interface {
	System(ctx context.Context) (*org.Org, error)
	CreateSystem(ctx context.Context, req org.CreateRequest) (*org.Org, error)
}

// Owners adds the owner membership and counts owners.
type Owners interface {
	Add(ctx context.Context, orgID, userID uuid.UUID, role member.Role) (*member.Membership, error)
	CountOwners(ctx context.Context, orgID uuid.UUID) (int, error)
}

// DefaultProjects finds or creates an org's default project.
type DefaultProjects interface {
	BySlug(ctx context.Context, orgID uuid.UUID, slug string) (*project.Project, error)
	Create(ctx context.Context, req project.CreateRequest) (*project.Project, error)
}

// SignIn signs the new owner in after install.
type SignIn interface {
	LoginLocal(ctx context.Context, c auth.Credentials) (session.Principal, error)
}

// SessionStarter starts the new owner's browser session.
type SessionStarter interface {
	Start(ctx context.Context, w http.ResponseWriter, p session.Principal) (string, time.Time, error)
}
