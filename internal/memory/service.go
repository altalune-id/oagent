package memory

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
)

// Listed is one fact as an admin sees it; every id is a public id and SourceRunID is empty once the run is gone.
type Listed struct {
	ID, Text, AgentID, SourceRunID string
	CreatedAt                      time.Time
}

// Page is one page of a person's facts, newest first; Next is empty on the last page.
type Page struct {
	Memories []Listed
	Next     string
	Total    int
}

// Service manages a project's memory stores and lets admins list, delete and clear what agents remember about an end user.
type Service struct {
	stores     Store
	facts      map[Backend]Facts
	enabled    []Backend
	uow        db.Transactor
	bindings   Bindings
	users      EndUsers
	agents     Agents
	runs       Runs
	unexpected apperror.UnexpectedFunc
}

// Deps are the Service's ports: the people, agents and runs it names, and the unit of work a store delete runs in.
type Deps struct {
	UOW    db.Transactor
	Users  EndUsers
	Agents Agents
	Runs   Runs
}

// NewService builds a Service over stores, one Facts driver per backend and the enabled backends.
func NewService(stores Store, facts map[Backend]Facts, enabled []Backend, d Deps, unexpected apperror.UnexpectedFunc) *Service {
	return &Service{
		stores: stores, facts: facts, enabled: enabled, uow: d.UOW, users: d.Users, agents: d.Agents, runs: d.Runs, unexpected: unexpected,
	}
}

// StoreUsable refuses a store outside projectID (*StoreNotFoundError) or one whose backend is off (*InvalidStoreError).
func (s *Service) StoreUsable(ctx context.Context, projectID, storeID uuid.UUID) error {
	_, err := s.usable(ctx, projectID, storeID)
	return err
}

func (s *Service) usable(ctx context.Context, projectID, storeID uuid.UUID) (*MemoryStore, error) {
	store, err := s.stores.StoreByID(ctx, storeID)
	if err != nil {
		return nil, s.errOrReport(ctx, "memory: store usable", err)
	}
	if store.ProjectID != projectID {
		return nil, &StoreNotFoundError{}
	}
	if !slices.Contains(s.enabled, store.Backend) {
		return nil, &InvalidStoreError{Reason: StoreBackendDisabled}
	}
	return store, nil
}

// Mode reports how scope's store takes part in a run; an unusable store is an error.
func (s *Service) Mode(ctx context.Context, scope Scope) (Mode, error) {
	store, driver, err := s.driver(ctx, scope)
	if err != nil {
		return Mode{}, err
	}
	return Mode{Extracts: driver.Extracts(), APIUsers: store.RememberAPIUsers}, nil
}

// Recall returns at most k facts of scope from its store's driver.
func (s *Service) Recall(ctx context.Context, scope Scope, query string, k int) ([]Fact, error) {
	_, driver, err := s.driver(ctx, scope)
	if err != nil {
		return nil, err
	}
	return driver.Recall(ctx, scope, query, k)
}

// Record stores a turn's facts or user texts in scope's store.
func (s *Service) Record(ctx context.Context, scope Scope, turn Turn) error {
	// SECURITY: the store stays share-locked around the driver call, so a store delete waits and no fact lands after its erase.
	return s.uow.RunInTx(ctx, func(ctx context.Context) error {
		if err := s.stores.LockStoreShared(ctx, scope.StoreID); err != nil {
			return err
		}
		_, driver, err := s.driver(ctx, scope)
		if err != nil {
			return err
		}
		return driver.Record(ctx, scope, turn)
	})
}

// SECURITY: a run's store must be in the run's project and its backend on; a disabled backend fails closed, so the leg has no memory.
func (s *Service) driver(ctx context.Context, scope Scope) (*MemoryStore, Facts, error) {
	store, err := s.usable(ctx, scope.ProjectID, scope.StoreID)
	if err != nil {
		return nil, nil, err
	}
	facts, err := s.driverFor(store)
	if err != nil {
		return nil, nil, err
	}
	return store, facts, nil
}

func (s *Service) driverFor(store *MemoryStore) (Facts, error) {
	if facts, ok := s.facts[store.Backend]; ok {
		return facts, nil
	}
	return nil, &StoreBackendOffError{Backend: store.Backend}
}

// LockStoreShared share-locks a store while an agent version that remembers people in it is published.
func (s *Service) LockStoreShared(ctx context.Context, id uuid.UUID) error {
	return s.errOrReport(ctx, "memory: lock store", s.stores.LockStoreShared(ctx, id))
}

