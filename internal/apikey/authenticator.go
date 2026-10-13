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
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/scope"
)

// Authenticator verifies an API key and builds its Principal.
type Authenticator struct {
	store      Reader
	usage      *UsageWorker
	scheme     Scheme
	owners     OwnerRoles
	unexpected apperror.UnexpectedFunc
	now        func() time.Time
}

var _ authn.Authenticator = (*Authenticator)(nil)

// NewAuthenticator binds an Authenticator to its store, usage recorder, scheme and owner roles; usage and unexpected may be nil.
func NewAuthenticator(
	store Reader,
	usage *UsageWorker,
	scheme Scheme,
	owners OwnerRoles,
	unexpected apperror.UnexpectedFunc,
) *Authenticator {
	return &Authenticator{
		store:      store,
		usage:      usage,
		scheme:     scheme,
		owners:     owners,
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
	if k.Kind == KindPersonal {
		if p, err = a.actAsOwner(ctx, k, p); err != nil {
			return session.Principal{}, err
		}
	}
	if a.usage != nil {
		a.usage.Record(k.ID, now)
	}
	return p, nil
}

// SECURITY: a personal key never exceeds its owner's current role, and dies when the owner leaves the org.
func (a *Authenticator) actAsOwner(ctx context.Context, k *APIKey, p session.Principal) (session.Principal, error) {
	if a.owners == nil || k.CreatedBy == uuid.Nil {
		return session.Principal{}, &authn.UnauthorizedError{}
	}
	role, ok, err := a.owners.Role(ctx, k.OrgID, k.CreatedBy)
	if err != nil {
		if a.unexpected != nil {
			_ = a.unexpected(ctx, "apikey: owner role", err)
		}
		return session.Principal{}, &authn.UnauthorizedError{}
	}
	if !ok {
		return session.Principal{}, &authn.UnauthorizedError{}
	}
	granted := member.BuiltinPermissions(role)
	p.Scopes = slices.DeleteFunc(p.Scopes, func(s string) bool { return !scope.Grants(granted, s) })
	p.OwnerID = k.CreatedBy
	return p, nil
}

// SECURITY: a key is never an operator, and person-only scopes left in an old row are dropped.
func principalFor(k *APIKey, now time.Time) session.Principal {
	return session.Principal{
		UserID:          uuid.Nil,
		KeyID:           k.ID,
		KeyHint:         k.SecretHint,
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
