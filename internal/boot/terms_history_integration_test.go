//go:build integration

package boot

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/legal"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/org/member"
)

// Task 8: every acceptance of the built-in terms is kept with the exact versions accepted.
func TestTermsHistory_RecordsEveryAcceptance(t *testing.T) {
	f := newAPIFixture(t)
	ctx := t.Context()
	o, err := org.NewOrg("terms-org", "Terms Org", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, f.svcs.OrgStore.Save(ctx, o))
	c, err := f.svcs.Invites.Create(ctx, member.Requester{Operator: true}, o.ID, "joiner@example.com", member.RoleMember)
	require.NoError(t, err)
	id, err := f.svcs.Invites.Join(ctx, c.Invite.ID, "Joiner", "a sturdy passphrase", true)
	require.NoError(t, err)
	_, err = f.svcs.Legal.Accept(ctx, id, legal.SourceWelcome)
	require.NoError(t, err)

	rows, err := f.db.QueryContext(ctx, `SELECT a.source, t.slug, t.version, p.slug
		FROM terms_acceptances a
		JOIN legal_documents t ON t.id = a.terms_document_id
		JOIN legal_documents p ON p.id = a.privacy_document_id
		WHERE a.user_id = $1 ORDER BY a.accepted_at`, id)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var got [][4]string
	for rows.Next() {
		var r [4]string
		require.NoError(t, rows.Scan(&r[0], &r[1], &r[2], &r[3]))
		got = append(got, r)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, [][4]string{
		{"join", "terms", "2026-10-11", "privacy"},
		{"welcome", "terms", "2026-10-11", "privacy"},
	}, got)
}
