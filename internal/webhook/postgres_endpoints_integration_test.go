//go:build integration

package webhook_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/testutil/pgtest"
	"altalune.id/oagent/internal/webhook"
)

type pgFixture struct {
	store    webhook.Store
	sqlDB    *sql.DB
	pool     db.Pool
	orgs     org.Store
	projects project.Store
}

func newPgFixture(t *testing.T) *pgFixture {
	t.Helper()
	sqlDB, schemaName := pgtest.Migrated(t)
	pool := db.Pool{W: sqlDB, R: sqlDB}
	return &pgFixture{
		store:    webhook.NewStore(pool, schemaName, ""),
		sqlDB:    sqlDB,
		pool:     pool,
		orgs:     org.NewStore(pool, schemaName, ""),
		projects: project.NewStore(pool, schemaName, ""),
	}
}

func shortSlug(prefix string) string {
	return prefix + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
}

func (f *pgFixture) seedTenant(t *testing.T) tenant.Context {
	t.Helper()
	o, err := org.NewOrg(shortSlug("o-"), "Org", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, f.orgs.Save(t.Context(), o))
	return tenant.Context{OrgID: o.ID, ProjectID: f.seedProject(t, o.ID)}
}

func (f *pgFixture) seedProject(t *testing.T, orgID uuid.UUID) uuid.UUID {
	t.Helper()
	p, err := project.NewProject(orgID, shortSlug("p-"), "Project", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, f.projects.Save(t.Context(), p))
	return p.ID
}

func randomBytes(t *testing.T, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	_, err := rand.Read(b)
	require.NoError(t, err)
	return b
}

func newEndpoint(t *testing.T, tc tenant.Context) *webhook.Endpoint {
	t.Helper()
	e, err := webhook.New(tc.OrgID, tc.ProjectID, "https://example.com/hook", "orders feed",
		[]events.Type{events.AgentCreated, events.AgentUpdated})
	require.NoError(t, err)
	e.Secrets = webhook.SealedSecrets{Primary: randomBytes(t, 32)}
	e.CreatedAt = e.CreatedAt.Truncate(time.Microsecond)
	e.UpdatedAt = e.CreatedAt
	return e
}

func (f *pgFixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, f.sqlDB.QueryRowContext(t.Context(), query, args...).Scan(&n))
	return n
}

func pgCode(err error) string {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return pgErr.Code
	}
	return ""
}

func assertSameEndpoint(t *testing.T, want, got *webhook.Endpoint) {
	t.Helper()
	assert.Equal(t, want.ID, got.ID)
	assert.Equal(t, want.PublicID, got.PublicID)
	assert.Equal(t, want.OrgID, got.OrgID)
	assert.Equal(t, want.ProjectID, got.ProjectID)
	assert.Equal(t, want.URL, got.URL)
	assert.Equal(t, want.Description, got.Description)
	assert.Equal(t, want.EventTypes, got.EventTypes)
	assert.Equal(t, want.Secrets, got.Secrets)
	assert.Equal(t, want.Active, got.Active)
	assert.True(t, want.CreatedAt.Equal(got.CreatedAt), "created_at want %v got %v", want.CreatedAt, got.CreatedAt)
	assert.True(t, want.UpdatedAt.Equal(got.UpdatedAt), "updated_at want %v got %v", want.UpdatedAt, got.UpdatedAt)
	assert.Equal(t, time.UTC, got.CreatedAt.Location())
	assert.Equal(t, time.UTC, got.UpdatedAt.Location())
}

