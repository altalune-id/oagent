package legal

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/testutil/fakes"
	"altalune.id/oagent/internal/user"
)

type memStore struct {
	snapshots   map[string]Snapshot
	acceptances []Acceptance
	err         error
	now         time.Time
}

func (m *memStore) Publish(_ context.Context, s Snapshot) (Snapshot, error) {
	if m.err != nil {
		return Snapshot{}, m.err
	}
	key := s.Slug + "@" + s.Version
	if got, ok := m.snapshots[key]; ok {
		return got, nil
	}
	s.PublishedAt = m.now
	m.snapshots[key] = s
	return s, nil
}

func (m *memStore) RecordAcceptance(_ context.Context, a Acceptance) error {
	if m.err != nil {
		return m.err
	}
	m.acceptances = append(m.acceptances, a)
	return nil
}

type acceptor struct{ users *user.Service }

func (a acceptor) AcceptTerms(ctx context.Context, id uuid.UUID) (*user.User, error) {
	return a.users.AcceptTerms(ctx, id)
}

func historyFixture(t *testing.T) (*Service, *memStore, *user.Service) {
	t.Helper()
	users := user.NewService(fakes.NewUserStore(), apperror.NewReporter(nil, false).Unexpected)
	store := &memStore{snapshots: map[string]Snapshot{}, now: time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)}
	svc := NewService(acceptor{users}).WithStore(store, fakes.Transactor{})
	return svc, store, users
}

func TestService_PublishStoresTheBuiltInVersions(t *testing.T) {
	svc, store, _ := historyFixture(t)
	require.NoError(t, svc.Publish(context.Background()))
	require.Len(t, store.snapshots, 2)
	terms := store.snapshots["terms@2026-10-11"]
	assert.Equal(t, "Terms of Service", terms.Title)
	assert.Contains(t, terms.Body, "## 1.")
	assert.Len(t, terms.SHA256, 64)
	assert.Equal(t, time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC), svc.TermsUpdatedAt(), "the gate asks again from when this version was first served")

	again, store2, _ := historyFixture(t)
	earlier := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	store2.snapshots["terms@2026-10-11"] = Snapshot{ID: uuid.New(), Slug: "terms", Version: "2026-10-11", SHA256: terms.SHA256, PublishedAt: earlier}
	require.NoError(t, again.Publish(context.Background()))
	assert.Equal(t, time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC), again.TermsUpdatedAt(), "the newer of the two")
}

func TestService_PublishRefusesChangedText(t *testing.T) {
	svc, store, _ := historyFixture(t)
	store.snapshots["privacy@2026-10-11"] = Snapshot{ID: uuid.New(), Slug: "privacy", Version: "2026-10-11", SHA256: "other"}
	err := svc.Publish(context.Background())
	var changed *VersionChangedError
	require.True(t, errors.As(err, &changed), "got %v", err)
	assert.Equal(t, "privacy", changed.Slug)
	assert.Contains(t, err.Error(), "2026-10-11")
}

func TestService_PublishSkipsExternalTermsAndFailures(t *testing.T) {
	svc, store, _ := historyFixture(t)
	svc.WithExternalTerms(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, svc.Publish(context.Background()))
	assert.Empty(t, store.snapshots, "custom terms keep no history")
	require.NoError(t, NewService(nil).Publish(context.Background()), "no store, nothing to publish")

	failing, store2, _ := historyFixture(t)
	store2.err = errors.New("db down")
	assert.Error(t, failing.Publish(context.Background()))
}

func TestService_AcceptRecordsTheVersions(t *testing.T) {
	svc, store, users := historyFixture(t)
	require.NoError(t, svc.Publish(context.Background()))
	u, err := users.Create(context.Background(), user.CreateRequest{Email: "ada@example.com", Password: "a sturdy passphrase"})
	require.NoError(t, err)

	at, err := svc.Accept(context.Background(), u.ID, SourceJoin)
	require.NoError(t, err)
	require.Len(t, store.acceptances, 1)
	a := store.acceptances[0]
	assert.Equal(t, u.ID, a.UserID)
	assert.Equal(t, store.snapshots["terms@2026-10-11"].ID, a.TermsDocumentID)
	assert.Equal(t, store.snapshots["privacy@2026-10-11"].ID, a.PrivacyDocumentID)
	assert.Equal(t, SourceJoin, a.Source)
	assert.Equal(t, at, a.AcceptedAt)
	assert.Len(t, a.PublicID, 21)

	store.err = errors.New("db down")
	_, err = svc.Accept(context.Background(), u.ID, SourceWelcome)
	assert.Error(t, err)
	_, err = svc.Accept(context.Background(), uuid.New(), SourceWelcome)
	assert.Error(t, err, "an unknown person")
}

func TestService_AcceptWithoutHistory(t *testing.T) {
	svc, store, users := historyFixture(t)
	svc.WithExternalTerms(time.Time{})
	require.NoError(t, svc.Publish(context.Background()))
	u, err := users.Create(context.Background(), user.CreateRequest{Email: "ada@example.com", Password: "a sturdy passphrase"})
	require.NoError(t, err)
	_, err = svc.Accept(context.Background(), u.ID, SourceWelcome)
	require.NoError(t, err)
	assert.Empty(t, store.acceptances, "custom terms keep no history")
}

func TestVersionChangedError(t *testing.T) {
	err := &VersionChangedError{Slug: "terms", Version: "2026-10-11"}
	assert.Equal(t, `legal: the text of terms version "2026-10-11" changed; set a new updated: in content/terms.md`, err.Error())
}
