package invite

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/auth"
	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/legal"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/user"
	"altalune.id/oagent/mailer"
)

// Members resolves who asks and adds the person who accepts; both join the transaction on ctx.
type Members interface {
	ActorFor(ctx context.Context, r member.Requester, orgID uuid.UUID) (member.Actor, error)
	AddFromInvite(ctx context.Context, orgID, userID uuid.UUID, role member.Role) (bool, error)
}

// Mail queues an email; it joins the transaction on ctx.
type Mail interface {
	Enqueue(ctx context.Context, m mailer.Message) (uuid.UUID, error)
}

// Orgs looks up an org's display name.
type Orgs interface {
	Name(ctx context.Context, orgID uuid.UUID) (string, error)
}

// OrgLinks looks up an org's slug.
type OrgLinks interface {
	Slug(ctx context.Context, orgID uuid.UUID) (string, error)
}

// Memberships tells whether a person already belongs to an org.
type Memberships interface {
	Find(ctx context.Context, orgID, userID uuid.UUID) (*member.Membership, error)
}

// SignIn signs a person in with the password they just chose, and ends a session.
type SignIn interface {
	LoginLocal(ctx context.Context, c auth.Credentials) (session.Principal, error)
	Logout(ctx context.Context, sid string) error
}

// OrgIDs resolves an org's public id to its id; an unknown public id is an *org.NotFoundError.
type OrgIDs interface {
	IDByPublicID(ctx context.Context, publicID string) (uuid.UUID, error)
}

// Sessions starts, reads and ends browser sessions.
type Sessions interface {
	Start(ctx context.Context, w http.ResponseWriter, p session.Principal) (string, time.Time, error)
	Read(r *http.Request) (string, bool)
	End(ctx context.Context, w http.ResponseWriter, sid string) error
}

// Users finds and creates the people who join through an invite; each call joins the transaction on ctx.
type Users interface {
	ByEmail(ctx context.Context, email string) (*user.User, error)
	Create(ctx context.Context, req user.CreateRequest) (*user.User, error)
}

// Terms records that a person accepted the current terms; it joins the transaction on ctx.
type Terms interface {
	Accept(ctx context.Context, userID uuid.UUID, source legal.Source) (time.Time, error)
}

// People looks up how to reach a person and their public id.
type People interface {
	Contact(ctx context.Context, userID uuid.UUID) (email, name string, err error)
	PublicID(ctx context.Context, userID uuid.UUID) (string, error)
}

// Events publishes an org event to its webhook subscribers; it joins the transaction on ctx.
type Events interface {
	EnqueueOrg(ctx context.Context, orgID uuid.UUID, t events.Type, data any) error
}
