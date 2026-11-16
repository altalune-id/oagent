//go:build integration

package webtool_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/testutil/pgtest"
	"altalune.id/oagent/internal/webtool"
)

type pgFixture struct {
	admin    *sql.DB
	pool     db.Pool
	schema   string
	store    webtool.Store
	orgs     org.Store
	projects project.Store
}

func newPgFixture(t *testing.T) *pgFixture {
	t.Helper()
	pool, admin, schemaName := pgtest.MigratedRLS(t)
	return &pgFixture{
		admin: admin, pool: pool, schema: schemaName,
		store:    webtool.NewStore(pool, schemaName, ""),
		orgs:     org.NewStore(pool, schemaName, ""),
		projects: project.NewStore(pool, schemaName, ""),
	}
}

func in(orgID uuid.UUID) context.Context {
	return tenant.Into(context.Background(), tenant.Context{OrgID: orgID})
}

func (f *pgFixture) seedScope(t *testing.T) (orgID, projectID uuid.UUID) {
	t.Helper()
	o, err := org.NewOrg("org-"+uuid.NewString()[:8], "Org", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, f.orgs.Save(in(o.ID), o))
	return o.ID, f.seedProject(t, o.ID)
}

func (f *pgFixture) seedProject(t *testing.T, orgID uuid.UUID) uuid.UUID {
	t.Helper()
	p, err := project.NewProject(orgID, "prj-"+uuid.NewString()[:8], "Project", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, f.projects.Save(in(orgID), p))
	return p.ID
}

func provider(orgID, projectID uuid.UUID, kind webtool.Kind, price int64) *webtool.Provider {
	now := time.Now().UTC().Truncate(time.Microsecond)
	return &webtool.Provider{OrgID: orgID, ProjectID: projectID, Kind: kind, APIKeySealed: []byte{1, 2}, PricePerCallMicros: price, CreatedAt: now, UpdatedAt: now}
}

func secret(t *testing.T, orgID, projectID uuid.UUID, host, header string, value byte) *webtool.HTTPSecret {
	t.Helper()
	s, err := webtool.NewHTTPSecret(orgID, projectID, host, header, time.Now())
	require.NoError(t, err)
	s.ValueSealed = []byte{value}
	return s
}

func TestPostgres_ProviderRoundTrip(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)

	_, err := f.store.Provider(ctx, projectID)
	assert.True(t, webtool.IsNotFoundError(err))

	p := provider(orgID, projectID, webtool.KindBrave, 0)
	require.NoError(t, f.store.SaveProvider(ctx, p))
	got, err := f.store.Provider(ctx, projectID)
	require.NoError(t, err)
	assert.Equal(t, p, got)

	p.Kind, p.PricePerCallMicros, p.APIKeySealed, p.UpdatedAt = webtool.KindExa, 5000, []byte{9}, p.UpdatedAt.Add(time.Second)
	require.NoError(t, f.store.SaveProvider(ctx, p))
	got, err = f.store.Provider(ctx, projectID)
	require.NoError(t, err)
	assert.Equal(t, p, got, "an upsert replaces kind, key and price")

	require.NoError(t, f.store.DeleteProvider(ctx, projectID))
	require.NoError(t, f.store.DeleteProvider(ctx, projectID), "delete is idempotent")
	_, err = f.store.Provider(ctx, projectID)
	assert.True(t, webtool.IsNotFoundError(err))
}

func TestPostgres_ProviderNegativePriceRefused(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	assert.Error(t, f.store.SaveProvider(in(orgID), provider(orgID, projectID, webtool.KindBrave, -1)))
}

