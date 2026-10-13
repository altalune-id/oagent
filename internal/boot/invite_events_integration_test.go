//go:build integration

package boot

import (
	"regexp"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/project"
)

var uuidShape = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

// Task 7: an invite and its accept reach the org's webhook subscribers in every project, through the outbox.
func TestInviteEvents_ReachEveryProjectsSubscribers(t *testing.T) {
	f := newAPIFixture(t)
	ctx := t.Context()
	o, err := org.NewOrg("evt-org", "Events Org", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, f.svcs.OrgStore.Save(ctx, o))
	main, err := f.svcs.Projects.Create(ctx, project.CreateRequest{OrgID: o.ID, Slug: "main", Name: "Main"})
	require.NoError(t, err)
	side, err := f.svcs.Projects.Create(ctx, project.CreateRequest{OrgID: o.ID, Slug: "side", Name: "Side"})
	require.NoError(t, err)
	for _, p := range []uuid.UUID{main.ID, side.ID} {
		_, _, err := f.svcs.Webhooks.Create(tenant.Into(ctx, tenant.Context{OrgID: o.ID, ProjectID: p}),
			"https://example.com/hook", "", []events.Type{events.InviteCreated, events.MemberAdded})
		require.NoError(t, err)
	}

	c, err := f.svcs.Invites.Create(ctx, member.Requester{Operator: true}, o.ID, "joiner@example.com", member.RoleMember)
	require.NoError(t, err)
	_, err = f.svcs.Invites.Join(ctx, c.Invite.ID, "Joiner", "a sturdy passphrase", true)
	require.NoError(t, err)

	rows, err := f.db.QueryContext(ctx, `SELECT project_id, convert_from(payload, 'UTF8') FROM outbox_entries WHERE org_id = $1`, o.ID)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	perProject := map[uuid.UUID]int{}
	var payloads []string
	for rows.Next() {
		var p uuid.UUID
		var body string
		require.NoError(t, rows.Scan(&p, &body))
		perProject[p]++
		payloads = append(payloads, body)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, map[uuid.UUID]int{main.ID: 2, side.ID: 2}, perProject, "invite.created and member.added, once per project")
	raw := c.Link[len(c.Link)-32:]
	for _, body := range payloads {
		assert.NotContains(t, body, raw, "never the token")
		assert.NotContains(t, body, "/invite/", "never the link")
		assert.False(t, uuidShape.MatchString(body), "no UUID in %s", body)
	}
	assert.Contains(t, payloads[0]+payloads[1]+payloads[2]+payloads[3], `"type":"member.added"`)
}
