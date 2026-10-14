// Package session models the authenticated caller (Principal) and the store that keeps browser sessions.
package session

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Source names the credential a Principal came from.
type Source string

// The credential sources.
const (
	SourceLocal  Source = "local"
	SourceBasic  Source = "basic"
	SourceAPIKey Source = "apikey"
	SourceOIDC   Source = "oidc"
	SourceToken  Source = "token"
	SourceSystem Source = "system"
)

// System is the Principal of a trusted local process: `oagent mcp` over stdio, run by whoever holds the DB DSN.
func System() Principal {
	return Principal{Source: SourceSystem, Name: "stdio", Operator: true}
}

// Principal is the authenticated caller. SECURITY: Scopes, Operator and reach fields are rebuilt per request, never persisted.
type Principal struct {
	UserID          uuid.UUID
	KeyID           uuid.UUID
	KeyHint         string
	OwnerID         uuid.UUID
	Email           string
	Name            string
	AvatarURL       string
	Source          Source
	IDPIssuer       string
	IDPSubject      string
	IDToken         string
	Scopes          []string `json:"-"`
	Operator        bool     `json:"-"`
	ActiveOrgID     uuid.UUID
	ActiveProjectID uuid.UUID
	ProjectIDs      []uuid.UUID `json:"-"`
	AllProjects     bool        `json:"-"`
	ResourceIDs     []uuid.UUID `json:"-"`
	Locale          string
	TermsAcceptedAt time.Time
	IssuedAt        time.Time
}

// IsPerson reports whether p is a signed-in person rather than a key or the shared basic credential.
func (p Principal) IsPerson() bool {
	return p.UserID != uuid.Nil && (p.Source == SourceLocal || p.Source == SourceOIDC || p.Source == SourceToken)
}

func (p Principal) withoutAuthority() Principal {
	p.Scopes, p.Operator, p.ProjectIDs, p.AllProjects, p.ResourceIDs = nil, false, nil, false, nil
	return p
}

type principalKey struct{}

// PrincipalInto returns ctx carrying p.
func PrincipalInto(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalFrom returns the Principal in ctx, or the zero Principal when there is none.
func PrincipalFrom(ctx context.Context) Principal {
	p, _ := ctx.Value(principalKey{}).(Principal)
	return p
}

type sidKey struct{}

// SIDInto returns ctx carrying the id of the session the request came with.
func SIDInto(ctx context.Context, sid string) context.Context {
	return context.WithValue(ctx, sidKey{}, sid)
}

// SIDFrom returns the request's session id, if it came with one.
func SIDFrom(ctx context.Context) (string, bool) {
	sid, _ := ctx.Value(sidKey{}).(string)
	return sid, sid != ""
}

// Store keeps browser sessions by opaque id.
type Store interface {
	Save(ctx context.Context, sid string, p Principal, expiresAt time.Time) error
	Load(ctx context.Context, sid string) (Principal, bool, error)
	Delete(ctx context.Context, sid string) error
	DeleteByUser(ctx context.Context, userID uuid.UUID) error
	// DeleteByUserExcept removes every session of userID but keepSID.
	DeleteByUserExcept(ctx context.Context, userID uuid.UUID, keepSID string) error
	DeleteExpired(ctx context.Context) (int, error)
}
