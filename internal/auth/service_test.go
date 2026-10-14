package auth

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/testutil/fakes"
	"altalune.id/oagent/internal/user"
	"altalune.id/oagent/password"
)

type fakeUsers struct {
	byEmail map[string]*user.User
	calls   int
}

func (f *fakeUsers) ByEmail(_ context.Context, email string) (*user.User, error) {
	f.calls++
	if u, ok := f.byEmail[email]; ok {
		return u, nil
	}
	return nil, &user.NotFoundError{}
}

func (f *fakeUsers) FindByEmail(ctx context.Context, raw string) (*user.User, error) {
	email, err := user.NormalizeEmail(raw)
	if err != nil {
		return nil, err
	}
	u, err := f.ByEmail(ctx, email)
	if user.IsNotFoundError(err) {
		return nil, &user.UnknownEmailError{Email: email}
	}
	return u, err
}

type fakePasswords struct {
	users *fakeUsers
	calls []uuid.UUID
}

func (f *fakePasswords) SetPassword(_ context.Context, id uuid.UUID, plain string) error {
	f.calls = append(f.calls, id)
	if err := password.Check(plain, password.Hints{}); err != nil {
		return err
	}
	for _, u := range f.users.byEmail {
		if u.ID == id {
			h, err := password.Hash(plain)
			if err != nil {
				return err
			}
			u.PasswordHash = h
			return nil
		}
	}
	return &user.NotFoundError{}
}

type fakeMembers struct {
	byUser map[uuid.UUID][]*member.Membership
}

func (f *fakeMembers) ListByUser(_ context.Context, userID uuid.UUID) ([]*member.Membership, error) {
	return f.byUser[userID], nil
}

type fakeOrgs struct{ system *org.Org }

func (f *fakeOrgs) System(context.Context) (*org.Org, error) {
	if f.system == nil {
		return nil, &org.NotFoundError{}
	}
	return f.system, nil
}

type fakeProjects struct {
	byOrg map[uuid.UUID][]*project.Project
}

func (f *fakeProjects) List(_ context.Context, orgID uuid.UUID) ([]*project.Project, error) {
	return f.byOrg[orgID], nil
}

type fixture struct {
	svc       *Service
	users     *fakeUsers
	members   *fakeMembers
	orgs      *fakeOrgs
	projects  *fakeProjects
	sessions  *session.MemoryStore
	passwords *fakePasswords
	verifies  int
	dummies   int
	mail      *fakeMail
}

const testPassword = "correct horse battery"

func newFixture(t *testing.T, basic BasicCredentials) *fixture {
	t.Helper()
	f := &fixture{
		users:    &fakeUsers{byEmail: map[string]*user.User{}},
		members:  &fakeMembers{byUser: map[uuid.UUID][]*member.Membership{}},
		orgs:     &fakeOrgs{},
		projects: &fakeProjects{byOrg: map[uuid.UUID][]*project.Project{}},
		sessions: session.NewMemoryStore(),
	}
	f.passwords = &fakePasswords{users: f.users}
	f.mail = &fakeMail{}
	f.svc = NewService(Deps{
		Users: f.users, Members: f.members, Orgs: f.orgs, Projects: f.projects,
		Sessions: f.sessions, Passwords: f.passwords, Basic: basic,
		Profiles: fakeProfiles{f.users}, Transactor: fakes.Transactor{}, Mail: f.mail,
		From: "noreply@example.com", BaseURL: "https://oagent.example.com",
	})
	verify, dummy := f.svc.verify, f.svc.verifyDummy
	f.svc.verify = func(hash, plain string) error { f.verifies++; return verify(hash, plain) }
	f.svc.verifyDummy = func(plain string) { f.dummies++; dummy(plain) }
	return f
}

func (f *fixture) addUser(t *testing.T, email string, withPassword bool) *user.User {
	t.Helper()
	u, err := user.New(email, "Ada")
	require.NoError(t, err)
	if withPassword {
		u.PasswordHash, err = password.Hash(testPassword)
		require.NoError(t, err)
	}
	f.users.byEmail[u.Email] = u
	return u
}

