package memory

import (
	"context"

	"github.com/google/uuid"
)

// Facts is a driver that keeps facts per Scope. SECURITY: every method checks its Scope before it reads or writes anything.
type Facts interface {
	// Extracts reports whether the driver extracts facts from Turn.UserTexts itself.
	Extracts() bool
	// Recall returns at most k facts of scope, those sharing words with query first, padded with the newest.
	Recall(ctx context.Context, scope Scope, query string, k int) ([]Fact, error)
	// Record stores a turn's facts once each and drops the oldest past MaxPerPerson.
	Record(ctx context.Context, scope Scope, turn Turn) error
	// List pages scope's facts newest first; scope.AgentID uuid.Nil lists every agent's.
	List(ctx context.Context, scope Scope, cursor string, limit int) (facts []Fact, next string, total int, err error)
	// Delete removes one fact of scope; one outside scope is a *NotFoundError.
	Delete(ctx context.Context, scope Scope, id string) error
	// Clear removes every fact of scope and returns how many.
	Clear(ctx context.Context, scope Scope) (int, error)
	// ClearStore removes every fact of scope's store, whoever and whichever agent it is about, and returns how many.
	ClearStore(ctx context.Context, scope Scope) (int, error)
}

// Ref is what a public id resolves to before any tenant is bound.
type Ref struct{ ID, OrgID, ProjectID uuid.UUID }

// EndUsers resolves an end user's public id across tenants; ok is false when there is none.
type EndUsers interface {
	Resolve(ctx context.Context, publicID string) (ref Ref, ok bool, err error)
}

// Agents resolves an agent's public id and names a page's agents by public id.
type Agents interface {
	Resolve(ctx context.Context, publicID string) (ref Ref, ok bool, err error)
	PublicIDs(ctx context.Context, orgID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]string, error)
}

// Runs names a page's source runs by public id; a run that is gone is missing from the map.
type Runs interface {
	PublicIDs(ctx context.Context, orgID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]string, error)
}
