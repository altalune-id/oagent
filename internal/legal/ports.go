package legal

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/user"
)

// TermsAcceptor records that a person accepted the terms.
type TermsAcceptor interface {
	AcceptTerms(ctx context.Context, id uuid.UUID) (*user.User, error)
}

// SessionRotator replaces the request's browser session with one for p.
type SessionRotator interface {
	Rotate(ctx context.Context, w http.ResponseWriter, r *http.Request, p session.Principal) error
}
