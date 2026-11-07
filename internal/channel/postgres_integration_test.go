//go:build integration

package channel_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/channel"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/testutil/fakes"
	"altalune.id/oagent/internal/testutil/pgtest"
	"altalune.id/oagent/scheduler"
	"altalune.id/oagent/sealer"
)

type pgWorld struct {
	admin  *sql.DB
	schema string
	store  channel.Store
	pool   db.Pool
	sl     sealer.Sealer
	pub    *fakes.Publisher
	refs   *fakes.PublicIDs
	svc    *channel.Service
}

type tenantScope struct{ org, project, agent uuid.UUID }

func newPgWorld(t *testing.T) *pgWorld {
	t.Helper()
	pool, admin, schemaName := pgtest.MigratedRLS(t)
	key, err := sealer.GenerateKey()
	require.NoError(t, err)
	sl, err := sealer.New(key)
	require.NoError(t, err)
	w := &pgWorld{admin: admin, schema: schemaName, pool: pool, store: channel.NewStore(pool, schemaName, ""), sl: sl, pub: &fakes.Publisher{}, refs: fakes.NewPublicIDs()}
	w.svc = channel.NewService(w.store, db.NewTransactor(pool), w.refs, apperror.NewReporter(nil, false).Unexpected).
		WithSealer(sl).WithPublisher(w.pub)
	return w
}

