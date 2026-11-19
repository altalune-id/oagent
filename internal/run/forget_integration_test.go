//go:build integration

package run_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/run/snapshotstore"
)

func TestStore_ForgetEndUserLeavesNothingOfTheirs(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	ctx := in(s.org)
	alice, bob := f.endUser(t, s), f.endUser(t, s)
	owned, err := f.store.UpsertAPIThread(ctx, s.project, s.agent, "alice-dm", alice)
	require.NoError(t, err)
	_, err = f.store.AppendMessage(ctx, owned.ID, userMsg("my address is 12 Elm St", ""))
	require.NoError(t, err)
	group, quiet := f.thread(t, s), f.thread(t, s)
	said := func(thread uuid.UUID, who uuid.UUID, text string) {
		m := userMsg(text, "")
		m.SpeakerID = &who
		_, err := f.store.AppendMessage(ctx, thread, m)
		require.NoError(t, err)
	}
	said(group, alice, "alice secret")
	said(group, bob, "bob hello")
	said(quiet, bob, "bob only")
	for _, th := range []uuid.UUID{group, quiet} {
		f.exec(t, `INSERT INTO `+f.schema+`.thread_snapshots (thread_id, org_id, project_id, agent_version_id, sdk_version, blob, updated_at) VALUES ($1, $2, $3, $4, '1', 'state', now())`,
			th, s.org, s.project, s.version)
	}
	other := f.seed(t)
	stranger := f.endUser(t, other)

	svc := run.NewService(f.store, db.NewTransactor(f.pool), apperror.NewReporter(nil, false).Unexpected).
		WithSnapshots(snapshotstore.NewPostgres(f.pool, f.schema, ""))
	f.exec(t, `UPDATE `+f.schema+`.threads SET lease_owner = 'tok', lease_until = now() + interval '1 minute' WHERE id = $1`, group)
	assert.True(t, run.IsInProgressError(svc.ForgetEndUser(ctx, s.org, s.project, alice)), "a live turn in a touched thread holds the delete back")
	assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM `+f.schema+`.threads WHERE id = $1`, owned.ID), "and nothing changed")
	f.exec(t, `UPDATE `+f.schema+`.threads SET lease_owner = NULL, lease_until = NULL WHERE id = $1`, group)

	require.NoError(t, svc.ForgetEndUser(ctx, s.org, s.project, alice))
	touched, err := f.store.ForgetEndUser(in(other.org), alice)
	require.NoError(t, err)
	assert.Empty(t, touched, "another tenant's call touches nothing")

	q := `SELECT count(*) FROM ` + f.schema + `.`
	assert.Zero(t, f.count(t, q+`threads WHERE id = $1`, owned.ID), "the thread they own is gone")
	assert.Zero(t, f.count(t, q+`messages WHERE body::text LIKE '%secret%' OR body::text LIKE '%Elm St%'`), "none of their words remain")
	assert.Equal(t, 1, f.count(t, q+`messages WHERE thread_id = $1 AND speaker_id = $2 AND deleted_at IS NOT NULL AND body->>'content' = '[message deleted]'`, group, alice),
		"their group message keeps its place, blanked")
	assert.Equal(t, 1, f.count(t, q+`messages WHERE thread_id = $1 AND body->>'content' = 'bob hello' AND deleted_at IS NULL`, group), "others' messages stay")
	assert.Zero(t, f.count(t, q+`thread_snapshots WHERE thread_id = $1`, group), "the group's saved state goes")
	assert.Equal(t, 1, f.count(t, q+`thread_snapshots WHERE thread_id = $1`, quiet), "a thread they never spoke in keeps its state")
	assert.Equal(t, 2, f.count(t, q+`end_users WHERE id IN ($1, $2)`, alice, stranger), "the end user row itself is the caller's to delete")

	msgs, err := f.store.ThreadMessages(ctx, group, 0, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	assert.True(t, msgs[0].Deleted)
	assert.False(t, msgs[1].Deleted)
}