func (f *fixture) addOrg(t *testing.T, system bool) *org.Org {
	t.Helper()
	o, err := org.NewOrg("acme-"+uuid.NewString()[:8], "Acme", uuid.Nil)
	require.NoError(t, err)
	if system {
		o.IsSystem = true
		f.orgs.system = o
	}
	return o
}

func (f *fixture) join(t *testing.T, o *org.Org, u *user.User, role member.Role) *member.Membership {
	t.Helper()
	m, err := member.New(o.ID, u.ID, role)
	require.NoError(t, err)
	f.members.byUser[u.ID] = append(f.members.byUser[u.ID], m)
	return m
}

func (f *fixture) addProject(o *org.Org) *project.Project {
	p := &project.Project{ID: uuid.New(), OrgID: o.ID}
	f.projects.byOrg[o.ID] = append(f.projects.byOrg[o.ID], p)
	return p
}

func TestLoginLocal_Success(t *testing.T) {
	f := newFixture(t, BasicCredentials{})
	u := f.addUser(t, "ada@example.com", true)
	o := f.addOrg(t, false)
	f.join(t, o, u, member.RoleMember)
	prj := f.addProject(o)

	p, err := f.svc.LoginLocal(context.Background(), Credentials{Email: "  ADA@Example.com ", Password: testPassword})
	require.NoError(t, err)
	assert.Equal(t, u.ID, p.UserID)
	assert.Equal(t, u.Email, p.Email)
	assert.Equal(t, u.Name, p.Name)
	assert.Equal(t, session.SourceLocal, p.Source)
	assert.Equal(t, o.ID, p.ActiveOrgID)
	assert.Equal(t, prj.ID, p.ActiveProjectID)
	assert.Equal(t, member.BuiltinPermissions(member.RoleMember), p.Scopes)
	assert.False(t, p.Operator)
	assert.WithinDuration(t, time.Now(), p.IssuedAt, time.Minute)
	assert.Equal(t, 1, f.verifies)
	assert.Equal(t, 0, f.dummies)
}

func TestLoginLocal_CarriesTermsAcceptedAt(t *testing.T) {
	f := newFixture(t, BasicCredentials{})
	u := f.addUser(t, "ada@example.com", true)
	accepted := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	u.TermsAcceptedAt = &accepted

	p, err := f.svc.LoginLocal(context.Background(), Credentials{Email: "ada@example.com", Password: testPassword})
	require.NoError(t, err)
	assert.Equal(t, accepted, p.TermsAcceptedAt)
}

func TestLoginLocal_WrongPassword(t *testing.T) {
	f := newFixture(t, BasicCredentials{})
	f.addUser(t, "ada@example.com", true)

	_, err := f.svc.LoginLocal(context.Background(), Credentials{Email: "ada@example.com", Password: "wrong password"})
	assert.True(t, IsInvalidCredentialsError(err))
	assert.Equal(t, 1, f.verifies)
}

func TestLoginLocal_UnknownEmailRunsDummyVerify(t *testing.T) {
	f := newFixture(t, BasicCredentials{})

	_, err := f.svc.LoginLocal(context.Background(), Credentials{Email: "nobody@example.com", Password: testPassword})
	assert.True(t, IsInvalidCredentialsError(err))
	assert.Equal(t, 1, f.dummies)
	assert.Equal(t, 0, f.verifies)
}

func TestLoginLocal_MalformedEmailRunsDummyVerify(t *testing.T) {
	f := newFixture(t, BasicCredentials{})

	_, err := f.svc.LoginLocal(context.Background(), Credentials{Email: "not an email", Password: testPassword})
	assert.True(t, IsInvalidCredentialsError(err))
	assert.Equal(t, 1, f.dummies)
	assert.Equal(t, 0, f.users.calls)
}

func TestLoginLocal_NoPasswordHashRunsDummyVerify(t *testing.T) {
	f := newFixture(t, BasicCredentials{})
	f.addUser(t, "ada@example.com", false)

	_, err := f.svc.LoginLocal(context.Background(), Credentials{Email: "ada@example.com", Password: testPassword})
	assert.True(t, IsInvalidCredentialsError(err))
	assert.Equal(t, 1, f.dummies)
	assert.Equal(t, 0, f.verifies)
}

