//go:build integration

package boot

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/config"
	orgv1 "altalune.id/oagent/gen/go/org/v1"
	"altalune.id/oagent/internal/kernel"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/testutil/pgtest"
)

const cloudIssuer = "https://idp.example.com"

func cloudServices(t *testing.T) *Services {
	t.Helper()
	svcs, _ := cloudServicesWithPool(t)
	return svcs
}

func cloudServicesWithPool(t *testing.T) (*Services, db.Pool) {
	t.Helper()
	cfg := devLocalConfig()
	cfg.Mode = config.ModeCloud
	cfg.HTTP.BaseURL = "https://oagent.example.com"
	cfg.OIDC = config.OIDCConfig{Issuer: cloudIssuer, ClientID: "oagent", Scopes: []string{"openid", "email"}, ButtonLabel: "Continue with SSO"}
	pool, _, schemaName := pgtest.MigratedRLS(t)
	cfg.Postgres.Schema = schemaName
	return buildServices(kernel.Kernel{Pool: pool, Reporter: apperror.NewReporter(nil, false)}, cfg), pool
}

// NOTE: each creation waits here, after its "owns one?" check, for the other or for a second; only the row lock keeps both from passing.
type racedOwners struct {
	*member.Service
	left atomic.Int32
	all  chan struct{}
}

func (r *racedOwners) ListByUser(ctx context.Context, userID uuid.UUID) ([]*member.Membership, error) {
	ms, err := r.Service.ListByUser(ctx, userID)
	if r.left.Add(-1) == 0 {
		close(r.all)
	}
	select {
	case <-r.all:
	case <-time.After(time.Second):
	}
	return ms, err
}

// SECURITY: one owned org per person holds when two creations both pass their check at once; the creator's row lock lets one through.
func TestCreateOwnOrg_OneOrgPerPerson(t *testing.T) {
	svcs, pool := cloudServicesWithPool(t)
	ctx := t.Context()
	ana, err := svcs.Users.CreateLinked(ctx, "ana@shop.test", "Ana", cloudIssuer, "sub-ana")
	require.NoError(t, err)

	raced := &racedOwners{Service: svcs.Members, all: make(chan struct{})}
	raced.left.Store(2)
	orgs := org.NewService(svcs.OrgStore, db.NewTransactor(pool), apperror.NewReporter(nil, false).Unexpected).
		WithCreation(true).
		WithAfterCreate(func(ctx context.Context, o *org.Org) error {
			if err := ownNewOrg(ctx, svcs.Users, raced, o); err != nil {
				return err
			}
			_, err := svcs.Projects.Create(ctx, project.CreateRequest{OrgID: o.ID, Slug: project.DefaultSlug, Name: project.DefaultName})
			return err
		})
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Go(func() { _, errs[i] = orgs.Create(ctx, org.CreateRequest{Name: "Ana Org", CreatedBy: ana.ID}) })
	}
	wg.Wait()
	wins := 0
	for _, err := range errs {
		if err == nil {
			wins++
			continue
		}
		assert.True(t, org.IsAlreadyOwnedError(err), "%v", err)
	}
	assert.Equal(t, 1, wins)

	ms, err := svcs.Members.ListByUser(ctx, ana.ID)
	require.NoError(t, err)
	require.Len(t, ms, 1)
	assert.Equal(t, member.RoleOwner, ms[0].Role, "the creator owns it, in the org's own unit of work")
	projects, err := svcs.Projects.List(tenant.Into(ctx, tenant.Context{OrgID: ms[0].OrgID, UserID: ana.ID}), ms[0].OrgID)
	require.NoError(t, err)
	assert.Len(t, projects, 1, "with its default project")

	_, err = svcs.Auth.CreateOwnOrg(ctx, ana.ID, "Second", "")
	assert.True(t, org.IsAlreadyOwnedError(err))
}

// SECURITY: a person's CreateOrg makes them the owner, never an orphan org, and a second one is ORG006; an operator owns nothing.
func TestCreateOrg_ThePersonOwnsIt(t *testing.T) {
	svcs := cloudServices(t)
	ctx := t.Context()
	ana, err := svcs.Users.CreateLinked(ctx, "ana@shop.test", "Ana", cloudIssuer, "sub-ana")
	require.NoError(t, err)
	as := session.PrincipalInto(ctx, session.Principal{Source: session.SourceOIDC, UserID: ana.ID, Email: ana.Email})
	plane := org.NewControlPlane(svcs.Orgs)

	resp, err := plane.CreateOrg(as, connect.NewRequest(&orgv1.CreateOrgRequest{Name: "Ana Shop"}))
	require.NoError(t, err)
	o, err := svcs.Orgs.ByPublicID(ctx, resp.Msg.GetOrg().GetId())
	require.NoError(t, err)
	m, err := svcs.Members.Find(ctx, o.ID, ana.ID)
	require.NoError(t, err)
	assert.Equal(t, member.RoleOwner, m.Role)

	_, err = plane.CreateOrg(as, connect.NewRequest(&orgv1.CreateOrgRequest{Name: "Second"}))
	assert.True(t, org.IsAlreadyOwnedError(err), "%v", err)

	staff, err := svcs.Users.CreateLinked(ctx, "staff@shop.test", "Staff", cloudIssuer, "sub-staff")
	require.NoError(t, err)
	op := session.PrincipalInto(ctx, session.Principal{Source: session.SourceOIDC, UserID: staff.ID, Operator: true})
	resp, err = plane.CreateOrg(op, connect.NewRequest(&orgv1.CreateOrgRequest{Name: "Staff Made"}))
	require.NoError(t, err)
	made, err := svcs.Orgs.ByPublicID(ctx, resp.Msg.GetOrg().GetId())
	require.NoError(t, err)
	_, err = svcs.Members.Find(ctx, made.ID, staff.ID)
	assert.True(t, member.IsNotFoundError(err), "an operator's org has no owner from the call")
}
