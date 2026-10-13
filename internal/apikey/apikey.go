// Package apikey mints and manages the machine credentials clients present to oagent.
package apikey

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/scope"
)

// DefaultPrefix marks an oagent key when no prefix is configured.
const DefaultPrefix = "oagent_"

const (
	secretBytes   = 32
	secretHintLen = 4
	nameMaxLen    = 100
)

// MaxLifetime is the longest a newly minted key may live.
const MaxLifetime = 366 * 24 * time.Hour

// Scheme is the one prefix a deployment mints keys under and recognizes them by.
type Scheme struct{ prefix string }

// NewScheme returns the Scheme for prefix, falling back to DefaultPrefix when prefix is empty.
func NewScheme(prefix string) Scheme { return Scheme{prefix: prefix} }

// Prefix returns the literal every plaintext secret minted under this Scheme starts with.
func (sc Scheme) Prefix() string {
	if sc.prefix == "" {
		return DefaultPrefix
	}
	return sc.prefix
}

// Kind names what a key is bound to.
type Kind string

// The key kinds. A project key is bound to one project; org and personal keys reach the projects their grant names.
const (
	KindProject  Kind = "project"
	KindOrg      Kind = "org"
	KindPersonal Kind = "personal"
)

// ProjectGrant is the set of projects an org key reaches: every project of the org, or the named ones.
type ProjectGrant struct {
	All        bool
	ProjectIDs []uuid.UUID
}

func (g ProjectGrant) validate() error {
	if g.All && len(g.ProjectIDs) > 0 {
		return &GrantConflictError{}
	}
	if !g.All && len(g.ProjectIDs) == 0 {
		return &EmptyGrantError{}
	}
	return nil
}

// APIKey is a machine credential bound to one project, or to an org and the projects its grant names.
type APIKey struct {
	ID          uuid.UUID
	OrgID       uuid.UUID
	ProjectID   uuid.UUID
	Kind        Kind
	AllProjects bool
	ProjectIDs  []uuid.UUID
	Name        string
	SecretHash  [32]byte
	SecretHint  string
	Scopes      []string
	ResourceIDs []uuid.UUID
	CreatedBy   uuid.UUID
	CreatedAt   time.Time
	ExpiresAt   *time.Time
	RevokedAt   *time.Time
	LastUsedAt  *time.Time
}

// MintProject returns a new project key and its one-time plaintext secret.
func (sc Scheme) MintProject(
	orgID, projectID uuid.UUID,
	name string,
	scopes []string,
	resourceIDs []uuid.UUID,
	expiresAt *time.Time,
	now time.Time,
) (*APIKey, string, error) {
	for _, s := range scopes {
		if level, ok := scope.LevelOf(s); ok && level != scope.LevelProject {
			return nil, "", &ScopeLevelError{Scope: s}
		}
	}
	k, plaintext, err := sc.mint(orgID, name, scopes, expiresAt, now)
	if err != nil {
		return nil, "", err
	}
	k.Kind = KindProject
	k.ProjectID = projectID
	k.ResourceIDs = dedupe(resourceIDs)
	return k, plaintext, nil
}

// MintOrg returns a new org key reaching grant and its one-time plaintext secret.
func (sc Scheme) MintOrg(
	orgID uuid.UUID,
	name string,
	scopes []string,
	grant ProjectGrant,
	expiresAt *time.Time,
	now time.Time,
) (*APIKey, string, error) {
	if err := grant.validate(); err != nil {
		return nil, "", err
	}
	k, plaintext, err := sc.mint(orgID, name, scopes, expiresAt, now)
	if err != nil {
		return nil, "", err
	}
	k.Kind = KindOrg
	k.AllProjects = grant.All
	k.ProjectIDs = dedupe(grant.ProjectIDs)
	return k, plaintext, nil
}

// MintPersonal returns a new personal key acting for ownerID and its one-time plaintext secret.
func (sc Scheme) MintPersonal(
	orgID, ownerID uuid.UUID,
	name string,
	scopes []string,
	grant ProjectGrant,
	expiresAt *time.Time,
	now time.Time,
) (*APIKey, string, error) {
	k, plaintext, err := sc.MintOrg(orgID, name, scopes, grant, expiresAt, now)
	if err != nil {
		return nil, "", err
	}
	k.Kind = KindPersonal
	k.CreatedBy = ownerID
	return k, plaintext, nil
}

func (sc Scheme) mint(orgID uuid.UUID, name string, scopes []string, expiresAt *time.Time, now time.Time) (*APIKey, string, error) {
	if err := ValidateName(name); err != nil {
		return nil, "", err
	}
	for _, s := range scopes {
		if !scope.Valid(s) {
			return nil, "", &UnknownScopeError{Scope: s}
		}
		if !scope.Mintable(s) {
			return nil, "", &PersonOnlyScopeError{Scope: s}
		}
	}
	if err := validateLifetime(expiresAt, now); err != nil {
		return nil, "", err
	}
	buf := make([]byte, secretBytes)
	if _, err := rand.Read(buf); err != nil {
		return nil, "", err
	}
	plaintext := sc.Prefix() + base64.RawURLEncoding.EncodeToString(buf)
	return &APIKey{
		ID:         uuid.New(),
		OrgID:      orgID,
		Name:       strings.TrimSpace(name),
		SecretHash: sha256.Sum256([]byte(plaintext)),
		SecretHint: plaintext[len(plaintext)-secretHintLen:],
		Scopes:     slices.Clone(scopes),
		CreatedAt:  now,
		ExpiresAt:  expiresAt,
	}, plaintext, nil
}