func TestPostgresEndpoint_SaveRoundTrip(t *testing.T) {
	f := newPgFixture(t)
	tc := f.seedTenant(t)
	ctx := tenant.Into(t.Context(), tc)

	cases := []struct {
		name      string
		secondary []byte
		active    bool
	}{
		{name: "secondary NULL", secondary: nil, active: true},
		{name: "secondary set", secondary: randomBytes(t, 32), active: false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			e := newEndpoint(t, tc)
			e.Secrets.Secondary = tt.secondary
			e.Active = tt.active
			e.CreatedAt = time.Date(2026, 3, 4, 5, 6, 7, 123456000, time.FixedZone("WIB", 7*3600))
			e.UpdatedAt = e.CreatedAt.Add(time.Hour)
			require.NoError(t, f.store.Save(ctx, e))

			got, err := f.store.ByID(ctx, e.ID)
			require.NoError(t, err)
			assertSameEndpoint(t, e, got)
			if tt.secondary == nil {
				assert.Nil(t, got.Secrets.Secondary)
			}
		})
	}
}

func TestPostgresEndpoint_SaveWritesAnEmptySecondaryAsNull(t *testing.T) {
	f := newPgFixture(t)
	tc := f.seedTenant(t)
	ctx := tenant.Into(t.Context(), tc)

	e := newEndpoint(t, tc)
	e.Secrets.Secondary = []byte{}
	require.NoError(t, f.store.Save(ctx, e), "an empty secondary must not trip the length CHECK")

	got, err := f.store.ByID(ctx, e.ID)
	require.NoError(t, err)
	assert.Nil(t, got.Secrets.Secondary)
	assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM webhook_endpoints WHERE id = $1 AND secret_secondary IS NULL", e.ID))
}

func TestPostgresEndpoint_SecondSaveUpdatesEditableColumnsOnly(t *testing.T) {
	f := newPgFixture(t)
	tc := f.seedTenant(t)
	ctx := tenant.Into(t.Context(), tc)

	e := newEndpoint(t, tc)
	require.NoError(t, f.store.Save(ctx, e))
	want := *e

	changed := *e
	changed.PublicID = "changed-public-id"
	changed.ProjectID = f.seedProject(t, tc.OrgID)
	changed.Secrets = webhook.SealedSecrets{Primary: randomBytes(t, 32), Secondary: randomBytes(t, 32)}
	changed.CreatedAt = e.CreatedAt.Add(-24 * time.Hour)
	changed.URL = "https://example.org/other"
	changed.Description = "renamed"
	changed.EventTypes = []events.Type{events.AgentUpdated}
	changed.Active = false
	changed.UpdatedAt = e.UpdatedAt.Add(time.Minute)
	require.NoError(t, f.store.Save(ctx, &changed))

	want.URL = changed.URL
	want.Description = changed.Description
	want.EventTypes = changed.EventTypes
	want.Active = changed.Active
	want.UpdatedAt = changed.UpdatedAt
	got, err := f.store.ByID(ctx, e.ID)
	require.NoError(t, err)
	assertSameEndpoint(t, &want, got)
}

func TestPostgresEndpoint_SaveRefusesAnotherOrg(t *testing.T) {
	f := newPgFixture(t)
	a := f.seedTenant(t)
	b := f.seedTenant(t)
	ctxA := tenant.Into(t.Context(), a)
	ctxB := tenant.Into(t.Context(), b)

	t.Run("fresh row naming another org", func(t *testing.T) {
		e := newEndpoint(t, a)
		err := f.store.Save(ctxB, e)
		assert.True(t, webhook.IsNotFoundError(err), "got %T: %v", err, err)
		assert.Zero(t, f.count(t, "SELECT count(*) FROM webhook_endpoints WHERE id = $1", e.ID))
	})

	t.Run("conflict on another org's id", func(t *testing.T) {
		victim := newEndpoint(t, a)
		require.NoError(t, f.store.Save(ctxA, victim))

		hijack := newEndpoint(t, b)
		hijack.ID = victim.ID
		hijack.URL = "https://attacker.example/hook"
		err := f.store.Save(ctxB, hijack)
		assert.True(t, webhook.IsNotFoundError(err), "the org guard on the conflict clause must refuse, got %T: %v", err, err)

		got, err := f.store.ByID(ctxA, victim.ID)
		require.NoError(t, err)
		assertSameEndpoint(t, victim, got)
	})
}

