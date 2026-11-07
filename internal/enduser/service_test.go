package enduser_test

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/enduser"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/testutil/fakes"
)

type fixture struct {
	store     *fakes.EndUserStore
	svc       *enduser.Service
	ctx       context.Context
	orgID     uuid.UUID
	projectID uuid.UUID
	logs      *bytes.Buffer
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{store: &fakes.EndUserStore{}, orgID: uuid.New(), projectID: uuid.New(), logs: &bytes.Buffer{}}
	f.ctx = tenant.Into(context.Background(), tenant.Context{OrgID: f.orgID})
	f.svc = enduser.NewService(f.store, fakes.Transactor{}, apperror.NewReporter(nil, false).Unexpected).
		WithLogger(slog.New(slog.NewTextHandler(f.logs, nil)))
	return f
}

var (
	phone = enduser.Identity{Kind: enduser.KindWAPhone, Value: "6281234567890"}
	lid   = enduser.Identity{Kind: enduser.KindWALid, Value: "123456789@lid"}
)

func TestUpsert_NewIdentityCreates(t *testing.T) {
	f := newFixture(t)
	e, err := f.svc.Upsert(f.ctx, f.projectID, phone)
	require.NoError(t, err)
	assert.Equal(t, f.orgID, e.OrgID)
	assert.Equal(t, f.projectID, e.ProjectID)
	assert.Len(t, f.store.Users, 1)
	assert.Equal(t, e.ID, f.store.Identities[f.projectID][phone])
}

func TestUpsert_KnownIdentityResolves(t *testing.T) {
	f := newFixture(t)
	first, err := f.svc.Upsert(f.ctx, f.projectID, phone)
	require.NoError(t, err)
	again, err := f.svc.Upsert(f.ctx, f.projectID, phone)
	require.NoError(t, err)
	assert.Equal(t, first.ID, again.ID)
	assert.Len(t, f.store.Users, 1)

	other, err := f.svc.Upsert(f.ctx, uuid.New(), phone)
	require.NoError(t, err)
	assert.NotEqual(t, first.ID, other.ID, "the same identity in another project is another end user")
}

func TestUpsert_TwoIdentitiesLink(t *testing.T) {
	f := newFixture(t)
	first, err := f.svc.Upsert(f.ctx, f.projectID, phone)
	require.NoError(t, err)
	linked, err := f.svc.Upsert(f.ctx, f.projectID, phone, lid)
	require.NoError(t, err)
	assert.Equal(t, first.ID, linked.ID, "a new identity arriving with a known one joins its end user")
	assert.Equal(t, first.ID, f.store.Identities[f.projectID][lid])

	both, err := f.svc.Upsert(f.ctx, f.projectID, enduser.Identity{Kind: enduser.KindAPIUser, Value: "u-1"}, enduser.Identity{Kind: enduser.KindWidgetSub, Value: "s-1"})
	require.NoError(t, err)
	assert.Equal(t, both.ID, f.store.Identities[f.projectID][enduser.Identity{Kind: enduser.KindWidgetSub, Value: "s-1"}], "two new identities arriving together are one end user")
	assert.Len(t, f.store.Users, 2)
}

func TestUpsert_ConflictLogsWithoutValue(t *testing.T) {
	f := newFixture(t)
	a, err := f.svc.Upsert(f.ctx, f.projectID, phone)
	require.NoError(t, err)
	b, err := f.svc.Upsert(f.ctx, f.projectID, lid)
	require.NoError(t, err)
	require.NotEqual(t, a.ID, b.ID)

	got, err := f.svc.Upsert(f.ctx, f.projectID, phone, lid)
	require.NoError(t, err)
	assert.Equal(t, a.ID, got.ID, "the first identity's end user wins")
	assert.Equal(t, b.ID, f.store.Identities[f.projectID][lid], "no merge")
	assert.Len(t, f.store.Users, 2)
	logs := f.logs.String()
	assert.Contains(t, logs, "wa_phone")
	assert.Contains(t, logs, "wa_lid")
	assert.NotContains(t, logs, phone.Value, "a phone number never reaches the logs")
	assert.NotContains(t, logs, lid.Value)
	assert.Contains(t, logs, a.PublicID, "the conflict names the end users by public id")
	assert.Contains(t, logs, b.PublicID)
	assert.NotContains(t, logs, a.ID.String(), "never the internal id")
}

func TestSetDisplayName(t *testing.T) {
	f := newFixture(t)
	e, err := f.svc.Upsert(f.ctx, f.projectID, phone)
	require.NoError(t, err)
	require.NoError(t, f.svc.SetDisplayName(f.ctx, e.ID, "Budi"))
	got, err := f.svc.Get(f.ctx, e.ID)
	require.NoError(t, err)
	assert.Equal(t, "Budi", got.DisplayName)
	assert.True(t, enduser.IsNotFoundError(f.svc.SetDisplayName(f.ctx, uuid.New(), "x")))
}

