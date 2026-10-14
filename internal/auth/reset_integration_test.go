//go:build integration

package auth_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/auth"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/mailqueue"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/testutil/pgtest"
	"altalune.id/oagent/internal/user"
	"altalune.id/oagent/mailer"
	"altalune.id/oagent/password"
	"altalune.id/oagent/sealer"
)

type resetDB struct {
	pool     db.Pool
	users    *user.Service
	sessions *session.PostgresStore
	store    auth.ResetStore
	queue    mailqueue.Store
	resets   *auth.Resets
	mail     *failingMail
	person   *user.User
	ahead    time.Duration
}

type failingMail struct {
	next    mailqueue.Store
	failOn  string
	queued  []mailer.Message
	queueMu sync.Mutex
}

func (f *failingMail) Enqueue(ctx context.Context, m mailer.Message) (uuid.UUID, error) {
	if f.failOn != "" && strings.Contains(m.Subject, f.failOn) {
		return uuid.Nil, errors.New("mail queue down")
	}
	f.queueMu.Lock()
	f.queued = append(f.queued, m)
	f.queueMu.Unlock()
	return f.next.Enqueue(ctx, m)
}

func newResetDB(t *testing.T) *resetDB {
	t.Helper()
	sqlDB, schemaName := pgtest.Migrated(t)
	pool := db.Pool{W: sqlDB, R: sqlDB}
	unexpected := apperror.NewReporter(nil, false).Unexpected
	sl, err := sealer.New(bytes.Repeat([]byte{7}, 32))
	require.NoError(t, err)
	d := &resetDB{
		pool:     pool,
		users:    user.NewService(user.NewStore(pool, schemaName, ""), unexpected),
		sessions: session.NewPostgresStore(sqlDB, schemaName, "", sl, unexpected),
		store:    auth.NewResetStore(pool, schemaName, ""),
		queue:    mailqueue.NewStore(pool, schemaName, "", sl, false),
	}
	d.mail = &failingMail{next: d.queue}
	d.resets = auth.NewResets(auth.ResetDeps{
		Store: d.store, Transactor: db.NewTransactor(pool), Users: d.users, Passwords: d.users,
		Sessions: d.sessions, Mail: d.mail, From: "noreply@example.com", BaseURL: "https://oagent.example.com",
	}).WithClock(func() time.Time { return time.Now().Add(d.ahead) })
	d.person, err = d.users.Create(t.Context(), user.CreateRequest{Email: "ada@example.com", Name: "Ada", Password: "the old passphrase"})
	require.NoError(t, err)
	return d
}

func (d *resetDB) link(t *testing.T) *auth.Reset {
	t.Helper()
	d.ahead += 2 * time.Minute
	require.NoError(t, d.resets.Request(t.Context(), "ada@example.com"))
	m := d.mail.queued[len(d.mail.queued)-1]
	i := strings.Index(m.TextBody, "/reset-password/")
	require.GreaterOrEqual(t, i, 0)
	r, err := d.resets.Lookup(t.Context(), strings.Fields(m.TextBody[i+len("/reset-password/"):])[0])
	require.NoError(t, err)
	return r
}

func (d *resetDB) password(t *testing.T) string {
	t.Helper()
	u, err := d.users.ByID(t.Context(), d.person.ID)
	require.NoError(t, err)
	return u.PasswordHash
}

func TestResetStore_AgainstPostgres(t *testing.T) {
	d := newResetDB(t)
	ctx := t.Context()
	first := d.link(t)
	recent, err := d.store.HasRecentUnused(ctx, d.person.ID, first.CreatedAt.Add(-time.Second))
	require.NoError(t, err)
	assert.True(t, recent)
	recent, err = d.store.HasRecentUnused(ctx, d.person.ID, first.CreatedAt)
	require.NoError(t, err)
	assert.False(t, recent, "only links created after since count")
	second := d.link(t)
	_, err = d.store.ByPublicID(ctx, first.PublicID)
	assert.Equal(t, auth.ResetNotFound, auth.ResetReasonOf(err), "a new request deletes the unused link")
	got, err := d.store.ByTokenHash(ctx, second.TokenHash)
	require.NoError(t, err)
	assert.Equal(t, second.ID, got.ID)
	assert.Equal(t, d.person.ID, got.UserID)

	now := time.Now()
	_, err = d.store.MarkUsed(ctx, second.PublicID, second.ExpiresAt)
	assert.Equal(t, auth.ResetNotFound, auth.ResetReasonOf(err), "an expired link is not marked")
	used, err := d.store.MarkUsed(ctx, second.PublicID, now)
	require.NoError(t, err)
	require.NotNil(t, used.UsedAt)
	_, err = d.store.MarkUsed(ctx, second.PublicID, now)
	assert.Equal(t, auth.ResetNotFound, auth.ResetReasonOf(err), "a used link is not marked again")

	recent, err = d.store.HasRecentUnused(ctx, d.person.ID, second.CreatedAt.Add(-time.Second))
	require.NoError(t, err)
	assert.False(t, recent, "a used link does not count")
	require.NoError(t, d.store.DeleteUnused(ctx, d.person.ID))
	_, err = d.store.ByPublicID(ctx, second.PublicID)
	assert.NoError(t, err, "a used link is kept")
}