func TestPostgresEndpoint_PublicIDIsUnique(t *testing.T) {
	f := newPgFixture(t)
	a := f.seedTenant(t)
	b := f.seedTenant(t)

	first := newEndpoint(t, a)
	require.NoError(t, f.store.Save(tenant.Into(t.Context(), a), first))

	cases := []struct {
		name string
		tc   tenant.Context
	}{
		{name: "same org", tc: a},
		{name: "another org", tc: b},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			dup := newEndpoint(t, tt.tc)
			dup.PublicID = first.PublicID
			err := f.store.Save(tenant.Into(t.Context(), tt.tc), dup)
			require.Error(t, err)
			assert.Equal(t, "23505", pgCode(err), "public_id collisions surface as a raw unique violation, got %v", err)
			assert.False(t, webhook.IsNotFoundError(err))
		})
	}
}

func TestPostgresEndpoint_SecondaryNonEmptyCheck(t *testing.T) {
	f := newPgFixture(t)
	tc := f.seedTenant(t)
	e := newEndpoint(t, tc)
	require.NoError(t, f.store.Save(tenant.Into(t.Context(), tc), e))

	_, err := f.sqlDB.ExecContext(t.Context(), "UPDATE webhook_endpoints SET secret_secondary = '\\x'::bytea WHERE id = $1", e.ID)
	require.Error(t, err)
	assert.Equal(t, "23514", pgCode(err), "an empty secondary must violate the CHECK, got %v", err)

	_, err = f.sqlDB.ExecContext(t.Context(), "UPDATE webhook_endpoints SET secret_secondary = '\\x01'::bytea WHERE id = $1", e.ID)
	require.NoError(t, err)
	_, err = f.sqlDB.ExecContext(t.Context(), "UPDATE webhook_endpoints SET secret_secondary = NULL WHERE id = $1", e.ID)
	require.NoError(t, err)
}

func TestPostgresEndpoint_ByIDNotFound(t *testing.T) {
	f := newPgFixture(t)
	a := f.seedTenant(t)
	b := f.seedTenant(t)
	e := newEndpoint(t, a)
	require.NoError(t, f.store.Save(tenant.Into(t.Context(), a), e))

	cases := []struct {
		name string
		tc   tenant.Context
		id   uuid.UUID
	}{
		{name: "unknown id", tc: a, id: uuid.New()},
		{name: "another org's id", tc: b, id: e.ID},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := f.store.ByID(tenant.Into(t.Context(), tt.tc), tt.id)
			assert.True(t, webhook.IsNotFoundError(err), "got %T: %v", err, err)
		})
	}
}

func TestPostgresEndpoint_RequiresTenant(t *testing.T) {
	f := newPgFixture(t)
	tc := f.seedTenant(t)
	e := newEndpoint(t, tc)

	assert.Error(t, f.store.Save(t.Context(), e))
	_, err := f.store.ByID(t.Context(), e.ID)
	assert.Error(t, err)
	_, err = f.store.List(t.Context(), tc.OrgID, tc.ProjectID)
	assert.Error(t, err)
	assert.Error(t, f.store.Delete(t.Context(), e.ID))
	assert.Zero(t, f.count(t, "SELECT count(*) FROM webhook_endpoints"))
}

