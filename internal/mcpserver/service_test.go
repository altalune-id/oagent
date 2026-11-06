package mcpserver_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/mcpserver"
	"altalune.id/oagent/internal/testutil/fakes"
	"altalune.id/oagent/sealer"
)

const authValue = "tok-live-secret"

func testUnexpected() apperror.UnexpectedFunc {
	return apperror.NewReporter(nil, false).Unexpected
}

func testSealer(t *testing.T) sealer.Sealer {
	t.Helper()
	key, err := sealer.GenerateKey()
	require.NoError(t, err)
	sl, err := sealer.New(key)
	require.NoError(t, err)
	return sl
}

func input(kind mcpserver.AuthKind) mcpserver.Input {
	return mcpserver.Input{Name: "yasaku", URL: "https://yasaku.example.com/mcp", AuthKind: kind, AuthHeader: "X-Api-Key", TimeoutMS: 5000}
}

type usageFake struct {
	inUse bool
	err   error
	fn    func(ctx context.Context, id uuid.UUID)
}

func (u *usageFake) ServerInUse(ctx context.Context, id uuid.UUID) (bool, error) {
	if u.fn != nil {
		u.fn(ctx, id)
	}
	return u.inUse, u.err
}

type fixture struct {
	store *fakes.MCPServerStore
	usage *usageFake
	tx    *fakes.CountingTransactor
	sl    sealer.Sealer
	svc   *mcpserver.Service
	ctx   context.Context
	orgID uuid.UUID
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{store: &fakes.MCPServerStore{}, usage: &usageFake{}, tx: &fakes.CountingTransactor{}, sl: testSealer(t), orgID: uuid.New()}
	f.ctx = tenant.Into(context.Background(), tenant.Context{OrgID: f.orgID})
	f.svc = mcpserver.NewService(f.store, f.tx, testUnexpected()).WithSealer(f.sl).WithUsage(f.usage)
	return f
}

func (f *fixture) create(t *testing.T, kind mcpserver.AuthKind) *mcpserver.Server {
	t.Helper()
	s, err := f.svc.Create(f.ctx, uuid.New(), input(kind), authValue)
	require.NoError(t, err)
	return s
}

type appErr interface {
	error
	ToAppError() *apperror.AppError
}

func codeOf(t *testing.T, err error) apperror.Code {
	t.Helper()
	require.Error(t, err)
	e, ok := errors.AsType[appErr](err)
	require.True(t, ok, "%T has no ToAppError: %v", err, err)
	return e.ToAppError().Code()
}

func reasonOf(t *testing.T, err error) string {
	t.Helper()
	e, ok := errors.AsType[appErr](err)
	require.True(t, ok, "%T: %v", err, err)
	return e.ToAppError().Meta()["reason"]
}

func TestService_CreateSealsAndNeverStoresPlainText(t *testing.T) {
	f := newFixture(t)
	for _, kind := range []mcpserver.AuthKind{mcpserver.AuthBearer, mcpserver.AuthHeader} {
		in := input(kind)
		in.Name = "s-" + string(kind)
		s, err := f.svc.Create(f.ctx, uuid.New(), in, "  "+authValue+"\n")
		require.NoError(t, err)
		saved := f.store.Servers[s.ID]
		require.NotEmpty(t, saved.AuthSealed)
		assert.False(t, bytes.Contains(saved.AuthSealed, []byte(authValue)))
		assert.Equal(t, f.orgID, saved.OrgID)
	}
}

func TestService_CreateWithoutAuthStoresNoSecret(t *testing.T) {
	f := newFixture(t)
	s, err := f.svc.Create(f.ctx, uuid.New(), input(mcpserver.AuthNone), "ignored")
	require.NoError(t, err)
	assert.Nil(t, f.store.Servers[s.ID].AuthSealed)
	assert.Empty(t, f.store.Servers[s.ID].AuthHeader)
}

func TestService_CreateBlankAuthValueIsSecretRequired(t *testing.T) {
	f := newFixture(t)
	for _, v := range []string{"", " \t\n"} {
		_, err := f.svc.Create(f.ctx, uuid.New(), input(mcpserver.AuthBearer), v)
		assert.Equal(t, apperror.CodeMcpServerSecretRequired, codeOf(t, err))
		assert.Equal(t, "secret_required", reasonOf(t, err))
	}
	assert.Empty(t, f.store.Servers)
}