// Review Focus 4: two submits of one link: one sets the password, the other gets AUT004 used.
func TestResets_RequestAgainWithinTwoMinutesKeepsTheFirstLink(t *testing.T) {
	d := newResetDB(t)
	first := d.link(t)
	require.NoError(t, d.resets.Request(t.Context(), "ada@example.com"))
	assert.Len(t, d.mail.queued, 1, "no second email")
	got, err := d.resets.ByPublicID(t.Context(), first.PublicID)
	require.NoError(t, err, "the first link still works")
	assert.Equal(t, first.ID, got.ID)
}

func TestResets_DoubleSubmitUsesTheLinkOnce(t *testing.T) {
	d := newResetDB(t)
	for round := range 5 {
		r := d.link(t)
		results := make([]error, 2)
		var wg sync.WaitGroup
		for i := range results {
			wg.Go(func() {
				results[i] = d.resets.Complete(t.Context(), r.PublicID, "brand new passphrase "+string(rune('a'+i)))
			})
		}
		wg.Wait()
		ok, used := 0, 0
		for _, err := range results {
			switch {
			case err == nil:
				ok++
			case auth.ResetReasonOf(err) == auth.ResetUsed:
				used++
			default:
				t.Fatalf("round %d: unexpected error %v", round, err)
			}
		}
		assert.Equal(t, 1, ok, "round %d", round)
		assert.Equal(t, 1, used, "round %d", round)
	}
}

// Review Focus 4: a failure after the link is marked used changes nothing.
func TestResets_CompleteIsOneUnitOfWork(t *testing.T) {
	d := newResetDB(t)
	ctx := t.Context()
	require.NoError(t, d.sessions.Save(ctx, "old-sid", session.Principal{UserID: d.person.ID, Source: session.SourceLocal}, time.Now().Add(time.Hour)))
	r := d.link(t)
	before := d.password(t)

	d.mail.failOn = "was changed"
	err := d.resets.Complete(ctx, r.PublicID, "a brand new passphrase")
	require.ErrorContains(t, err, "mail queue down")

	assert.Equal(t, before, d.password(t), "the password is unchanged")
	_, ok, err := d.sessions.Load(ctx, "old-sid")
	require.NoError(t, err)
	assert.True(t, ok, "the session survives")
	still, err := d.resets.ByPublicID(ctx, r.PublicID)
	require.NoError(t, err, "the link is still usable")
	assert.Nil(t, still.UsedAt)

	d.mail.failOn = ""
	require.NoError(t, d.resets.Complete(ctx, r.PublicID, "a brand new passphrase"))
	assert.NoError(t, password.Verify(d.password(t), "a brand new passphrase"))
	_, ok, err = d.sessions.Load(ctx, "old-sid")
	require.NoError(t, err)
	assert.False(t, ok, "every session ended")
}

func TestResets_WeakPasswordKeepsTheLink(t *testing.T) {
	d := newResetDB(t)
	r := d.link(t)
	err := d.resets.Complete(t.Context(), r.PublicID, "ada@example.com")
	require.Error(t, err)
	_, err = d.resets.ByPublicID(t.Context(), r.PublicID)
	assert.NoError(t, err)
}

func TestChangePassword_IsOneUnitOfWork(t *testing.T) {
	d := newResetDB(t)
	ctx := t.Context()
	svc := auth.NewService(auth.Deps{
		Users: d.users, Passwords: d.users, Sessions: d.sessions, Profiles: d.users,
		Transactor: db.NewTransactor(d.pool), Mail: d.mail, From: "noreply@example.com", BaseURL: "https://oagent.example.com",
	})
	p := session.Principal{UserID: d.person.ID, Source: session.SourceLocal}
	for _, sid := range []string{"this", "other"} {
		require.NoError(t, d.sessions.Save(ctx, sid, p, time.Now().Add(time.Hour)))
	}
	before := d.password(t)

	d.mail.failOn = "was changed"
	err := svc.ChangePassword(ctx, d.person.ID, "this", "the old passphrase", "a brand new passphrase")
	require.ErrorContains(t, err, "mail queue down")
	assert.Equal(t, before, d.password(t), "the password is unchanged")
	_, ok, err := d.sessions.Load(ctx, "other")
	require.NoError(t, err)
	assert.True(t, ok, "the other session survives")

	d.mail.failOn = ""
	require.NoError(t, svc.ChangePassword(ctx, d.person.ID, "this", "the old passphrase", "a brand new passphrase"))
	assert.NoError(t, password.Verify(d.password(t), "a brand new passphrase"))
	for sid, want := range map[string]bool{"this": true, "other": false} {
		_, ok, err := d.sessions.Load(ctx, sid)
		require.NoError(t, err)
		assert.Equal(t, want, ok, sid)
	}
}