func TestPostgresEndpoint_ListScopesAndOrders(t *testing.T) {
	f := newPgFixture(t)
	a := f.seedTenant(t)
	b := f.seedTenant(t)
	ctxA := tenant.Into(t.Context(), a)

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	older := newEndpoint(t, a)
	older.CreatedAt = base
	tieA := newEndpoint(t, a)
	tieA.CreatedAt = base.Add(time.Minute)
	tieB := newEndpoint(t, a)
	tieB.CreatedAt = base.Add(time.Minute)
	newest := newEndpoint(t, a)
	newest.CreatedAt = base.Add(time.Hour)
	for _, e := range []*webhook.Endpoint{tieA, older, newest, tieB} {
		require.NoError(t, f.store.Save(ctxA, e))
	}

	otherProject := a
	otherProject.ProjectID = f.seedProject(t, a.OrgID)
	require.NoError(t, f.store.Save(tenant.Into(t.Context(), otherProject), newEndpoint(t, otherProject)))
	require.NoError(t, f.store.Save(tenant.Into(t.Context(), b), newEndpoint(t, b)))

	ties := []uuid.UUID{tieA.ID, tieB.ID}
	if ties[0].String() < ties[1].String() {
		ties[0], ties[1] = ties[1], ties[0]
	}
	got, err := f.store.List(ctxA, a.OrgID, a.ProjectID)
	require.NoError(t, err)
	ids := make([]uuid.UUID, 0, len(got))
	for _, e := range got {
		ids = append(ids, e.ID)
	}
	assert.Equal(t, []uuid.UUID{newest.ID, ties[0], ties[1], older.ID}, ids, "created_at DESC, then id DESC")

	t.Run("another project of the same org", func(t *testing.T) {
		got, err := f.store.List(ctxA, a.OrgID, otherProject.ProjectID)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, otherProject.ProjectID, got[0].ProjectID)
	})

	t.Run("another org's ids from my tenant", func(t *testing.T) {
		got, err := f.store.List(ctxA, b.OrgID, b.ProjectID)
		require.NoError(t, err)
		assert.Empty(t, got, "the tenant org on ctx must also bound List")
	})

	t.Run("my ids from another tenant", func(t *testing.T) {
		got, err := f.store.List(tenant.Into(t.Context(), b), a.OrgID, a.ProjectID)
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("empty project", func(t *testing.T) {
		got, err := f.store.List(ctxA, a.OrgID, f.seedProject(t, a.OrgID))
		require.NoError(t, err)
		assert.NotNil(t, got)
		assert.Empty(t, got)
	})
}

func TestPostgresEndpoint_Delete(t *testing.T) {
	f := newPgFixture(t)
	a := f.seedTenant(t)
	b := f.seedTenant(t)
	ctxA := tenant.Into(t.Context(), a)

	e := newEndpoint(t, a)
	require.NoError(t, f.store.Save(ctxA, e))

	err := f.store.Delete(tenant.Into(t.Context(), b), e.ID)
	assert.True(t, webhook.IsNotFoundError(err), "another org must not delete, got %T: %v", err, err)
	_, err = f.store.ByID(ctxA, e.ID)
	require.NoError(t, err)

	require.NoError(t, f.store.Delete(ctxA, e.ID))
	_, err = f.store.ByID(ctxA, e.ID)
	assert.True(t, webhook.IsNotFoundError(err))

	err = f.store.Delete(ctxA, e.ID)
	assert.True(t, webhook.IsNotFoundError(err), "a second delete is not found, got %T: %v", err, err)
}

func TestPostgresEndpoint_ParentDeleteCascades(t *testing.T) {
	cases := []struct {
		name   string
		delete func(t *testing.T, f *pgFixture, tc tenant.Context)
	}{
		{name: "project", delete: func(t *testing.T, f *pgFixture, tc tenant.Context) {
			t.Helper()
			_, err := f.sqlDB.ExecContext(t.Context(), "DELETE FROM projects WHERE id = $1", tc.ProjectID)
			require.NoError(t, err)
		}},
		{name: "org", delete: func(t *testing.T, f *pgFixture, tc tenant.Context) {
			t.Helper()
			_, err := f.sqlDB.ExecContext(t.Context(), "DELETE FROM orgs WHERE id = $1", tc.OrgID)
			require.NoError(t, err)
		}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			f := newPgFixture(t)
			tc := f.seedTenant(t)
			ctx := tenant.Into(t.Context(), tc)
			e := newEndpoint(t, tc)
			require.NoError(t, f.store.Save(ctx, e))
			require.NoError(t, f.store.SaveAttempt(ctx, newAttempt(tc, e.ID, uuid.New(), 1, time.Now())))

			tt.delete(t, f, tc)

			assert.Zero(t, f.count(t, "SELECT count(*) FROM webhook_endpoints"))
			assert.Zero(t, f.count(t, "SELECT count(*) FROM webhook_deliveries"))
		})
	}
}