func TestLoginLocal_SameErrorForAllFailures(t *testing.T) {
	f := newFixture(t, BasicCredentials{})
	f.addUser(t, "ada@example.com", true)
	f.addUser(t, "nohash@example.com", false)

	var errs []error
	for _, c := range []Credentials{
		{Email: "ada@example.com", Password: "wrong password"},
		{Email: "nobody@example.com", Password: testPassword},
		{Email: "nohash@example.com", Password: testPassword},
		{Email: "", Password: testPassword},
	} {
		_, err := f.svc.LoginLocal(context.Background(), c)
		require.Error(t, err)
		errs = append(errs, err)
	}
	for _, err := range errs[1:] {
		assert.Equal(t, errs[0].Error(), err.Error())
		want, _ := apperror.AsAppError(errs[0])
		got, ok := apperror.AsAppError(err)
		require.True(t, ok)
		assert.Equal(t, want.Message(), got.Message())
		assert.Equal(t, apperror.CodeAuthInvalidCredentials, got.Code())
	}
}

func TestLoginLocal_EmptyInputNoLookup(t *testing.T) {
	f := newFixture(t, BasicCredentials{})
	for _, c := range []Credentials{{Email: "", Password: testPassword}, {Email: "ada@example.com", Password: ""}, {Email: "  ", Password: testPassword}} {
		_, err := f.svc.LoginLocal(context.Background(), c)
		assert.True(t, IsInvalidCredentialsError(err))
	}
	assert.Equal(t, 0, f.users.calls)
}

func TestBasic_ConstantTimeAndOffWhenUnset(t *testing.T) {
	off := newFixture(t, BasicCredentials{})
	_, err := off.svc.Basic(context.Background(), "", "")
	assert.True(t, IsInvalidCredentialsError(err), "basic mode off refuses even empty credentials")

	f := newFixture(t, BasicCredentials{User: "admin", Password: "s3cret-pass"})
	sys := f.addOrg(t, true)
	prj := f.addProject(sys)

	for _, c := range [][2]string{{"admin", "wrong"}, {"root", "s3cret-pass"}, {"", ""}, {"admin", ""}} {
		_, err := f.svc.Basic(context.Background(), c[0], c[1])
		assert.True(t, IsInvalidCredentialsError(err), c)
	}

	p, err := f.svc.Basic(context.Background(), "admin", "s3cret-pass")
	require.NoError(t, err)
	assert.Equal(t, session.SourceBasic, p.Source)
	assert.Equal(t, uuid.Nil, p.UserID)
	assert.True(t, p.Operator)
	assert.Equal(t, []string{"*"}, p.Scopes)
	assert.Equal(t, sys.ID, p.ActiveOrgID)
	assert.Equal(t, prj.ID, p.ActiveProjectID)
}

