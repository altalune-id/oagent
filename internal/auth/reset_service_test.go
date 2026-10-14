package auth

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/testutil/fakes"
	"altalune.id/oagent/internal/user"
	"altalune.id/oagent/mailer"
)

type memResets struct {
	mu   sync.Mutex
	rows map[string]*Reset
	err  error
}

func newMemResets() *memResets { return &memResets{rows: map[string]*Reset{}} }

func (m *memResets) Insert(_ context.Context, r *Reset) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	c := *r
	m.rows[r.PublicID] = &c
	return nil
}

func (m *memResets) DeleteUnused(_ context.Context, userID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, r := range m.rows {
		if r.UserID == userID && r.UsedAt == nil {
			delete(m.rows, k)
		}
	}
	return nil
}

func (m *memResets) HasRecentUnused(_ context.Context, userID uuid.UUID, since time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.rows {
		if r.UserID == userID && r.UsedAt == nil && r.CreatedAt.After(since) {
			return true, nil
		}
	}
	return false, nil
}

func (m *memResets) ByTokenHash(_ context.Context, hash string) (*Reset, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.rows {
		if r.TokenHash == hash {
			c := *r
			return &c, nil
		}
	}
	return nil, &ResetNotUsableError{Reason: ResetNotFound}
}

func (m *memResets) ByPublicID(_ context.Context, publicID string) (*Reset, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.rows[publicID]; ok {
		c := *r
		return &c, nil
	}
	return nil, &ResetNotUsableError{Reason: ResetNotFound}
}

func (m *memResets) MarkUsed(_ context.Context, publicID string, now time.Time) (*Reset, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rows[publicID]
	if !ok || r.UsedAt != nil || !now.Before(r.ExpiresAt) {
		return nil, &ResetNotUsableError{Reason: ResetNotFound}
	}
	r.UsedAt = &now
	c := *r
	return &c, nil
}

type fakeMail struct {
	sent []mailer.Message
	err  error
}

func (f *fakeMail) Enqueue(_ context.Context, m mailer.Message) (uuid.UUID, error) {
	if f.err != nil {
		return uuid.Nil, f.err
	}
	f.sent = append(f.sent, m)
	return uuid.New(), nil
}

type resetUsers struct{ users *fakeUsers }

func (r resetUsers) ByEmail(ctx context.Context, email string) (*user.User, error) {
	return r.users.ByEmail(ctx, email)
}

func (r resetUsers) ByID(_ context.Context, id uuid.UUID) (*user.User, error) {
	for _, u := range r.users.byEmail {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, &user.NotFoundError{}
}

type resetFixture struct {
	*fixture
	store *memResets
	mail  *fakeMail
	now   time.Time
	svc   *Resets
}

func newResetFixture(t *testing.T) *resetFixture {
	t.Helper()
	f := &resetFixture{fixture: newFixture(t, BasicCredentials{}), store: newMemResets(), mail: &fakeMail{},
		now: time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)}
	f.svc = NewResets(ResetDeps{
		Store: f.store, Transactor: fakes.Transactor{}, Users: resetUsers{f.users}, Passwords: f.passwords,
		Sessions: f.sessions, Mail: f.mail, From: "oagent <noreply@example.com>", BaseURL: "https://oagent.example.com/app",
	}).WithClock(func() time.Time { return f.now })
	return f
}

func linkToken(t *testing.T, m mailer.Message) string {
	t.Helper()
	const prefix = "https://oagent.example.com/app/reset-password/"
	i := strings.Index(m.TextBody, prefix)
	require.GreaterOrEqual(t, i, 0, m.TextBody)
	return strings.Fields(m.TextBody[i+len(prefix):])[0]
}