func TestPostgresEndpoint_SaveRejectsUnknownParents(t *testing.T) {
	f := newPgFixture(t)
	tc := f.seedTenant(t)
	ctx := tenant.Into(t.Context(), tc)

	e := newEndpoint(t, tc)
	e.ProjectID = uuid.New()
	err := f.store.Save(ctx, e)
	require.Error(t, err)
	assert.Equal(t, "23503", pgCode(err), "an unknown project surfaces as a raw FK violation, got %v", err)
}

func TestPostgresEndpoint_SaveSecrets(t *testing.T) {
	p1, p2, p3 := []byte("primary-1"), []byte("primary-2"), []byte("primary-3")

	cases := []struct {
		name     string
		stored   webhook.SealedSecrets
		expected webhook.SealedSecrets
		next     webhook.SealedSecrets
		want     webhook.SealedSecrets
		conflict bool
	}{
		{
			name:     "rotate into a pair",
			stored:   webhook.SealedSecrets{Primary: p1},
			expected: webhook.SealedSecrets{Primary: p1},
			next:     webhook.SealedSecrets{Primary: p2, Secondary: p1},
			want:     webhook.SealedSecrets{Primary: p2, Secondary: p1},
		},
		{
			name:     "finish rotation clears the secondary",
			stored:   webhook.SealedSecrets{Primary: p2, Secondary: p1},
			expected: webhook.SealedSecrets{Primary: p2, Secondary: p1},
			next:     webhook.SealedSecrets{Primary: p2},
			want:     webhook.SealedSecrets{Primary: p2},
		},
		{
			name:     "empty next secondary is written as NULL",
			stored:   webhook.SealedSecrets{Primary: p2, Secondary: p1},
			expected: webhook.SealedSecrets{Primary: p2, Secondary: p1},
			next:     webhook.SealedSecrets{Primary: p3, Secondary: []byte{}},
			want:     webhook.SealedSecrets{Primary: p3},
		},
		{
			name:     "empty expected secondary matches NULL",
			stored:   webhook.SealedSecrets{Primary: p1},
			expected: webhook.SealedSecrets{Primary: p1, Secondary: []byte{}},
			next:     webhook.SealedSecrets{Primary: p2},
			want:     webhook.SealedSecrets{Primary: p2},
		},
		{
			name:     "stale primary",
			stored:   webhook.SealedSecrets{Primary: p2},
			expected: webhook.SealedSecrets{Primary: p1},
			next:     webhook.SealedSecrets{Primary: p3},
			conflict: true,
		},
		{
			name:     "expected NULL secondary but stored set",
			stored:   webhook.SealedSecrets{Primary: p2, Secondary: p1},
			expected: webhook.SealedSecrets{Primary: p2},
			next:     webhook.SealedSecrets{Primary: p3},
			conflict: true,
		},
		{
			name:     "expected secondary but stored NULL",
			stored:   webhook.SealedSecrets{Primary: p2},
			expected: webhook.SealedSecrets{Primary: p2, Secondary: p1},
			next:     webhook.SealedSecrets{Primary: p3},
			conflict: true,
		},
		{
			name:     "different secondary",
			stored:   webhook.SealedSecrets{Primary: p2, Secondary: p1},
			expected: webhook.SealedSecrets{Primary: p2, Secondary: p3},
			next:     webhook.SealedSecrets{Primary: p3},
			conflict: true,
		},
	}
	f := newPgFixture(t)
	tc := f.seedTenant(t)
	ctx := tenant.Into(t.Context(), tc)
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			e := newEndpoint(t, tc)
			e.Secrets = tt.stored
			require.NoError(t, f.store.Save(ctx, e))
			before := time.Now().Add(-time.Second)

			err := f.store.SaveSecrets(ctx, e.ID, tt.expected, tt.next)

			got, rerr := f.store.ByID(ctx, e.ID)
			require.NoError(t, rerr)
			if tt.conflict {
				assert.True(t, webhook.IsSecretConflictError(err), "got %T: %v", err, err)
				assertSameEndpoint(t, e, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got.Secrets)
			if tt.want.Secondary == nil {
				assert.Nil(t, got.Secrets.Secondary)
			}
			assert.True(t, got.UpdatedAt.After(before), "updated_at must be bumped")
			assert.Equal(t, e.URL, got.URL)
			assert.True(t, e.CreatedAt.Equal(got.CreatedAt))
		})
	}
}