func TestService_CreateBadHeaderNameIsHeaderName(t *testing.T) {
	f := newFixture(t)
	in := input(mcpserver.AuthHeader)
	in.AuthHeader = "Host"
	_, err := f.svc.Create(f.ctx, uuid.New(), in, authValue)
	assert.Equal(t, apperror.CodeMcpServerSecretRequired, codeOf(t, err))
	assert.Equal(t, "header_name", reasonOf(t, err))
}

func TestService_CreateNeedsATenant(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.Create(context.Background(), uuid.New(), input(mcpserver.AuthNone), "")
	assert.Error(t, err)
	assert.Empty(t, f.store.Servers)
}

type projectOrgs map[uuid.UUID]uuid.UUID

func (m projectOrgs) OrgOf(_ context.Context, id uuid.UUID) (uuid.UUID, error) {
	if org, ok := m[id]; ok {
		return org, nil
	}
	return uuid.Nil, &mcpserver.NotFoundError{}
}

func TestService_CreateProjectMustBelongToOrg(t *testing.T) {
	f := newFixture(t)
	foreign, own := uuid.New(), uuid.New()
	f.svc.WithProjects(projectOrgs{foreign: uuid.New(), own: f.orgID})
	_, err := f.svc.Create(f.ctx, foreign, input(mcpserver.AuthNone), "")
	assert.True(t, tenant.IsProjectOutsideOrgError(err), "got %v", err)
	assert.Equal(t, apperror.CodeProjectNotInOrg, codeOf(t, err), "the tenant error, not GEN900")
	_, err = f.svc.Create(f.ctx, own, input(mcpserver.AuthNone), "")
	require.NoError(t, err)
}

func TestService_DisabledSealerIsSecretsUnavailable(t *testing.T) {
	f := newFixture(t)
	f.svc.WithSealer(sealer.Disabled())
	_, err := f.svc.Create(f.ctx, uuid.New(), input(mcpserver.AuthBearer), authValue)
	assert.Equal(t, apperror.CodeMcpServerSecretsUnavailable, codeOf(t, err))
	assert.True(t, sealer.IsUnavailableError(err))

	svc := mcpserver.NewService(&fakes.MCPServerStore{}, fakes.Transactor{}, testUnexpected())
	_, err = svc.Create(f.ctx, uuid.New(), input(mcpserver.AuthBearer), authValue)
	assert.Equal(t, apperror.CodeMcpServerSecretsUnavailable, codeOf(t, err), "no sealer configured")
}

func TestService_UpdateWithNilValueKeepsSealedBytes(t *testing.T) {
	f := newFixture(t)
	s := f.create(t, mcpserver.AuthBearer)
	before := f.store.Servers[s.ID].AuthSealed
	in := input(mcpserver.AuthBearer)
	in.TimeoutMS = 9000
	got, err := f.svc.Update(f.ctx, s.ID, in, nil)
	require.NoError(t, err)
	assert.Equal(t, int32(9000), got.TimeoutMS)
	assert.Equal(t, before, f.store.Servers[s.ID].AuthSealed)
	assert.Equal(t, []string{"update:" + s.ID.String()}, f.store.Locks)
	assert.Equal(t, 1, f.tx.Calls)
}

func TestService_UpdateHeaderNameAloneKeepsTheValue(t *testing.T) {
	f := newFixture(t)
	s := f.create(t, mcpserver.AuthHeader)
	before := f.store.Servers[s.ID].AuthSealed
	in := input(mcpserver.AuthHeader)
	in.AuthHeader = "X-Other-Key"
	_, err := f.svc.Update(f.ctx, s.ID, in, nil)
	require.NoError(t, err)
	assert.Equal(t, "X-Other-Key", f.store.Servers[s.ID].AuthHeader)
	assert.Equal(t, before, f.store.Servers[s.ID].AuthSealed)
}

