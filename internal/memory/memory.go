// Package memory keeps a project's memory stores and the facts an agent remembers about one end user in each, behind driver-agnostic Facts.
package memory

import (
	"crypto/sha256"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/text/unicode/norm"

	"altalune.id/oagent/internal/kernel/runtime"
	"altalune.id/oagent/nanoid"
)

// Backend is the driver that keeps a store's facts.
type Backend string

// BackendNative keeps facts in Postgres.
const BackendNative Backend = "native"

// PublicIDLen is the length of a store's or a native fact's public id.
const PublicIDLen = 21

// MaxStoreNameLen bounds a store name, in characters.
const MaxStoreNameLen = 200

// MemoryStore is a named place in a project where agents keep what they remember about people.
type MemoryStore struct {
	ID        uuid.UUID
	PublicID  string
	OrgID     uuid.UUID
	ProjectID uuid.UUID
	Name      string
	Backend   Backend
	// RememberAPIUsers lets agents remember a named API user; off, only people the server can tell apart are remembered.
	RememberAPIUsers bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// StoreInput is what a person declares for a new store.
type StoreInput struct {
	Name             string
	Backend          Backend
	RememberAPIUsers bool
}

// Mode is how a store takes part in a run: whether its driver extracts facts itself, and whether named API users are remembered.
type Mode struct {
	Extracts, APIUsers bool
}

// NewStore validates in against the enabled backends and mints the store's ids.
func NewStore(orgID, projectID uuid.UUID, in StoreInput, enabled []Backend, now time.Time) (*MemoryStore, error) {
	name := strings.TrimSpace(in.Name)
	switch {
	case name == "":
		return nil, &InvalidStoreError{Reason: StoreNameRequired}
	case utf8.RuneCountInString(name) > MaxStoreNameLen:
		return nil, &InvalidStoreError{Reason: StoreNameTooLong}
	case !slices.Contains(enabled, in.Backend):
		return nil, &InvalidStoreError{Reason: StoreBackendDisabled}
	}
	publicID, err := nanoid.New(PublicIDLen)
	if err != nil {
		return nil, err
	}
	now = now.UTC()
	return &MemoryStore{
		ID: uuid.New(), PublicID: publicID, OrgID: orgID, ProjectID: projectID, Name: name, Backend: in.Backend,
		RememberAPIUsers: in.RememberAPIUsers, CreatedAt: now, UpdatedAt: now,
	}, nil
}

// MaxPerPerson caps the facts kept about one person for one agent in one store; the oldest go first.
const MaxPerPerson = 1000

// Scope is whose facts a call touches: one end user in one store of one project, and one agent unless AgentID is uuid.Nil.
type Scope struct {
	OrgID, ProjectID, StoreID, AgentID, EndUserID uuid.UUID
}

// Check refuses a scope with a nil org, project, store or end user, and a nil agent when needAgent is set.
func (s Scope) Check(needAgent bool) error {
	if err := s.CheckStore(); err != nil {
		return err
	}
	switch {
	case s.EndUserID == uuid.Nil:
		return &IncompleteScopeError{Field: "end_user"}
	case needAgent && s.AgentID == uuid.Nil:
		return &IncompleteScopeError{Field: "agent"}
	}
	return nil
}

// CheckStore refuses a scope with a nil org, project or store; the end user and agent are ignored.
func (s Scope) CheckStore() error {
	switch uuid.Nil {
	case s.OrgID:
		return &IncompleteScopeError{Field: "org"}
	case s.ProjectID:
		return &IncompleteScopeError{Field: "project"}
	case s.StoreID:
		return &IncompleteScopeError{Field: "store"}
	}
	return nil
}

// Fact is one remembered fact; ID is driver-issued and opaque. SECURITY: Text is the person's own words; never log it.
type Fact struct {
	ID          string
	Text        string
	CreatedAt   time.Time
	AgentID     uuid.UUID
	SourceRunID uuid.UUID
}

// Turn is what one run leg hands the driver: Facts for a driver that does not extract, UserTexts for one that does.
type Turn struct {
	RunID     uuid.UUID
	Facts     []string
	UserTexts []string
}

// CleanFacts trims each fact, drops blank ones, cuts each to runtime.MaxMemoryFactRunes and keeps the first runtime.MaxExtractedFacts.
func CleanFacts(in []string) []string {
	return runtime.Clean(in, runtime.MaxMemoryFactRunes, runtime.MaxExtractedFacts)
}

// CleanUserTexts trims each text, drops blank ones, cuts each to runtime.MaxUserTextRunes and keeps the first runtime.MaxUserTexts.
func CleanUserTexts(in []string) []string {
	return runtime.Clean(in, runtime.MaxUserTextRunes, runtime.MaxUserTexts)
}

// Hash is the SHA-256 of text after NFKC, lower case and collapsed whitespace, so a repeated fact is stored once.
func Hash(text string) []byte {
	sum := sha256.Sum256([]byte(strings.Join(strings.Fields(strings.ToLower(norm.NFKC.String(text))), " ")))
	return sum[:]
}