func TestPostgresEndpoint_SaveSecretsStaleAfterRotation(t *testing.T) {
	f := newPgFixture(t)
	tc := f.seedTenant(t)
	ctx := tenant.Into(t.Context(), tc)
	e := newEndpoint(t, tc)
	require.NoError(t, f.store.Save(ctx, e))
	original := e.Secrets

	rotated := webhook.SealedSecrets{Primary: randomBytes(t, 32), Secondary: original.Primary}
	require.NoError(t, f.store.SaveSecrets(ctx, e.ID, original, rotated))

	err := f.store.SaveSecrets(ctx, e.ID, original, webhook.SealedSecrets{Primary: randomBytes(t, 32)})
	assert.True(t, webhook.IsSecretConflictError(err), "a reused expected must be stale, got %T: %v", err, err)

	got, err := f.store.ByID(ctx, e.ID)
	require.NoError(t, err)
	assert.Equal(t, rotated, got.Secrets)
}

func TestPostgresEndpoint_SaveSecretsNotFound(t *testing.T) {
	f := newPgFixture(t)
	a := f.seedTenant(t)
	b := f.seedTenant(t)
	e := newEndpoint(t, a)
	require.NoError(t, f.store.Save(tenant.Into(t.Context(), a), e))

	cases := []struct {
		name string
		tc   tenant.Context
		id   uuid.UUID
	}{
		{name: "unknown id", tc: a, id: uuid.New()},
		{name: "another org's id with the right expected", tc: b, id: e.ID},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			err := f.store.SaveSecrets(tenant.Into(t.Context(), tt.tc), tt.id, e.Secrets,
				webhook.SealedSecrets{Primary: randomBytes(t, 32)})
			assert.True(t, webhook.IsNotFoundError(err), "got %T: %v", err, err)
		})
	}
	got, err := f.store.ByID(tenant.Into(t.Context(), a), e.ID)
	require.NoError(t, err)
	assert.Equal(t, e.Secrets, got.Secrets)
}

