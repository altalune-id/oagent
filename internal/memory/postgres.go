package memory

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/schema/entity"
)

type postgresStore struct {
	pool    db.Pool
	stores  *entity.MemoryStores
	resolve string
}

// NewPostgresStore builds the Postgres store of memory stores over pool.
func NewPostgresStore(pool db.Pool, schema, tablePrefix string) Store {
	if schema == "" {
		schema = "public"
	}
	return &postgresStore{
		pool:   pool,
		stores: entity.NewMemoryStores(schema, tablePrefix),
		// NOTE: RawStatement because go-jet has no builder for a set-returning function in FROM position.
		resolve: `SELECT x.id AS "memory_stores.id", x.org_id AS "memory_stores.org_id", x.project_id AS "memory_stores.project_id" FROM ` +
			schema + "." + tablePrefix + "resolve_memory_store_by_public_id(#publicID) x",
	}
}

type storeRow struct {
	ID        uuid.UUID `alias:"memory_stores.id"`
	PublicID  string    `alias:"memory_stores.public_id"`
	OrgID     uuid.UUID `alias:"memory_stores.org_id"`
	ProjectID uuid.UUID `alias:"memory_stores.project_id"`
	Name      string    `alias:"memory_stores.name"`
	Backend   string    `alias:"memory_stores.backend"`
	APIUsers  bool      `alias:"memory_stores.remember_api_users"`
	CreatedAt time.Time `alias:"memory_stores.created_at"`
	UpdatedAt time.Time `alias:"memory_stores.updated_at"`
}

func (r *storeRow) toStore() *MemoryStore {
	return &MemoryStore{
		ID: r.ID, PublicID: r.PublicID, OrgID: r.OrgID, ProjectID: r.ProjectID, Name: r.Name, Backend: Backend(r.Backend),
		RememberAPIUsers: r.APIUsers, CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
}

// InsertStore records a new store; a name taken in the project is a *StoreAlreadyExistsError.
func (p *postgresStore) InsertStore(ctx context.Context, s *MemoryStore) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	// SECURITY: without RLS in force, a fresh row naming another org would otherwise land in it.
	if s.OrgID != tc.OrgID {
		return &StoreNotFoundError{}
	}
	t := p.stores
	stmt := t.INSERT(t.AllColumns).
		VALUES(s.ID, s.PublicID, s.OrgID, s.ProjectID, s.Name, string(s.Backend), s.RememberAPIUsers, s.CreatedAt.UTC(), s.UpdatedAt.UTC())
	if err := db.Write(ctx, p.pool, func(q qrm.DB) error { _, err := stmt.ExecContext(ctx, q); return err }); err != nil {
		if db.IsUniqueViolation(err) {
			return &StoreAlreadyExistsError{Name: s.Name}
		}
		return fmt.Errorf("memory.postgres: InsertStore: %w", err)
	}
	return nil
}

// StoreByID loads a store of the org on ctx.
func (p *postgresStore) StoreByID(ctx context.Context, id uuid.UUID) (*MemoryStore, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	t := p.stores
	stmt := postgres.SELECT(t.AllColumns).FROM(t).WHERE(t.ID.EQ(postgres.UUID(id)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID)))).LIMIT(1)
	var row storeRow
	if err := db.ReadPrimary(ctx, p.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		if db.IsNoRows(err) {
			return nil, &StoreNotFoundError{}
		}
		return nil, fmt.Errorf("memory.postgres: StoreByID: %w", err)
	}
	return row.toStore(), nil
}

// ListStores returns a project's stores of the org on ctx, oldest first.
func (p *postgresStore) ListStores(ctx context.Context, projectID uuid.UUID) ([]*MemoryStore, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	t := p.stores
	stmt := postgres.SELECT(t.AllColumns).FROM(t).
		WHERE(t.OrgID.EQ(postgres.UUID(tc.OrgID)).AND(t.ProjectID.EQ(postgres.UUID(projectID)))).
		ORDER_BY(t.CreatedAt.ASC(), t.ID.ASC())
	var rows []storeRow
	if err := db.Read(ctx, p.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &rows) }); err != nil && !db.IsNoRows(err) {
		return nil, fmt.Errorf("memory.postgres: ListStores: %w", err)
	}
	out := make([]*MemoryStore, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toStore())
	}
	return out, nil
}

// ResolveStore maps a store's public id to its ids across tenants; reach is the caller's to check.
func (p *postgresStore) ResolveStore(ctx context.Context, publicID string) (Ref, error) {
	var row struct {
		ID        uuid.UUID `alias:"memory_stores.id"`
		OrgID     uuid.UUID `alias:"memory_stores.org_id"`
		ProjectID uuid.UUID `alias:"memory_stores.project_id"`
	}
	stmt := postgres.RawStatement(p.resolve, postgres.RawArgs{"#publicID": publicID})
	if err := db.Read(ctx, p.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		if db.IsNoRows(err) {
			return Ref{}, &StoreNotFoundError{}
		}
		return Ref{}, fmt.Errorf("memory.postgres: ResolveStore: %w", err)
	}
	return Ref(row), nil
}

var errLockOutsideTx = errors.New("memory.postgres: a row lock needs a unit of work")

// LockStoreShared share-locks a store of the org on ctx until the unit of work on ctx ends; a gone one is a *StoreNotFoundError.
func (p *postgresStore) LockStoreShared(ctx context.Context, id uuid.UUID) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	tx, ok := db.CurrentTx(ctx)
	if !ok {
		return errLockOutsideTx
	}
	t := p.stores
	stmt := postgres.SELECT(t.ID).FROM(t).WHERE(t.ID.EQ(postgres.UUID(id)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID)))).FOR(postgres.SHARE())
	var row struct {
		ID uuid.UUID `alias:"memory_stores.id"`
	}
	if err := stmt.QueryContext(ctx, tx, &row); err != nil {
		if db.IsNoRows(err) {
			return &StoreNotFoundError{}
		}
		return fmt.Errorf("memory.postgres: LockStoreShared: %w", err)
	}
	return nil
}

// LockStore loads a store of the org on ctx FOR UPDATE until the unit of work on ctx ends.
func (p *postgresStore) LockStore(ctx context.Context, id uuid.UUID) (*MemoryStore, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	tx, ok := db.CurrentTx(ctx)
	if !ok {
		return nil, errLockOutsideTx
	}
	t := p.stores
	stmt := postgres.SELECT(t.AllColumns).FROM(t).
		WHERE(t.ID.EQ(postgres.UUID(id)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID)))).FOR(postgres.UPDATE())
	var row storeRow
	if err := stmt.QueryContext(ctx, tx, &row); err != nil {
		if db.IsNoRows(err) {
			return nil, &StoreNotFoundError{}
		}
		return nil, fmt.Errorf("memory.postgres: LockStore: %w", err)
	}
	return row.toStore(), nil
}

// DeleteStore removes a store of the org on ctx; its native facts go with it.
func (p *postgresStore) DeleteStore(ctx context.Context, id uuid.UUID) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	t := p.stores
	stmt := t.DELETE().WHERE(t.ID.EQ(postgres.UUID(id)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID))))
	if err := db.Write(ctx, p.pool, func(q qrm.DB) error { _, err := stmt.ExecContext(ctx, q); return err }); err != nil {
		return fmt.Errorf("memory.postgres: DeleteStore: %w", err)
	}
	return nil
}
