package member

import (
	"context"

	"github.com/google/uuid"

	"altalune.id/oagent/mailer"
)

// Sessions ends a person's sessions.
type Sessions interface {
	DeleteByUser(ctx context.Context, userID uuid.UUID) error
}

// PersonalKeys revokes the personal API keys a person owns in one org.
type PersonalKeys interface {
	RevokePersonalOf(ctx context.Context, orgID, userID uuid.UUID) (int, error)
}

// People looks up how to reach a person.
type People interface {
	Contact(ctx context.Context, userID uuid.UUID) (email, name string, err error)
}

// Orgs looks up an org's display name.
type Orgs interface {
	Name(ctx context.Context, orgID uuid.UUID) (string, error)
}

// Directory finds the person and org an operator names by email and slug.
type Directory interface {
	UserIDByEmail(ctx context.Context, email string) (uuid.UUID, error)
	OrgIDBySlug(ctx context.Context, slug string) (uuid.UUID, error)
}

// Mail queues an email; it joins the transaction on ctx.
type Mail interface {
	Enqueue(ctx context.Context, m mailer.Message) (uuid.UUID, error)
}

// Effects are what a removal or role change does besides the membership row, inside the same transaction.
type Effects struct {
	Sessions Sessions
	Keys     PersonalKeys
	People   People
	Orgs     Orgs
	Mail     Mail
	From     string
	BaseURL  string
}
