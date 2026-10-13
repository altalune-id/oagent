package boot

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/apikey"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/testutil/fakes"
	"altalune.id/oagent/internal/user"
	"altalune.id/oagent/sealer"
)

func TestBuildSealer(t *testing.T) {
	key := hex.EncodeToString(bytes.Repeat([]byte{1}, sealer.KeyLen))

	s := buildSealer(config.SecretsConfig{EncryptionKey: key})
	_, err := s.Seal([]byte("x"), nil)
	assert.NoError(t, err)

	d := buildSealer(config.SecretsConfig{EncryptionKey: ""})
	_, err = d.Seal(nil, nil)
	assert.True(t, sealer.IsUnavailableError(err))

	bad := buildSealer(config.SecretsConfig{EncryptionKey: "zz"})
	_, err = bad.Seal(nil, nil)
	assert.True(t, sealer.IsUnavailableError(err))
}

func TestRefResolver(t *testing.T) {
	p, err := project.NewProject(uuid.New(), "acme", "Acme", uuid.Nil)
	require.NoError(t, err)
	o, err := org.NewOrg("acme", "Acme", uuid.Nil)
	require.NoError(t, err)

	r := refResolver{
		projects: project.NewService(&fakes.ProjectStore{Project: p}, testUnexpected()),
		orgs:     org.NewService(&fakes.OrgStore{Org: o}, fakes.Transactor{}, testUnexpected()),
	}

	slug, publicID, err := r.ProjectRef(context.Background(), p.ID)
	require.NoError(t, err)
	assert.Equal(t, "acme", slug)
	assert.Equal(t, p.PublicID, publicID)

	orgPublicID, err := r.OrgRef(context.Background(), o.ID)
	require.NoError(t, err)
	assert.Equal(t, o.PublicID, orgPublicID)
}

func TestOwnerRoles(t *testing.T) {
	store := &fakes.MemberStore{}
	members := member.NewService(store, fakes.Transactor{}, testUnexpected())
	orgID, userID := uuid.New(), uuid.New()
	_, err := members.Add(context.Background(), orgID, userID, member.RoleAdmin)
	require.NoError(t, err)
	r := ownerRoles{members: members}

	role, ok, err := r.Role(context.Background(), orgID, userID)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, member.RoleAdmin, role)

	_, ok, err = r.Role(context.Background(), orgID, uuid.New())
	require.NoError(t, err)
	assert.False(t, ok)

	store.Err = errors.New("db down")
	_, _, err = r.Role(context.Background(), orgID, userID)
	assert.Error(t, err)
}

func TestProjectOrgs(t *testing.T) {
	p, err := project.NewProject(uuid.New(), "acme", "Acme", uuid.Nil)
	require.NoError(t, err)
	r := projectOrgs{projects: project.NewService(&fakes.ProjectStore{Project: p}, testUnexpected())}

	got, err := r.OrgOf(context.Background(), p.ID)
	require.NoError(t, err)
	assert.Equal(t, p.OrgID, got)

	_, err = projectOrgs{projects: project.NewService(&fakes.ProjectStore{Err: &project.NotFoundError{}}, testUnexpected())}.OrgOf(context.Background(), uuid.New())
	assert.True(t, project.IsNotFoundError(err))
}

func TestRefResolver_PropagatesNotFound(t *testing.T) {
	r := refResolver{
		projects: project.NewService(&fakes.ProjectStore{Err: &project.NotFoundError{}}, testUnexpected()),
		orgs:     org.NewService(&fakes.OrgStore{Err: &org.NotFoundError{}}, fakes.Transactor{}, testUnexpected()),
	}

	_, _, err := r.ProjectRef(context.Background(), uuid.New())
	assert.True(t, project.IsNotFoundError(err))

	_, err = r.OrgRef(context.Background(), uuid.New())
	assert.True(t, org.IsNotFoundError(err))
}

