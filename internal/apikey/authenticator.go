package apikey

import (
	"context"
	"crypto/sha256"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/authn"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/scope"
)

// Authenticator verifies an API key and builds its Principal.
type Authenticator struct {
	store      Reader
	usage      *UsageWorker
	scheme     Scheme
	unexpected apperror.UnexpectedFunc
	now        func() time.Time
}

var _ authn.Authenticator = (*Authenticator)(nil)

// NewAuthenticator binds an Authenticator to its store, usage recorder and scheme; usage and unexpected may be nil.
func NewAuthenticator(
	store Reader,
	usage *UsageWorker,
	scheme Scheme,
	unexpected apperror.UnexpectedFunc,
) *Authenticator {
	return &Authenticator{
		store:      store,
		usage:      usage,
		scheme:     scheme,
		unexpected: unexpected,
		now:        func() time.Time { return time.Now().UTC() },
	}
}

// Authenticate implements authn.Authenticator. SECURITY: every rejection is one opaque *authn.UnauthorizedError.
func (a *Authenticator) Authenticate(ctx context.Context, c authn.Credential) (session.Principal, error) {
	if c.Kind != authn.KindAPIKey || !strings.HasPrefix(c.Token, a.scheme.Prefix()) {
		return session.Principal{}, &authn.UnauthorizedError{}
	}
	now := a.now()
	k, err := a.store.BySecretHash(ctx, sha256.Sum256([]byte(c.Token)))
	if err != nil {
		if !IsNotFoundError(err) && a.unexpected != nil {
			_ = a.unexpected(ctx, "apikey: authenticate", err)
		}
		return session.Principal{}, &authn.UnauthorizedError{}
	}
	if !k.Usable(now) {
		return session.Principal{}, &authn.UnauthorizedError{}
	}
	p := principalFor(k, now)
	if a.usage != nil {
		a.usage.Record(k.ID, now)
	}
	return p, nil
}

// SECURITY: a key is never an operator, and person-only scopes left in an old row are dropped.
func principalFor(k *APIKey, now time.Time) session.Principal {
	return session.Principal{
		UserID:          uuid.Nil,
		KeyID:           k.ID,
		Name:            k.Name,
		Source:          session.SourceAPIKey,
		Scopes:          mintableOnly(k.Scopes),
		ActiveOrgID:     k.OrgID,
		ActiveProjectID: k.ProjectID,
		ProjectIDs:      k.ReachableProjects(),
		AllProjects:     k.AllProjects,
		ResourceIDs:     slices.Clone(k.ResourceIDs),
		IssuedAt:        now,
	}
}

func mintableOnly(scopes []string) []string {
	out := make([]string, 0, len(scopes))
	for _, s := range scopes {
		if scope.Mintable(s) {
			out = append(out, s)
		}
	}
	return out
}