func TestService_UpdateNeedsAValue(t *testing.T) {
	cases := map[string]struct {
		from mcpserver.AuthKind
		edit func(*mcpserver.Input)
	}{
		"url change":       {mcpserver.AuthBearer, func(in *mcpserver.Input) { in.URL = "https://other.example.com/mcp" }},
		"none to bearer":   {mcpserver.AuthNone, func(in *mcpserver.Input) { in.AuthKind = mcpserver.AuthBearer }},
		"none to header":   {mcpserver.AuthNone, func(in *mcpserver.Input) { in.AuthKind = mcpserver.AuthHeader }},
		"bearer to header": {mcpserver.AuthBearer, func(in *mcpserver.Input) { in.AuthKind = mcpserver.AuthHeader }},
		"header to bearer": {mcpserver.AuthHeader, func(in *mcpserver.Input) { in.AuthKind = mcpserver.AuthBearer }},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			s := f.create(t, c.from)
			before := *f.store.Servers[s.ID]
			in := input(c.from)
			c.edit(&in)
			_, err := f.svc.Update(f.ctx, s.ID, in, nil)
			assert.Equal(t, apperror.CodeMcpServerSecretRequired, codeOf(t, err))
			assert.Equal(t, "secret_required", reasonOf(t, err))
			assert.Equal(t, before, *f.store.Servers[s.ID], "nothing saved")

			v := "  tok-new "
			_, err = f.svc.Update(f.ctx, s.ID, in, &v)
			require.NoError(t, err)
			assert.NotEqual(t, before.AuthSealed, f.store.Servers[s.ID].AuthSealed)
		})
	}
}

func TestService_UpdateURLOnANoAuthServerNeedsNoValue(t *testing.T) {
	f := newFixture(t)
	s := f.create(t, mcpserver.AuthNone)
	in := input(mcpserver.AuthNone)
	in.URL = "https://other.example.com/mcp"
	_, err := f.svc.Update(f.ctx, s.ID, in, nil)
	require.NoError(t, err)
}

func TestService_UpdateToNoneClearsTheSecret(t *testing.T) {
	for _, from := range []mcpserver.AuthKind{mcpserver.AuthBearer, mcpserver.AuthHeader} {
		t.Run(string(from), func(t *testing.T) {
			f := newFixture(t)
			s := f.create(t, from)
			_, err := f.svc.Update(f.ctx, s.ID, input(mcpserver.AuthNone), nil)
			require.NoError(t, err)
			assert.Nil(t, f.store.Servers[s.ID].AuthSealed)
			assert.Empty(t, f.store.Servers[s.ID].AuthHeader)
		})
	}
}

func TestService_AuthValueWithControlCharactersIsSecretRequired(t *testing.T) {
	f := newFixture(t)
	s := f.create(t, mcpserver.AuthBearer)
	before := f.store.Servers[s.ID].AuthSealed
	for _, v := range []string{"tok\r\nX-Evil: 1", "tok\nx", "tok\rx", "tok\x00x", "tok\x7fx", "tok\tx"} {
		_, err := f.svc.Create(f.ctx, uuid.New(), input(mcpserver.AuthBearer), v)
		assert.Equal(t, apperror.CodeMcpServerSecretRequired, codeOf(t, err), "create %q", v)
		assert.Equal(t, "secret_required", reasonOf(t, err))
		_, err = f.svc.Update(f.ctx, s.ID, input(mcpserver.AuthBearer), &v)
		assert.Equal(t, apperror.CodeMcpServerSecretRequired, codeOf(t, err), "update %q", v)
	}
	assert.Len(t, f.store.Servers, 1)
	assert.Equal(t, before, f.store.Servers[s.ID].AuthSealed)
}

func TestService_UpdateBlankValueIsSecretRequired(t *testing.T) {
	f := newFixture(t)
	s := f.create(t, mcpserver.AuthBearer)
	before := f.store.Servers[s.ID].AuthSealed
	blank := "   "
	_, err := f.svc.Update(f.ctx, s.ID, input(mcpserver.AuthBearer), &blank)
	assert.Equal(t, apperror.CodeMcpServerSecretRequired, codeOf(t, err))
	assert.Equal(t, before, f.store.Servers[s.ID].AuthSealed)
}

func TestUpdate_NameImmutable(t *testing.T) {
	f := newFixture(t)
	s := f.create(t, mcpserver.AuthNone)
	in := input(mcpserver.AuthNone)
	in.Name = "renamed"
	got, err := f.svc.Update(f.ctx, s.ID, in, nil)
	require.NoError(t, err)
	assert.Equal(t, "yasaku", got.Name)
	assert.Equal(t, "yasaku", f.store.Servers[s.ID].Name)
}

func TestService_UpdateMissingIsNotFound(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.Update(f.ctx, uuid.New(), input(mcpserver.AuthNone), nil)
	assert.Equal(t, apperror.CodeMcpServerNotFound, codeOf(t, err))
}

func TestService_DeleteInUseIsInUse(t *testing.T) {
	f := newFixture(t)
	s := f.create(t, mcpserver.AuthNone)
	f.usage.inUse = true
	err := f.svc.Delete(f.ctx, s.ID)
	assert.Equal(t, apperror.CodeMcpServerInUse, codeOf(t, err))
	assert.Contains(t, f.store.Servers, s.ID)
}

