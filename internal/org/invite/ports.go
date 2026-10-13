package invite

import (
	"context"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/org/member"
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

// People looks up how to reach a person.
type People interface {
	Contact(ctx context.Context, userID uuid.UUID) (email, name string, err error)
}