func TestPostgres_ProviderOfAnotherOrgIsRefused(t *testing.T) {
	f := newPgFixture(t)
	orgA, projectA := f.seedScope(t)
	orgB, _ := f.seedScope(t)
	require.NoError(t, f.store.SaveProvider(in(orgA), provider(orgA, projectA, webtool.KindBrave, 1)))

	err := f.store.SaveProvider(in(orgB), provider(orgB, projectA, webtool.KindExa, 2))
	assert.True(t, webtool.IsNotFoundError(err), "another org cannot write org A's project row: %v", err)
	_, err = f.store.Provider(in(orgB), projectA)
	assert.True(t, webtool.IsNotFoundError(err))
	got, err := f.store.Provider(in(orgA), projectA)
	require.NoError(t, err)
	assert.Equal(t, webtool.KindBrave, got.Kind, "org A's row is unchanged")
}

func TestPostgres_SecretUpsertKeepsTheRow(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)

	first, err := f.store.SaveSecret(ctx, secret(t, orgID, projectID, "api.example.com", "x-api-key", 1))
	require.NoError(t, err)
	second, err := f.store.SaveSecret(ctx, secret(t, orgID, projectID, "api.example.com", "x-api-key", 2))
	require.NoError(t, err)
	assert.Equal(t, first.ID, second.ID)
	assert.Equal(t, first.PublicID, second.PublicID)
	assert.Equal(t, []byte{2}, second.ValueSealed)

	list, err := f.store.Secrets(ctx, projectID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, []byte{2}, list[0].ValueSealed)
}

func TestPostgres_SecretsOrderedAndFilteredByHost(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)
	for _, s := range []*webtool.HTTPSecret{
		secret(t, orgID, projectID, "b.example.com", "x-key", 1),
		secret(t, orgID, projectID, "a.example.com", "y-key", 2),
		secret(t, orgID, projectID, "a.example.com", "x-key", 3),
	} {
		_, err := f.store.SaveSecret(ctx, s)
		require.NoError(t, err)
	}
	list, err := f.store.Secrets(ctx, projectID)
	require.NoError(t, err)
	var keys []string
	for _, s := range list {
		keys = append(keys, s.Host+"|"+s.Header)
	}
	assert.Equal(t, []string{"a.example.com|x-key", "a.example.com|y-key", "b.example.com|x-key"}, keys)

	only, err := f.store.SecretsForHosts(ctx, projectID, []string{"b.example.com", "c.example.com"})
	require.NoError(t, err)
	require.Len(t, only, 1)
	assert.Equal(t, "b.example.com", only[0].Host)

	none, err := f.store.SecretsForHosts(ctx, projectID, nil)
	require.NoError(t, err)
	assert.Empty(t, none)

	total, exists, err := f.store.SecretCount(ctx, projectID, "a.example.com", "y-key")
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.True(t, exists)
	total, exists, err = f.store.SecretCount(ctx, projectID, "a.example.com", "z-key")
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.False(t, exists)
}

func TestPostgres_SecretConstraints(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)

	mixed := secret(t, orgID, projectID, "api.example.com", "x-api-key", 1)
	mixed.Header = "X-Api-Key"
	_, err := f.store.SaveSecret(ctx, mixed)
	assert.Error(t, err, "a mixed-case header is refused by the table")

	otherOrg, _ := f.seedScope(t)
	_, err = f.store.SaveSecret(in(otherOrg), secret(t, otherOrg, projectID, "api.example.com", "x-api-key", 1))
	assert.Error(t, err, "a project of another org is refused")
}

func TestPostgres_SecretUpsertOfAnotherOrgChangesNothing(t *testing.T) {
	f := newPgFixture(t)
	orgA, projectA := f.seedScope(t)
	orgB, _ := f.seedScope(t)
	_, err := f.store.SaveSecret(in(orgA), secret(t, orgA, projectA, "api.example.com", "x-api-key", 1))
	require.NoError(t, err)

	hostile := secret(t, orgA, projectA, "api.example.com", "x-api-key", 9)
	_, err = f.store.SaveSecret(in(orgB), hostile)
	assert.True(t, webtool.IsNotFoundError(err), "%v", err)

	list, err := f.store.Secrets(in(orgA), projectA)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, []byte{1}, list[0].ValueSealed)
}