func TestService_DeleteLocksAndChecksUsageInOneUnitOfWork(t *testing.T) {
	f := newFixture(t)
	s := f.create(t, mcpserver.AuthNone)
	checked := false
	f.usage.fn = func(ctx context.Context, id uuid.UUID) {
		checked = true
		assert.Equal(t, s.ID, id)
		assert.True(t, fakes.InTx(ctx), "the usage check runs inside the unit of work")
		assert.Equal(t, []string{"update:" + s.ID.String()}, f.store.Locks, "after the row lock")
	}
	require.NoError(t, f.svc.Delete(f.ctx, s.ID))
	assert.True(t, checked)
	assert.Equal(t, 1, f.tx.Calls)
	assert.NotContains(t, f.store.Servers, s.ID)
}

func TestService_UsageFailureIsUnexpected(t *testing.T) {
	f := newFixture(t)
	s := f.create(t, mcpserver.AuthNone)
	f.usage.err = assert.AnError
	ae, ok := apperror.AsAppError(f.svc.Delete(f.ctx, s.ID))
	require.True(t, ok)
	assert.Equal(t, apperror.CodeUnexpected, ae.Code())
	assert.Contains(t, f.store.Servers, s.ID)
}

func TestService_GetListResolve(t *testing.T) {
	f := newFixture(t)
	projectID := uuid.New()
	s, err := f.svc.Create(f.ctx, projectID, input(mcpserver.AuthNone), "")
	require.NoError(t, err)
	got, err := f.svc.Get(f.ctx, s.ID)
	require.NoError(t, err)
	assert.Equal(t, s.ID, got.ID)
	list, err := f.svc.List(f.ctx, projectID)
	require.NoError(t, err)
	assert.Len(t, list, 1)
	ref, err := f.svc.Resolve(context.Background(), s.PublicID)
	require.NoError(t, err)
	assert.Equal(t, mcpserver.Ref{ID: s.ID, OrgID: f.orgID, ProjectID: projectID}, ref)
	_, err = f.svc.Resolve(context.Background(), "nope")
	assert.Equal(t, apperror.CodeMcpServerNotFound, codeOf(t, err))
}

func TestService_StoreFailureIsUnexpected(t *testing.T) {
	f := newFixture(t)
	f.store.Err = assert.AnError
	for name, call := range map[string]func() error{
		"create":  func() error { _, err := f.svc.Create(f.ctx, uuid.New(), input(mcpserver.AuthNone), ""); return err },
		"get":     func() error { _, err := f.svc.Get(f.ctx, uuid.New()); return err },
		"list":    func() error { _, err := f.svc.List(f.ctx, uuid.New()); return err },
		"resolve": func() error { _, err := f.svc.Resolve(f.ctx, "x"); return err },
		"delete":  func() error { return f.svc.Delete(f.ctx, uuid.New()) },
		"update":  func() error { _, err := f.svc.Update(f.ctx, uuid.New(), input(mcpserver.AuthNone), nil); return err },
		"tools":   func() error { _, err := f.svc.ListTools(f.ctx, uuid.New()); return err },
	} {
		t.Run(name, func(t *testing.T) {
			ae, ok := apperror.AsAppError(call())
			require.True(t, ok)
			assert.Equal(t, apperror.CodeUnexpected, ae.Code())
		})
	}
}

func TestService_OpenGivesTheCredentialForATurn(t *testing.T) {
	f := newFixture(t)
	s := f.create(t, mcpserver.AuthHeader)
	srv, value, err := f.svc.Open(f.ctx, s.ID)
	require.NoError(t, err)
	assert.Equal(t, s.ID, srv.ID)
	assert.Equal(t, authValue, value)

	in := input(mcpserver.AuthNone)
	in.Name = "open"
	none, err := f.svc.Create(f.ctx, uuid.New(), in, "")
	require.NoError(t, err)
	_, value, err = f.svc.Open(f.ctx, none.ID)
	require.NoError(t, err)
	assert.Empty(t, value)

	_, _, err = f.svc.Open(f.ctx, uuid.New())
	assert.Equal(t, apperror.CodeMcpServerNotFound, codeOf(t, err))

	locked := mcpserver.NewService(f.store, f.tx, testUnexpected())
	_, _, err = locked.Open(f.ctx, s.ID)
	assert.Equal(t, apperror.CodeMcpServerSecretsUnavailable, codeOf(t, err), "no sealer, no credential")
}