func TestPostgresEndpoint_SaveSecretsConcurrentCASHasOneWinner(t *testing.T) {
	f := newPgFixture(t)
	tc := f.seedTenant(t)
	ctx := tenant.Into(t.Context(), tc)
	e := newEndpoint(t, tc)
	require.NoError(t, f.store.Save(ctx, e))

	const racers = 8
	nexts := make([]webhook.SealedSecrets, racers)
	for i := range nexts {
		nexts[i] = webhook.SealedSecrets{Primary: randomBytes(t, 32), Secondary: e.Secrets.Primary}
	}
	errs := make([]error, racers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range racers {
		wg.Go(func() {
			<-start
			errs[i] = f.store.SaveSecrets(ctx, e.ID, e.Secrets, nexts[i])
		})
	}
	close(start)
	wg.Wait()

	winner := -1
	for i, err := range errs {
		if err == nil {
			assert.Equal(t, -1, winner, "only one racer may win")
			winner = i
			continue
		}
		assert.True(t, webhook.IsSecretConflictError(err), "losers must see a conflict, got %T: %v", err, err)
	}
	require.NotEqual(t, -1, winner, "one racer must win")
	got, err := f.store.ByID(ctx, e.ID)
	require.NoError(t, err)
	assert.Equal(t, nexts[winner], got.Secrets)
}

func TestPostgresEndpoint_UnitOfWork(t *testing.T) {
	f := newPgFixture(t)
	tc := f.seedTenant(t)
	ctx := tenant.Into(t.Context(), tc)
	errRollback := errors.New("rollback")

	rolled := newEndpoint(t, tc)
	err := db.RunInTx(ctx, f.pool, func(ctx context.Context) error {
		require.NoError(t, f.store.Save(ctx, rolled))
		got, err := f.store.ByID(ctx, rolled.ID)
		require.NoError(t, err, "reads inside the unit of work see its writes")
		assert.Equal(t, rolled.ID, got.ID)
		require.NoError(t, f.store.SaveAttempt(ctx, newAttempt(tc, rolled.ID, uuid.New(), 1, time.Now())))
		return errRollback
	})
	require.ErrorIs(t, err, errRollback)
	_, err = f.store.ByID(ctx, rolled.ID)
	assert.True(t, webhook.IsNotFoundError(err), "a rolled-back Save leaves nothing")
	assert.Zero(t, f.count(t, "SELECT count(*) FROM webhook_deliveries"))

	kept := newEndpoint(t, tc)
	next := webhook.SealedSecrets{Primary: randomBytes(t, 32)}
	require.NoError(t, db.RunInTx(ctx, f.pool, func(ctx context.Context) error {
		if err := f.store.Save(ctx, kept); err != nil {
			return err
		}
		return f.store.SaveSecrets(ctx, kept.ID, kept.Secrets, next)
	}))
	got, err := f.store.ByID(ctx, kept.ID)
	require.NoError(t, err)
	assert.Equal(t, next, got.Secrets)

	err = db.RunInTx(ctx, f.pool, func(ctx context.Context) error {
		require.NoError(t, f.store.Delete(ctx, kept.ID))
		return errRollback
	})
	require.ErrorIs(t, err, errRollback)
	_, err = f.store.ByID(ctx, kept.ID)
	assert.NoError(t, err, "a rolled-back Delete keeps the row")
}

func TestPostgresEndpoint_ListByOrg(t *testing.T) {
	f := newPgFixture(t)
	a := f.seedTenant(t)
	b := f.seedTenant(t)
	first := newEndpoint(t, a)
	first.CreatedAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, f.store.Save(tenant.Into(t.Context(), a), first))
	sibling := a
	sibling.ProjectID = f.seedProject(t, a.OrgID)
	second := newEndpoint(t, sibling)
	second.CreatedAt = first.CreatedAt.Add(time.Hour)
	require.NoError(t, f.store.Save(tenant.Into(t.Context(), sibling), second))
	require.NoError(t, f.store.Save(tenant.Into(t.Context(), b), newEndpoint(t, b)))

	got, err := f.store.ListByOrg(t.Context(), a.OrgID)
	require.NoError(t, err)
	require.Len(t, got, 2, "both projects of org A, none of org B")
	assert.Equal(t, first.ID, got[0].ID)
	assert.Equal(t, second.ID, got[1].ID)
	assert.Equal(t, sibling.ProjectID, got[1].ProjectID)
}

func TestPostgres_ByPublicIDStaysInTheCallersOrg(t *testing.T) {
	f := newPgFixture(t)
	mine, other := f.seedTenant(t), f.seedTenant(t)
	e := newEndpoint(t, mine)
	require.NoError(t, f.store.Save(tenant.Into(t.Context(), mine), e))

	got, err := f.store.ByPublicID(tenant.Into(t.Context(), mine), e.PublicID)
	require.NoError(t, err)
	assert.Equal(t, e.ID, got.ID)
	_, err = f.store.ByPublicID(tenant.Into(t.Context(), other), e.PublicID)
	assert.True(t, webhook.IsNotFoundError(err), "another org's endpoint is not found")
	_, err = f.store.ByPublicID(tenant.Into(t.Context(), mine), e.ID.String())
	assert.True(t, webhook.IsNotFoundError(err), "a uuid is not a public id")
	_, err = f.store.ByPublicID(t.Context(), e.PublicID)
	assert.True(t, tenant.IsMissingError(err))
}