func TestResets_RequestKnownEmailQueuesOneLink(t *testing.T) {
	f := newResetFixture(t)
	u := f.addUser(t, "ada@example.com", true)
	require.NoError(t, f.svc.Request(t.Context(), " Ada@Example.com "))

	require.Len(t, f.mail.sent, 1)
	m := f.mail.sent[0]
	assert.Equal(t, "ada@example.com", m.To)
	assert.Equal(t, "oagent <noreply@example.com>", m.From)
	assert.Equal(t, "Reset your oagent password", m.Subject)
	raw := linkToken(t, m)
	assert.Contains(t, m.HTMLBody, "/reset-password/"+raw)
	assert.Contains(t, m.TextBody, "13 October 2026, 10:00 UTC")
	require.Len(t, f.store.rows, 1)
	for _, r := range f.store.rows {
		assert.Equal(t, u.ID, r.UserID)
		assert.Equal(t, hashResetToken(raw), r.TokenHash, "only the hash is stored")
		assert.NotContains(t, m.TextBody, r.PublicID, "the email holds the token, not the public id")
		assert.NotContains(t, m.TextBody, u.ID.String())
	}
}

func TestResets_ANewRequestReplacesUnusedLinks(t *testing.T) {
	f := newResetFixture(t)
	f.addUser(t, "ada@example.com", true)
	require.NoError(t, f.svc.Request(t.Context(), "ada@example.com"))
	first := linkToken(t, f.mail.sent[0])
	f.now = f.now.Add(2 * time.Minute)
	require.NoError(t, f.svc.Request(t.Context(), "ada@example.com"))
	require.Len(t, f.mail.sent, 2)
	assert.Len(t, f.store.rows, 1)
	_, err := f.svc.Lookup(t.Context(), first)
	assert.Equal(t, ResetNotFound, ResetReasonOf(err), "the first link no longer works")
	_, err = f.svc.Lookup(t.Context(), linkToken(t, f.mail.sent[1]))
	assert.NoError(t, err)
}

func TestResets_RequestAgainWithinTwoMinutesSendsNothingNew(t *testing.T) {
	f := newResetFixture(t)
	f.addUser(t, "ada@example.com", true)
	require.NoError(t, f.svc.Request(t.Context(), "ada@example.com"))
	f.now = f.now.Add(2*time.Minute - time.Second)
	require.NoError(t, f.svc.Request(t.Context(), "ada@example.com"))
	require.Len(t, f.mail.sent, 1, "no second email")
	assert.Len(t, f.store.rows, 1, "one live link")
	_, err := f.svc.Lookup(t.Context(), linkToken(t, f.mail.sent[0]))
	assert.NoError(t, err, "the first link still works")
}

func TestResets_RequestAfterTheFirstLinkWasUsedSendsANewOne(t *testing.T) {
	f := newResetFixture(t)
	f.addUser(t, "ada@example.com", true)
	require.NoError(t, f.svc.Request(t.Context(), "ada@example.com"))
	r, err := f.svc.Lookup(t.Context(), linkToken(t, f.mail.sent[0]))
	require.NoError(t, err)
	_, err = f.store.MarkUsed(t.Context(), r.PublicID, f.now)
	require.NoError(t, err)
	require.NoError(t, f.svc.Request(t.Context(), "ada@example.com"))
	assert.Len(t, f.mail.sent, 2)
}

func TestResets_RequestUnknownOrPasswordlessSendsNothing(t *testing.T) {
	f := newResetFixture(t)
	f.addUser(t, "sso@example.com", false)
	for _, email := range []string{"nobody@example.com", "sso@example.com", "not an email", ""} {
		require.NoError(t, f.svc.Request(t.Context(), email), email)
	}
	assert.Empty(t, f.mail.sent)
	assert.Empty(t, f.store.rows)
}

func TestResets_RequestErrors(t *testing.T) {
	f := newResetFixture(t)
	f.addUser(t, "ada@example.com", true)
	f.store.err = errors.New("db down")
	assert.Error(t, f.svc.Request(t.Context(), "ada@example.com"))
	f.store.err = nil
	f.mail.err = errors.New("queue down")
	assert.Error(t, f.svc.Request(t.Context(), "ada@example.com"))
}