func TestPostgres_ResolveAndDeleteSecret(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	saved, err := f.store.SaveSecret(in(orgID), secret(t, orgID, projectID, "api.example.com", "x-api-key", 1))
	require.NoError(t, err)

	ref, err := f.store.ResolveSecret(context.Background(), saved.PublicID)
	require.NoError(t, err)
	assert.Equal(t, webtool.Ref{ID: saved.ID, OrgID: orgID, ProjectID: projectID}, ref)
	_, err = f.store.ResolveSecret(context.Background(), saved.ID.String())
	assert.True(t, webtool.IsNotFoundError(err), "a uuid is not a public id")

	otherOrg, _ := f.seedScope(t)
	assert.True(t, webtool.IsNotFoundError(f.store.DeleteSecret(in(otherOrg), saved.ID)))
	require.NoError(t, f.store.DeleteSecret(in(orgID), saved.ID))
	assert.True(t, webtool.IsNotFoundError(f.store.DeleteSecret(in(orgID), saved.ID)))
}

func TestPostgres_ProjectDeleteCascades(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)
	require.NoError(t, f.store.SaveProvider(ctx, provider(orgID, projectID, webtool.KindBrave, 1)))
	_, err := f.store.SaveSecret(ctx, secret(t, orgID, projectID, "api.example.com", "x-api-key", 1))
	require.NoError(t, err)

	_, err = f.admin.ExecContext(context.Background(), `DELETE FROM `+f.schema+`.projects WHERE id = $1`, projectID)
	require.NoError(t, err)
	_, err = f.store.Provider(ctx, projectID)
	assert.True(t, webtool.IsNotFoundError(err))
	list, err := f.store.Secrets(ctx, projectID)
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestPostgres_WebToolHonorsUnitOfWork(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)
	uow := db.NewTransactor(f.pool)
	err := uow.RunInTx(ctx, func(ctx context.Context) error {
		if err := f.store.SaveProvider(ctx, provider(orgID, projectID, webtool.KindBrave, 1)); err != nil {
			return err
		}
		if _, err := f.store.SaveSecret(ctx, secret(t, orgID, projectID, "api.example.com", "x-api-key", 1)); err != nil {
			return err
		}
		return assert.AnError
	})
	require.ErrorIs(t, err, assert.AnError)
	_, err = f.store.Provider(ctx, projectID)
	assert.True(t, webtool.IsNotFoundError(err), "the rolled-back provider is gone")
	list, err := f.store.Secrets(ctx, projectID)
	require.NoError(t, err)
	assert.Empty(t, list)
}

// SECURITY: the conflict clauses' org guards hold on their own, with row level security bypassed by the owner role.
func TestPostgres_UpsertOrgGuardsHoldWithoutRLS(t *testing.T) {
	f := newPgFixture(t)
	orgA, projectA := f.seedScope(t)
	orgB, _ := f.seedScope(t)
	owner := webtool.NewStore(db.Pool{W: f.admin, R: f.admin}, f.schema, "")
	require.NoError(t, owner.SaveProvider(in(orgA), provider(orgA, projectA, webtool.KindBrave, 1)))
	_, err := owner.SaveSecret(in(orgA), secret(t, orgA, projectA, "api.example.com", "x-api-key", 1))
	require.NoError(t, err)

	forgedProvider := provider(orgB, projectA, webtool.KindExa, 2)
	assert.True(t, webtool.IsNotFoundError(owner.SaveProvider(in(orgB), forgedProvider)))
	forgedSecret := secret(t, orgB, projectA, "api.example.com", "x-api-key", 9)
	_, err = owner.SaveSecret(in(orgB), forgedSecret)
	assert.True(t, webtool.IsNotFoundError(err), "%v", err)

	got, err := owner.Provider(in(orgA), projectA)
	require.NoError(t, err)
	assert.Equal(t, webtool.KindBrave, got.Kind)
	list, err := owner.Secrets(in(orgA), projectA)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, []byte{1}, list[0].ValueSealed)
}