func TestUpsert_InvalidIdentityIsEUS002(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.Upsert(f.ctx, f.projectID, enduser.Identity{Kind: "email", Value: "a@b.c"})
	ae := err.(interface{ ToAppError() *apperror.AppError }).ToAppError()
	assert.Equal(t, apperror.CodeEndUserInvalidIdentity, ae.Code())
	assert.Equal(t, "kind", ae.Meta()["reason"])
	_, err = f.svc.Upsert(f.ctx, f.projectID)
	assert.True(t, enduser.IsInvalidIdentityError(err), "no identity at all")
	assert.Empty(t, f.store.Users)
}

func TestUpsert_NeedsATenant(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.Upsert(context.Background(), f.projectID, phone)
	assert.Error(t, err)
}

func TestGet(t *testing.T) {
	f := newFixture(t)
	e, err := f.svc.Upsert(f.ctx, f.projectID, phone)
	require.NoError(t, err)
	got, err := f.svc.Get(f.ctx, e.ID)
	require.NoError(t, err)
	assert.Equal(t, e.ID, got.ID)
	_, err = f.svc.Get(f.ctx, uuid.New())
	ae := err.(interface{ ToAppError() *apperror.AppError }).ToAppError()
	assert.Equal(t, apperror.CodeEndUserNotFound, ae.Code())
}

func TestStoreFailureIsUnexpected(t *testing.T) {
	f := newFixture(t)
	f.store.Err = assert.AnError
	_, err := f.svc.Upsert(f.ctx, f.projectID, phone)
	ae, ok := apperror.AsAppError(err)
	require.True(t, ok)
	assert.Equal(t, apperror.CodeUnexpected, ae.Code())
}

func TestUpsert_RepeatedIdentityIsOne(t *testing.T) {
	f := newFixture(t)
	e, err := f.svc.Upsert(f.ctx, f.projectID, phone, lid, phone)
	require.NoError(t, err)
	assert.Equal(t, e.ID, f.store.Identities[f.projectID][phone])
	assert.Equal(t, e.ID, f.store.Identities[f.projectID][lid])
	assert.Len(t, f.store.Users, 1)
}

func TestLookup_FindsWithoutCreating(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.Lookup(f.ctx, f.projectID, phone)
	assert.True(t, enduser.IsNotFoundError(err), "an unknown identity is not found")
	assert.Empty(t, f.store.Users, "and nothing is created")
	e, err := f.svc.Upsert(f.ctx, f.projectID, phone)
	require.NoError(t, err)
	got, err := f.svc.Lookup(f.ctx, f.projectID, phone)
	require.NoError(t, err)
	assert.Equal(t, e.ID, got.ID)
	_, err = f.svc.Lookup(f.ctx, f.projectID, enduser.Identity{Kind: "bogus", Value: "x"})
	assert.True(t, enduser.IsInvalidIdentityError(err))
}

func TestName_NamesTheEndUserWithoutTheValue(t *testing.T) {
	f := newFixture(t)
	e, err := f.svc.Upsert(f.ctx, f.projectID, phone)
	require.NoError(t, err)
	got, err := f.svc.Name(f.ctx, e.ID)
	require.NoError(t, err)
	assert.Equal(t, enduser.Name{Kind: enduser.KindWAPhone, Hint: "90"}, got)
	require.NoError(t, f.svc.SetDisplayName(f.ctx, e.ID, "Sari Wulandari"))
	got, err = f.svc.Name(f.ctx, e.ID)
	require.NoError(t, err)
	assert.Equal(t, enduser.Name{DisplayName: "Sari Wulandari", Kind: enduser.KindWAPhone, Hint: "90"}, got)
	gone, err := f.svc.Name(f.ctx, uuid.New())
	require.NoError(t, err)
	assert.Equal(t, enduser.Name{}, gone, "an end user that is gone has no name")
}

type keyHints map[uuid.UUID]string

func (k keyHints) KeyHint(_ context.Context, id uuid.UUID) (string, error) { return k[id], nil }

func TestName_AKeyActorShowsTheKeysHint(t *testing.T) {
	f := newFixture(t)
	keyID := uuid.New()
	f.svc.WithKeyHints(keyHints{keyID: "Zz09"})
	e, err := f.svc.Upsert(f.ctx, f.projectID, enduser.Identity{Kind: enduser.KindAPIUser, Value: "key:" + keyID.String()})
	require.NoError(t, err)
	got, err := f.svc.Name(f.ctx, e.ID)
	require.NoError(t, err)
	assert.Equal(t, enduser.Name{Kind: enduser.KindAPIKey, Hint: "Zz09"}, got, "the key's public handle, never its id")

	gone := uuid.New()
	e2, err := f.svc.Upsert(f.ctx, f.projectID, enduser.Identity{Kind: enduser.KindAPIUser, Value: "key:" + gone.String()})
	require.NoError(t, err)
	got, err = f.svc.Name(f.ctx, e2.ID)
	require.NoError(t, err)
	assert.Equal(t, enduser.Name{Kind: enduser.KindAPIKey}, got, "a key that is gone shows only the kind")
}