func TestLogout_DeletesSession(t *testing.T) {
	f := newFixture(t, BasicCredentials{})
	ctx := context.Background()
	require.NoError(t, f.sessions.Save(ctx, "sid-1", session.Principal{Source: session.SourceLocal, UserID: uuid.New()}, time.Now().Add(time.Hour)))

	require.NoError(t, f.svc.Logout(ctx, "sid-1"))
	_, ok, err := f.sessions.Load(ctx, "sid-1")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestRefresh_PicksActiveOrgOrFirst(t *testing.T) {
	f := newFixture(t, BasicCredentials{})
	u := f.addUser(t, "ada@example.com", true)
	first, second := f.addOrg(t, false), f.addOrg(t, false)
	f.join(t, first, u, member.RoleMember)
	f.join(t, second, u, member.RoleAdmin)
	firstPrj := f.addProject(first)
	secondPrj, secondOther := f.addProject(second), f.addProject(second)
	ctx := context.Background()

	p, err := f.svc.Refresh(ctx, session.Principal{Source: session.SourceLocal, UserID: u.ID})
	require.NoError(t, err)
	assert.Equal(t, first.ID, p.ActiveOrgID, "no active org picks the oldest membership")
	assert.Equal(t, firstPrj.ID, p.ActiveProjectID)

	p, err = f.svc.Refresh(ctx, session.Principal{Source: session.SourceLocal, UserID: u.ID, ActiveOrgID: second.ID, ActiveProjectID: firstPrj.ID})
	require.NoError(t, err)
	assert.Equal(t, second.ID, p.ActiveOrgID)
	assert.Equal(t, member.BuiltinPermissions(member.RoleAdmin), p.Scopes)
	assert.Equal(t, secondPrj.ID, p.ActiveProjectID, "a project from another org is replaced")

	p, err = f.svc.Refresh(ctx, session.Principal{Source: session.SourceLocal, UserID: u.ID, ActiveOrgID: second.ID, ActiveProjectID: secondOther.ID})
	require.NoError(t, err)
	assert.Equal(t, secondOther.ID, p.ActiveProjectID, "the active project is kept inside the same org")

	p, err = f.svc.Refresh(ctx, session.Principal{Source: session.SourceLocal, UserID: u.ID, ActiveOrgID: uuid.New()})
	require.NoError(t, err)
	assert.Equal(t, first.ID, p.ActiveOrgID, "an org the user left falls back to the first membership")
}

func TestRefresh_NoMemberships(t *testing.T) {
	f := newFixture(t, BasicCredentials{})
	u := f.addUser(t, "ada@example.com", true)

	p, err := f.svc.Refresh(context.Background(), session.Principal{
		Source: session.SourceLocal, UserID: u.ID, ActiveOrgID: uuid.New(), ActiveProjectID: uuid.New(),
		Scopes: []string{"*"}, Operator: true,
	})
	require.NoError(t, err)
	assert.Equal(t, u.ID, p.UserID)
	assert.Equal(t, uuid.Nil, p.ActiveOrgID)
	assert.Equal(t, uuid.Nil, p.ActiveProjectID)
	assert.Empty(t, p.Scopes)
	assert.False(t, p.Operator)
}

func TestRefresh_OperatorOnlyFromSystemOrg(t *testing.T) {
	f := newFixture(t, BasicCredentials{})
	sys := f.addOrg(t, true)
	other := f.addOrg(t, false)
	ctx := context.Background()

	otherAdmin := f.addUser(t, "other@example.com", true)
	f.join(t, other, otherAdmin, member.RoleAdmin)
	p, err := f.svc.Refresh(ctx, session.Principal{Source: session.SourceLocal, UserID: otherAdmin.ID})
	require.NoError(t, err)
	assert.False(t, p.Operator, "an admin of a non-system org is not an operator")

	sysMember := f.addUser(t, "member@example.com", true)
	f.join(t, sys, sysMember, member.RoleMember)
	p, err = f.svc.Refresh(ctx, session.Principal{Source: session.SourceLocal, UserID: sysMember.ID})
	require.NoError(t, err)
	assert.False(t, p.Operator, "a plain member of the system org is not an operator")

	sysAdmin := f.addUser(t, "admin@example.com", true)
	f.join(t, other, sysAdmin, member.RoleMember)
	f.join(t, sys, sysAdmin, member.RoleAdmin)
	p, err = f.svc.Refresh(ctx, session.Principal{Source: session.SourceLocal, UserID: sysAdmin.ID})
	require.NoError(t, err)
	assert.True(t, p.Operator, "an admin of the system org is an operator")
	assert.Equal(t, other.ID, p.ActiveOrgID, "operator status does not move the active org")
}

func TestRefresh_Basic(t *testing.T) {
	f := newFixture(t, BasicCredentials{User: "admin", Password: "s3cret-pass"})
	ctx := context.Background()

	_, err := f.svc.Refresh(ctx, session.Principal{Source: session.SourceBasic})
	require.Error(t, err, "basic mode without a system org is a boot bug")

	sys := f.addOrg(t, true)
	prj := f.addProject(sys)
	p, err := f.svc.Refresh(ctx, session.Principal{Source: session.SourceBasic})
	require.NoError(t, err)
	assert.True(t, p.Operator)
	assert.Equal(t, []string{"*"}, p.Scopes)
	assert.Equal(t, sys.ID, p.ActiveOrgID)
	assert.Equal(t, prj.ID, p.ActiveProjectID)
}

func TestRefresh_KeyUntouchedAndUnknownStripped(t *testing.T) {
	f := newFixture(t, BasicCredentials{})
	ctx := context.Background()

	key := session.Principal{Source: session.SourceAPIKey, KeyID: uuid.New(), Scopes: []string{"agent:read"}, ActiveOrgID: uuid.New()}
	p, err := f.svc.Refresh(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, key, p)

	p, err = f.svc.Refresh(ctx, session.Principal{Source: session.SourceLocal, Scopes: []string{"*"}, Operator: true})
	require.NoError(t, err)
	assert.Empty(t, p.Scopes, "a person without a user id gets no authority")
	assert.False(t, p.Operator)
}

func TestSetPassword_ReplacesHashAndEndsEverySession(t *testing.T) {
	f := newFixture(t, BasicCredentials{})
	u := f.addUser(t, "ada@example.com", true)
	other := f.addUser(t, "bob@example.com", true)
	old := u.PasswordHash
	ctx := context.Background()
	exp := time.Now().Add(time.Hour)
	require.NoError(t, f.sessions.Save(ctx, "s1", session.Principal{Source: session.SourceLocal, UserID: u.ID}, exp))
	require.NoError(t, f.sessions.Save(ctx, "s2", session.Principal{Source: session.SourceLocal, UserID: u.ID}, exp))
	require.NoError(t, f.sessions.Save(ctx, "s3", session.Principal{Source: session.SourceLocal, UserID: other.ID}, exp))

	require.NoError(t, f.svc.SetPassword(ctx, " Ada@Example.com ", "a brand new password"))

	assert.NotEqual(t, old, u.PasswordHash)
	assert.Equal(t, []uuid.UUID{u.ID}, f.passwords.calls)
	for _, sid := range []string{"s1", "s2"} {
		_, ok, err := f.sessions.Load(ctx, sid)
		require.NoError(t, err)
		assert.False(t, ok, sid)
	}
	_, ok, err := f.sessions.Load(ctx, "s3")
	require.NoError(t, err)
	assert.True(t, ok, "another user's session stays")
}

func TestSetPassword_PolicyErrorChangesNothing(t *testing.T) {
	f := newFixture(t, BasicCredentials{})
	u := f.addUser(t, "ada@example.com", true)
	old := u.PasswordHash
	ctx := context.Background()
	require.NoError(t, f.sessions.Save(ctx, "s1", session.Principal{Source: session.SourceLocal, UserID: u.ID}, time.Now().Add(time.Hour)))

	err := f.svc.SetPassword(ctx, "ada@example.com", "short")
	require.Error(t, err)
	assert.Equal(t, old, u.PasswordHash)
	_, ok, err := f.sessions.Load(ctx, "s1")
	require.NoError(t, err)
	assert.True(t, ok, "sessions stay when the password is refused")
}

func TestSetPassword_UnknownEmailIsNotFound(t *testing.T) {
	f := newFixture(t, BasicCredentials{})
	err := f.svc.SetPassword(context.Background(), "nobody@example.com", "a brand new password")
	assert.True(t, user.IsUnknownEmailError(err), "%v", err)
	assert.True(t, user.IsNotFoundError(err), "%v", err)
	assert.False(t, IsInvalidCredentialsError(err))
	assert.Empty(t, f.passwords.calls)
}

type fakeProfiles struct{ users *fakeUsers }

func (f fakeProfiles) ByID(ctx context.Context, id uuid.UUID) (*user.User, error) {
	return resetUsers(f).ByID(ctx, id)
}

func (f fakeProfiles) SetName(ctx context.Context, id uuid.UUID, name string) (*user.User, error) {
	u, err := f.ByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if len(name) > 200 {
		return nil, &user.InvalidNameError{}
	}
	u.Name = strings.TrimSpace(name)
	return u, nil
}