func (w *pgWorld) seed(t *testing.T) tenantScope {
	t.Helper()
	s := tenantScope{org: uuid.New(), project: uuid.New(), agent: uuid.New()}
	p := w.schema + "."
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO ` + p + `orgs (id, public_id, slug, name, created_at, updated_at) VALUES ($1, $2, $2, 'O', now(), now())`, []any{s.org, "o" + s.org.String()[:12]}},
		{`INSERT INTO ` + p + `projects (id, public_id, org_id, slug, name, created_at, updated_at) VALUES ($1, $2, $3, 'p', 'P', now(), now())`, []any{s.project, "p" + s.project.String()[:12], s.org}},
		{`INSERT INTO ` + p + `agents (id, public_id, org_id, project_id, slug, name, created_at, updated_at) VALUES ($1, $2, $3, $4, 'a', 'A', now(), now())`, []any{s.agent, "a" + s.agent.String()[:12], s.org, s.project}},
	} {
		_, err := w.admin.ExecContext(t.Context(), q.sql, q.args...)
		require.NoError(t, err)
	}
	w.refs.Org(s.org)
	w.refs.Project(s.project, s.org)
	w.refs.Resource(s.agent, s.org)
	return s
}

func in(s tenantScope) context.Context {
	return tenant.Into(context.Background(), tenant.Context{OrgID: s.org, ProjectID: s.project})
}

func TestService_SealsSecretsAndOpensThem(t *testing.T) {
	w := newPgWorld(t)
	s := w.seed(t)
	c, err := create(in(s), w.svc, s.project, s.agent, input(), "whsec_primary_1", "owk_device_key")
	require.NoError(t, err)
	var primary, key []byte
	require.NoError(t, w.admin.QueryRowContext(t.Context(), `SELECT signing_secret_primary_sealed, device_key_sealed FROM `+w.schema+`.channel_connections WHERE id = $1`, c.ID).Scan(&primary, &key))
	assert.NotContains(t, string(primary), "whsec_primary_1", "sealed at rest")
	assert.NotContains(t, string(key), "owk_device_key")

	got, err := w.svc.Get(in(s), c.ID)
	require.NoError(t, err)
	secrets, err := w.svc.OpenSecrets(in(s), got)
	require.NoError(t, err)
	assert.Equal(t, []string{"whsec_primary_1"}, secrets.Signing)
	assert.Equal(t, "owk_device_key", secrets.DeviceKey)

	_, err = create(in(s), w.svc, s.project, s.agent, input(), " ", "owk")
	assert.True(t, channel.IsSecretRequiredError(err), "%v", err)
}

func TestService_SealedBytesNeverOpenOnAnotherRowSlotOrOrg(t *testing.T) {
	w := newPgWorld(t)
	s := w.seed(t)
	a, err := create(in(s), w.svc, s.project, s.agent, input(), "whsec_a", "owk_a")
	require.NoError(t, err)
	b, err := create(in(s), w.svc, s.project, s.agent, input(), "whsec_b", "owk_b")
	require.NoError(t, err)

	_, err = w.admin.ExecContext(t.Context(), `UPDATE `+w.schema+`.channel_connections SET signing_secret_primary_sealed = (SELECT signing_secret_primary_sealed FROM `+w.schema+`.channel_connections WHERE id = $1) WHERE id = $2`, a.ID, b.ID)
	require.NoError(t, err)
	got, err := w.svc.Get(in(s), b.ID)
	require.NoError(t, err)
	_, err = w.svc.OpenSecrets(in(s), got)
	assert.True(t, channel.IsSecretsUnavailableError(err), "another row's bytes never open: %v", err)

	_, err = w.admin.ExecContext(t.Context(), `UPDATE `+w.schema+`.channel_connections SET signing_secret_primary_sealed = device_key_sealed WHERE id = $1`, a.ID)
	require.NoError(t, err)
	got, err = w.svc.Get(in(s), a.ID)
	require.NoError(t, err)
	_, err = w.svc.OpenSecrets(in(s), got)
	assert.True(t, channel.IsSecretsUnavailableError(err), "a swapped slot never opens: %v", err)

	other := w.seed(t)
	_, err = w.svc.Get(in(other), a.ID)
	assert.True(t, channel.IsNotFoundError(err), "another org sees nothing: %v", err)
}

func TestService_RotationKeepsTheOldSecretValidInTheSecondarySlot(t *testing.T) {
	w := newPgWorld(t)
	s := w.seed(t)
	c, err := create(in(s), w.svc, s.project, s.agent, input(), "whsec_old", "owk")
	require.NoError(t, err)
	c, err = w.svc.SetSecret(in(s), c.ID, "whsec_new", false)
	require.NoError(t, err)
	secrets, err := w.svc.OpenSecrets(in(s), c)
	require.NoError(t, err)
	assert.Equal(t, []string{"whsec_new", "whsec_old"}, secrets.Signing, "OpenWA may still sign with the old one during its rotation")
	c, err = w.svc.SetSecret(in(s), c.ID, "whsec_newest", false)
	require.NoError(t, err)
	secrets, err = w.svc.OpenSecrets(in(s), c)
	require.NoError(t, err)
	assert.Equal(t, []string{"whsec_newest", "whsec_new"}, secrets.Signing, "only one previous secret is kept")
	assert.Equal(t, "owk", secrets.DeviceKey)
}

func TestService_UpdateKeepsSecretsWhenNil(t *testing.T) {
	w := newPgWorld(t)
	s := w.seed(t)
	c, err := create(in(s), w.svc, s.project, s.agent, input(), "whsec_1", "owk_1")
	require.NoError(t, err)
	next := input()
	next.QuietWindow = new(5 * time.Second)
	next.Status = channel.StatusDisabled
	c, err = w.svc.Update(in(s), c.ID, next, nil)
	require.NoError(t, err)
	assert.Equal(t, 5*time.Second, c.QuietWindow)
	assert.Equal(t, channel.StatusDisabled, c.Status)
	secrets, err := w.svc.OpenSecrets(in(s), c)
	require.NoError(t, err)
	assert.Equal(t, "owk_1", secrets.DeviceKey, "a nil key keeps the stored one")
	c, err = w.svc.Update(in(s), c.ID, next, new("owk_2"))
	require.NoError(t, err)
	secrets, err = w.svc.OpenSecrets(in(s), c)
	require.NoError(t, err)
	assert.Equal(t, "owk_2", secrets.DeviceKey)
}

func TestService_EveryWriteEmitsItsEventInTheSameTransaction(t *testing.T) {
	w := newPgWorld(t)
	s := w.seed(t)
	c, err := w.svc.Create(in(s), s.project, s.agent, input(), "owk")
	require.NoError(t, err)
	_, err = w.svc.Update(in(s), c.ID, input(), nil)
	require.NoError(t, err)
	_, err = w.svc.SetSecret(in(s), c.ID, "whsec_2", false)
	require.NoError(t, err)
	require.NoError(t, w.svc.Delete(in(s), c.ID))
	var types []events.Type
	for _, e := range w.pub.Events() {
		types = append(types, e.Type)
	}
	assert.Equal(t, []events.Type{events.ChannelConnectionCreated, events.ChannelConnectionUpdated, events.ChannelConnectionUpdated, events.ChannelConnectionDeleted}, types)
	created, ok := w.pub.Events()[0].Data.(events.ChannelConnectionCreatedV1)
	require.True(t, ok)
	assert.Equal(t, c.PublicID, created.ID)
	assert.Equal(t, w.refs.Project(s.project, s.org), created.ProjectID, "public ids only")
	assert.NotEmpty(t, created.AgentID, "an event always names its agent")
	assert.Equal(t, "waiting", created.Status)

	w.pub.Err = errors.New("outbox down")
	_, err = w.svc.Create(in(s), s.project, s.agent, input(), "owk")
	require.Error(t, err)
	var n int
	require.NoError(t, w.admin.QueryRowContext(t.Context(), `SELECT count(*) FROM `+w.schema+`.channel_connections WHERE org_id = $1`, s.org).Scan(&n))
	assert.Zero(t, n, "a write whose event fails is rolled back")

	w.pub.Err = nil
	kept, err := create(in(s), w.svc, s.project, s.agent, input(), "whsec", "owk")
	require.NoError(t, err)
	w.pub.Err = errors.New("outbox down")
	disabled := input()
	disabled.Status = channel.StatusDisabled
	_, err = w.svc.Update(in(s), kept.ID, disabled, nil)
	require.Error(t, err)
	require.Error(t, w.svc.Delete(in(s), kept.ID))
	got, err := w.svc.Get(in(s), kept.ID)
	require.NoError(t, err, "a delete whose event fails is rolled back")
	assert.Equal(t, channel.StatusActive, got.Status, "an update whose event fails is rolled back")
}

func TestService_ResolveWithoutATenantThenBindsIt(t *testing.T) {
	w := newPgWorld(t)
	s := w.seed(t)
	c, err := create(in(s), w.svc, s.project, s.agent, input(), "whsec", "owk")
	require.NoError(t, err)
	got, err := w.svc.Resolve(context.Background(), c.PublicID)
	require.NoError(t, err)
	assert.Equal(t, c.ID, got.ID)
	assert.Equal(t, s.org, got.OrgID)
	assert.Equal(t, "https://wa.example.com", got.BaseURL, "the full row is read after the tenant is bound")
	_, err = w.svc.Resolve(context.Background(), "nope")
	assert.True(t, channel.IsNotFoundError(err), "%v", err)
}

func TestService_AgentOfAnotherProjectIsRefused(t *testing.T) {
	w := newPgWorld(t)
	s := w.seed(t)
	other := w.seed(t)
	_, err := create(in(s), w.svc, s.project, other.agent, input(), "whsec", "owk")
	var invalid *channel.InvalidError
	require.ErrorAs(t, err, &invalid)
	assert.Equal(t, "agent_id", invalid.Field)
}

func TestService_ListMissingRowsAndRefusals(t *testing.T) {
	w := newPgWorld(t)
	s := w.seed(t)
	a, err := create(in(s), w.svc, s.project, s.agent, input(), "whsec", "owk")
	require.NoError(t, err)
	b, err := create(in(s), w.svc, s.project, s.agent, input(), "whsec", "owk")
	require.NoError(t, err)
	list, err := w.svc.List(in(s), s.project)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, []uuid.UUID{a.ID, b.ID}, []uuid.UUID{list[0].ID, list[1].ID}, "oldest first")
	other := w.seed(t)
	empty, err := w.svc.List(in(other), s.project)
	require.NoError(t, err)
	assert.Empty(t, empty, "another tenant sees none")

	missing := uuid.New()
	_, err = w.svc.Get(in(s), missing)
	assert.True(t, channel.IsNotFoundError(err))
	_, err = w.svc.Update(in(s), missing, input(), nil)
	assert.True(t, channel.IsNotFoundError(err))
	_, err = w.svc.SetSecret(in(s), missing, "x", false)
	assert.True(t, channel.IsNotFoundError(err))
	assert.True(t, channel.IsNotFoundError(w.svc.Delete(in(s), missing)))
	assert.True(t, channel.IsNotFoundError(w.svc.Delete(in(other), a.ID)), "another org deletes nothing")

	bad := input()
	bad.BaseURL = "ftp://wa.example.com"
	_, err = w.svc.Update(in(s), a.ID, bad, nil)
	assert.True(t, channel.IsInvalidError(err))
	_, err = w.svc.Update(in(s), a.ID, input(), new(" "))
	assert.True(t, channel.IsSecretRequiredError(err), "an empty key is not a key")
	_, err = w.svc.SetSecret(in(s), a.ID, "  ", false)
	assert.True(t, channel.IsSecretRequiredError(err))
	_, err = create(in(s), w.svc, s.project, s.agent, bad, "whsec", "owk")
	assert.True(t, channel.IsInvalidError(err))
	_, err = create(context.Background(), w.svc, s.project, s.agent, input(), "whsec", "owk")
	assert.Error(t, err, "no tenant, no write")

	require.NoError(t, w.svc.Delete(in(s), a.ID))
	_, err = w.svc.Get(in(s), a.ID)
	assert.True(t, channel.IsNotFoundError(err))
}

func TestService_WithoutASealerNothingIsSealedOrOpened(t *testing.T) {
	w := newPgWorld(t)
	s := w.seed(t)
	plain := channel.NewService(w.store, db.NewTransactor(w.pool), w.refs, apperror.NewReporter(nil, false).Unexpected).WithAllowPrivateHosts(true)
	_, err := plain.Create(in(s), s.project, s.agent, input(), "owk")
	assert.True(t, channel.IsSecretsUnavailableError(err), "%v", err)
	c, err := create(in(s), w.svc, s.project, s.agent, input(), "whsec", "owk")
	require.NoError(t, err)
	_, err = plain.OpenSecrets(in(s), c)
	assert.True(t, channel.IsSecretsUnavailableError(err))
	_, err = plain.SetSecret(in(s), c.ID, "whsec_2", false)
	assert.True(t, channel.IsSecretsUnavailableError(err))
	private := input()
	private.BaseURL = "http://openwa.internal:8080"
	_, err = channel.NewService(w.store, db.NewTransactor(w.pool), w.refs, nil).WithSealer(w.sl).WithAllowPrivateHosts(true).
		Create(in(s), s.project, s.agent, private, "owk")
	assert.NoError(t, err, "the operator flag allows http")
}

func TestNewService_RequiresRefs(t *testing.T) {
	assert.Panics(t, func() { channel.NewService(nil, nil, nil, nil) }, "events never go out without public ids")
}

func TestSchema_NoSecondarySecretWithoutAPrimary(t *testing.T) {
	w := newPgWorld(t)
	s := w.seed(t)
	c, err := w.svc.Create(in(s), s.project, s.agent, input(), "owk")
	require.NoError(t, err)
	var primary []byte
	require.NoError(t, w.admin.QueryRowContext(t.Context(), `SELECT signing_secret_primary_sealed FROM `+w.schema+`.channel_connections WHERE id = $1`, c.ID).Scan(&primary))
	assert.Nil(t, primary, "a new connection has no secret until OpenWA mints one")
	_, err = w.admin.ExecContext(t.Context(), `UPDATE `+w.schema+`.channel_connections SET signing_secret_secondary_sealed = '\x01' WHERE id = $1`, c.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "channel_connections_secondary_needs_primary")
}

func TestPruneJob_DropsDeliveryIDsOlderThan48h(t *testing.T) {
	w := newPgWorld(t)
	s := w.seed(t)
	c, err := w.svc.Create(in(s), s.project, s.agent, input(), "owk")
	require.NoError(t, err)
	now := time.Now()
	for id, at := range map[string]time.Time{"dlv_old": now.Add(-49 * time.Hour), "dlv_edge": now.Add(-47 * time.Hour), "dlv_new": now} {
		ok, err := w.store.InsertDelivery(in(s), c, id, at)
		require.NoError(t, err)
		require.True(t, ok)
	}
	jobs := w.svc.SchedulerJobs()
	require.Len(t, jobs, 1)
	job := jobs[0]
	assert.Equal(t, "ingest-prune", job.Name)
	assert.Equal(t, scheduler.ScopeTenant, job.Scope)
	require.NoError(t, job.Run(in(s)))
	var ids []string
	rows, err := w.admin.QueryContext(t.Context(), `SELECT delivery_id FROM `+w.schema+`.ingest_deliveries ORDER BY delivery_id`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	assert.Equal(t, []string{"dlv_edge", "dlv_new"}, ids, "a redelivery of a pruned id is past OpenWA's retry window anyway")
}

func TestStore_RecordResultIsThrottledUnlessTheResultChanges(t *testing.T) {
	w := newPgWorld(t)
	s := w.seed(t)
	c, err := w.svc.Create(in(s), s.project, s.agent, input(), "owk")
	require.NoError(t, err)
	state := func() (string, time.Time) {
		var result string
		var at time.Time
		require.NoError(t, w.admin.QueryRowContext(t.Context(), `SELECT last_result, last_result_at FROM `+w.schema+`.channel_connections WHERE id = $1`, c.ID).Scan(&result, &at))
		return result, at.UTC()
	}
	t0 := time.Now().UTC().Truncate(time.Microsecond)
	require.NoError(t, w.store.RecordResult(in(s), c.ID, channel.ResultConnected, t0))

	require.NoError(t, w.store.RecordResult(in(s), c.ID, channel.ResultConnected, t0.Add(30*time.Second)), "a throttled call is no error")
	result, at := state()
	assert.Equal(t, channel.ResultConnected, result)
	assert.True(t, at.Equal(t0), "the same result within the minute writes nothing")

	for i, next := range []string{channel.ResultKeyRefused, channel.ResultDeviceMismatch, channel.ResultConnected} {
		when := t0.Add(time.Duration(i+31) * time.Second)
		require.NoError(t, w.store.RecordResult(in(s), c.ID, next, when))
		result, at = state()
		assert.Equal(t, next, result, "a changed result always writes")
		assert.True(t, at.Equal(when), next)
	}

	later := t0.Add(2 * time.Minute)
	require.NoError(t, w.store.RecordResult(in(s), c.ID, channel.ResultConnected, later))
	_, at = state()
	assert.True(t, at.Equal(later), "the same result a minute later moves the stamp")
}