// SECURITY: an unknown or unreachable person, or an agent outside the person's project, answers the same not-found.
func (s *Service) scope(ctx context.Context, storeID, endUserID, agentID string) (context.Context, Scope, Facts, error) {
	notFound := &NotFoundError{ID: endUserID}
	user, ok, err := s.users.Resolve(ctx, endUserID)
	if err != nil {
		return ctx, Scope{}, nil, s.unexpected(ctx, "memory: resolve end user", err)
	}
	if !ok || !authz.ReachWholeProject(ctx, user.OrgID, user.ProjectID) {
		return ctx, Scope{}, nil, notFound
	}
	ctx = tenant.Into(ctx, tenant.Context{OrgID: user.OrgID, ProjectID: user.ProjectID})
	store, err := s.store(ctx, storeID, user)
	if err != nil {
		return ctx, Scope{}, nil, err
	}
	facts, err := s.driverFor(store)
	if err != nil {
		return ctx, Scope{}, nil, err
	}
	scope := Scope{OrgID: user.OrgID, ProjectID: user.ProjectID, StoreID: store.ID, EndUserID: user.ID}
	if agentID == "" {
		return ctx, scope, facts, nil
	}
	agent, ok, err := s.agents.Resolve(ctx, agentID)
	if err != nil {
		return ctx, Scope{}, nil, s.unexpected(ctx, "memory: resolve agent", err)
	}
	if !ok || agent.OrgID != user.OrgID || agent.ProjectID != user.ProjectID {
		return ctx, Scope{}, nil, notFound
	}
	scope.AgentID = agent.ID
	return ctx, scope, facts, nil
}

func (s *Service) store(ctx context.Context, publicID string, in Ref) (*MemoryStore, error) {
	ref, err := s.stores.ResolveStore(ctx, publicID)
	if err != nil {
		return nil, s.errOrReport(ctx, "memory: resolve store", err)
	}
	if ref.OrgID != in.OrgID || ref.ProjectID != in.ProjectID {
		return nil, &StoreNotFoundError{}
	}
	store, err := s.stores.StoreByID(ctx, ref.ID)
	if err != nil {
		return nil, s.errOrReport(ctx, "memory: load store", err)
	}
	return store, nil
}

func (s *Service) errOrReport(ctx context.Context, message string, err error) error {
	if err == nil || IsExpectedError(err) {
		return err
	}
	return s.unexpected(ctx, message, err)
}

// List pages an end user's facts in a store newest first, every agent's or only agentID's.
func (s *Service) List(ctx context.Context, storeID, endUserID, agentID, cursor string, limit int) (Page, error) {
	ctx, scope, driver, err := s.scope(ctx, storeID, endUserID, agentID)
	if err != nil {
		return Page{}, err
	}
	facts, next, total, err := driver.List(ctx, scope, cursor, limit)
	if err != nil {
		return Page{}, s.errOrReport(ctx, "memory: list", err)
	}
	agentIDs, runIDs := make([]uuid.UUID, 0, len(facts)), make([]uuid.UUID, 0, len(facts))
	for _, f := range facts {
		agentIDs = append(agentIDs, f.AgentID)
		if f.SourceRunID != uuid.Nil {
			runIDs = append(runIDs, f.SourceRunID)
		}
	}
	agents, err := s.agents.PublicIDs(ctx, scope.OrgID, agentIDs)
	if err != nil {
		return Page{}, s.unexpected(ctx, "memory: agent ids", err)
	}
	runs, err := s.runs.PublicIDs(ctx, scope.OrgID, runIDs)
	if err != nil {
		return Page{}, s.unexpected(ctx, "memory: run ids", err)
	}
	page := Page{Memories: make([]Listed, len(facts)), Next: next, Total: total}
	for i, f := range facts {
		page.Memories[i] = Listed{ID: f.ID, Text: f.Text, AgentID: agents[f.AgentID], SourceRunID: runs[f.SourceRunID], CreatedAt: f.CreatedAt}
	}
	return page, nil
}

// Delete removes one fact of an end user in a store; a fact of anyone else, or of another store, is not found.
func (s *Service) Delete(ctx context.Context, storeID, endUserID, id string) error {
	ctx, scope, driver, err := s.scope(ctx, storeID, endUserID, "")
	if err != nil {
		return err
	}
	if err := driver.Delete(ctx, scope, id); err != nil {
		return s.errOrReport(ctx, "memory: delete", err)
	}
	return nil
}

// Clear removes an end user's facts in a store, every agent's or only agentID's, and returns how many.
func (s *Service) Clear(ctx context.Context, storeID, endUserID, agentID string) (int, error) {
	ctx, scope, driver, err := s.scope(ctx, storeID, endUserID, agentID)
	if err != nil {
		return 0, err
	}
	n, err := driver.Clear(ctx, scope)
	if err != nil {
		return 0, s.errOrReport(ctx, "memory: clear", err)
	}
	return n, nil
}