func TestProjectIDs(t *testing.T) {
	orgID := uuid.New()
	a, err := project.NewProject(orgID, "alpha", "Alpha", uuid.Nil)
	require.NoError(t, err)
	b, err := project.NewProject(orgID, "beta", "Beta", uuid.Nil)
	require.NoError(t, err)

	ids, err := projectIDs{projects: project.NewService(&fakes.ProjectStore{Projects: []*project.Project{a, b}}, testUnexpected())}.ProjectIDs(context.Background(), orgID)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{a.ID, b.ID}, ids)

	ids, err = projectIDs{projects: project.NewService(&fakes.ProjectStore{}, testUnexpected())}.ProjectIDs(context.Background(), orgID)
	require.NoError(t, err)
	assert.Empty(t, ids)

	_, err = projectIDs{projects: project.NewService(&fakes.ProjectStore{Err: errors.New("db down")}, testUnexpected())}.ProjectIDs(context.Background(), orgID)
	assert.Error(t, err)
}

func TestMemberPeople(t *testing.T) {
	store := fakes.NewUserStore()
	users := user.NewService(store, testUnexpected())
	u, err := users.Create(context.Background(), user.CreateRequest{Email: "ana@example.com", Name: "Ana", Password: "quiet-forest-lamp-9"})
	require.NoError(t, err)

	email, name, err := memberPeople{users: users}.Contact(context.Background(), u.ID)
	require.NoError(t, err)
	assert.Equal(t, "ana@example.com", email)
	assert.Equal(t, "Ana", name)

	_, _, err = memberPeople{users: users}.Contact(context.Background(), uuid.New())
	assert.Error(t, err)

	publicID, err := memberPeople{users: users}.PublicID(context.Background(), u.ID)
	require.NoError(t, err)
	assert.Equal(t, u.PublicID, publicID)
	_, err = memberPeople{users: users}.PublicID(context.Background(), uuid.New())
	assert.Error(t, err)
}

func TestMemberOrgs(t *testing.T) {
	o, err := org.NewOrg("acme", "Acme Corp", uuid.Nil)
	require.NoError(t, err)
	store := &fakes.OrgStore{Org: o}
	orgs := memberOrgs{orgs: org.NewService(store, fakes.Transactor{}, testUnexpected())}

	name, err := orgs.Name(context.Background(), o.ID)
	require.NoError(t, err)
	assert.Equal(t, "Acme Corp", name)
	slug, err := orgs.Slug(context.Background(), o.ID)
	require.NoError(t, err)
	assert.Equal(t, "acme", slug)

	store.Err = errors.New("db down")
	_, err = orgs.Name(context.Background(), o.ID)
	assert.Error(t, err)
	_, err = orgs.Slug(context.Background(), o.ID)
	assert.Error(t, err)
}

func TestMemberEffects(t *testing.T) {
	svcs := &Services{
		Users: user.NewService(fakes.NewUserStore(), testUnexpected()), Orgs: org.NewService(&fakes.OrgStore{}, fakes.Transactor{}, testUnexpected()),
		Sessions: session.NewMemoryStore(), APIKeys: apikey.NewService(&fakes.APIKeyStore{}, apikey.NewScheme(""), nil, testUnexpected()),
		MailQueue: buildMailQueue(db.Pool{}, config.Config{}, buildSealer(config.SecretsConfig{})),
	}
	cfg := config.Config{
		HTTP: config.HTTPConfig{BaseURL: "https://oagent.example.com/", BasePath: "/app"},
		Mail: config.MailConfig{From: "team@example.com"},
	}

	e := memberEffects(cfg, svcs)
	assert.Same(t, svcs.APIKeys, e.Keys)
	assert.Equal(t, svcs.Sessions, e.Sessions)
	assert.Equal(t, svcs.MailQueue, e.Mail)
	assert.Equal(t, memberPeople{users: svcs.Users}, e.People)
	assert.Equal(t, memberOrgs{orgs: svcs.Orgs}, e.Orgs)
	assert.Equal(t, "team@example.com", e.From)
	assert.Equal(t, "https://oagent.example.com/app", e.BaseURL)
}