func validateLifetime(expiresAt *time.Time, now time.Time) error {
	if expiresAt == nil {
		return &ExpiryRequiredError{}
	}
	if !expiresAt.After(now) {
		return &ExpiryInPastError{}
	}
	if expiresAt.Sub(now) > MaxLifetime {
		return &ExpiryTooLongError{}
	}
	return nil
}

// ValidateName reports whether name is usable as a key name.
func ValidateName(name string) error {
	n := strings.TrimSpace(name)
	if n == "" {
		return &InvalidNameError{Reason: NameRequired}
	}
	if len(n) > nameMaxLen {
		return &InvalidNameError{Reason: NameLength}
	}
	return nil
}

// HasGrant reports whether the key reaches projects through a grant rather than one bound project.
func (k *APIKey) HasGrant() bool { return k.Kind == KindOrg || k.Kind == KindPersonal }

// ReachableProjects returns the projects the key reaches when it is not granted all of them.
func (k *APIKey) ReachableProjects() []uuid.UUID {
	if k.HasGrant() {
		return slices.Clone(k.ProjectIDs)
	}
	return []uuid.UUID{k.ProjectID}
}

// Matches reports whether plaintext hashes to this key's secret, in constant time.
func (k *APIKey) Matches(plaintext string) bool {
	sum := sha256.Sum256([]byte(plaintext))
	return subtle.ConstantTimeCompare(sum[:], k.SecretHash[:]) == 1
}

// Usable reports whether the key is neither revoked nor expired at now.
func (k *APIKey) Usable(now time.Time) bool {
	if k.RevokedAt != nil && !k.RevokedAt.After(now.UTC()) {
		return false
	}
	if k.ExpiresAt != nil && !k.ExpiresAt.After(now.UTC()) {
		return false
	}
	return true
}

// Revoke marks the key revoked at now; the first revocation wins.
func (k *APIKey) Revoke(now time.Time) {
	if k.RevokedAt == nil {
		t := now.UTC()
		k.RevokedAt = &t
	}
}

func dedupe(ids []uuid.UUID) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id != uuid.Nil && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

type apiKeyRow struct {
	ID          uuid.UUID  `alias:"api_keys.id"`
	OrgID       uuid.UUID  `alias:"api_keys.org_id"`
	ProjectID   *uuid.UUID `alias:"api_keys.project_id"`
	Kind        string     `alias:"api_keys.kind"`
	AllProjects bool       `alias:"api_keys.all_projects"`
	ProjectIDs  string     `alias:"api_keys.project_ids"`
	Name        string     `alias:"api_keys.name"`
	SecretHash  []byte     `alias:"api_keys.secret_hash"`
	SecretHint  string     `alias:"api_keys.secret_hint"`
	Scopes      string     `alias:"api_keys.scopes"`
	ResourceIDs string     `alias:"api_keys.resource_ids"`
	CreatedBy   *uuid.UUID `alias:"api_keys.created_by"`
	CreatedAt   time.Time  `alias:"api_keys.created_at"`
	ExpiresAt   *time.Time `alias:"api_keys.expires_at"`
	RevokedAt   *time.Time `alias:"api_keys.revoked_at"`
	LastUsedAt  *time.Time `alias:"api_keys.last_used_at"`
}

func (r *apiKeyRow) toKey() (*APIKey, error) {
	scopes, err := parsePgTextArray(r.Scopes)
	if err != nil {
		return nil, fmt.Errorf("apikey.postgres: parse scopes: %w", err)
	}
	resourceIDs, err := parsePgUUIDArray(r.ResourceIDs)
	if err != nil {
		return nil, fmt.Errorf("apikey.postgres: parse resource_ids: %w", err)
	}
	projectIDs, err := parsePgUUIDArray(r.ProjectIDs)
	if err != nil {
		return nil, fmt.Errorf("apikey.postgres: parse project_ids: %w", err)
	}
	var hash [32]byte
	copy(hash[:], r.SecretHash)
	k := &APIKey{
		ID:          r.ID,
		OrgID:       r.OrgID,
		Kind:        Kind(r.Kind),
		AllProjects: r.AllProjects,
		ProjectIDs:  projectIDs,
		Name:        r.Name,
		SecretHash:  hash,
		SecretHint:  r.SecretHint,
		Scopes:      scopes,
		ResourceIDs: resourceIDs,
		CreatedAt:   r.CreatedAt.UTC(),
	}
	if r.ProjectID != nil {
		k.ProjectID = *r.ProjectID
	}
	if r.CreatedBy != nil {
		k.CreatedBy = *r.CreatedBy
	}
	if r.ExpiresAt != nil {
		t := r.ExpiresAt.UTC()
		k.ExpiresAt = &t
	}
	if r.RevokedAt != nil {
		t := r.RevokedAt.UTC()
		k.RevokedAt = &t
	}
	if r.LastUsedAt != nil {
		t := r.LastUsedAt.UTC()
		k.LastUsedAt = &t
	}
	return k, nil
}