func TestResets_LookupAndByPublicID(t *testing.T) {
	f := newResetFixture(t)
	f.addUser(t, "ada@example.com", true)
	require.NoError(t, f.svc.Request(t.Context(), "ada@example.com"))
	raw := linkToken(t, f.mail.sent[0])

	r, err := f.svc.Lookup(t.Context(), raw)
	require.NoError(t, err)
	got, err := f.svc.ByPublicID(t.Context(), r.PublicID)
	require.NoError(t, err)
	assert.Equal(t, r.ID, got.ID)
	email, name, err := f.svc.Person(t.Context(), r)
	require.NoError(t, err)
	assert.Equal(t, "ada@example.com", email)
	assert.Equal(t, "Ada", name)

	for _, bad := range []string{"", "nope", raw + "x"} {
		_, err = f.svc.Lookup(t.Context(), bad)
		assert.Equal(t, ResetNotFound, ResetReasonOf(err), bad)
	}
	_, err = f.svc.ByPublicID(t.Context(), "nope")
	assert.Equal(t, ResetNotFound, ResetReasonOf(err))

	f.now = f.now.Add(ResetTTL)
	_, err = f.svc.Lookup(t.Context(), raw)
	assert.Equal(t, ResetExpired, ResetReasonOf(err))
	_, err = f.svc.ByPublicID(t.Context(), r.PublicID)
	assert.Equal(t, ResetExpired, ResetReasonOf(err))
}

func TestResets_CompleteSetsThePasswordEndsEverySessionAndTellsThePerson(t *testing.T) {
	f := newResetFixture(t)
	u := f.addUser(t, "ada@example.com", true)
	other := f.addUser(t, "bob@example.com", true)
	ctx := t.Context()
	for _, sid := range []string{"s1", "s2"} {
		require.NoError(t, f.sessions.Save(ctx, sid, sessionOf(u), time.Now().Add(time.Hour)))
	}
	require.NoError(t, f.sessions.Save(ctx, "bob", sessionOf(other), time.Now().Add(time.Hour)))
	require.NoError(t, f.svc.Request(ctx, "ada@example.com"))
	r, err := f.svc.Lookup(ctx, linkToken(t, f.mail.sent[0]))
	require.NoError(t, err)

	require.NoError(t, f.svc.Complete(ctx, r.PublicID, "a brand new passphrase"))
	assert.NoError(t, f.fixture.svc.verify(u.PasswordHash, "a brand new passphrase"))
	for sid, want := range map[string]bool{"s1": false, "s2": false, "bob": true} {
		_, ok, err := f.sessions.Load(ctx, sid)
		require.NoError(t, err)
		assert.Equal(t, want, ok, sid)
	}
	require.Len(t, f.mail.sent, 2)
	changed := f.mail.sent[1]
	assert.Equal(t, "ada@example.com", changed.To)
	assert.Equal(t, "Your oagent password was changed", changed.Subject)
	assert.Contains(t, changed.TextBody, "https://oagent.example.com/app/login")

	err = f.svc.Complete(ctx, r.PublicID, "another new passphrase")
	assert.Equal(t, ResetUsed, ResetReasonOf(err), "a link works once")
}

func TestResets_CompleteRefusals(t *testing.T) {
	f := newResetFixture(t)
	f.addUser(t, "ada@example.com", true)
	require.NoError(t, f.svc.Request(t.Context(), "ada@example.com"))
	r, err := f.svc.Lookup(t.Context(), linkToken(t, f.mail.sent[0]))
	require.NoError(t, err)

	err = f.svc.Complete(t.Context(), r.PublicID, "short")
	assert.Error(t, err)
	assert.False(t, IsResetNotUsableError(err), "a weak password is a policy error")
	assert.Nil(t, f.store.rows[r.PublicID].UsedAt, "and leaves the link usable")

	err = f.svc.Complete(t.Context(), "nope", "a brand new passphrase")
	assert.Equal(t, ResetNotFound, ResetReasonOf(err))
}

func sessionOf(u *user.User) session.Principal {
	return session.Principal{UserID: u.ID, Email: u.Email, Source: session.SourceLocal}
}
