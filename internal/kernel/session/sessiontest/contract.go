// Package sessiontest holds the behavior every session.Store must share.
package sessiontest

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/session"
)

// RunStoreContract runs the shared Store behavior against stores built by newStore.
func RunStoreContract(t *testing.T, newStore func(t *testing.T) session.Store) {
	t.Helper()
	ctx := context.Background()
	live := time.Now().Add(time.Hour)
	past := time.Now().Add(-time.Minute)
	person := func() session.Principal {
		return session.Principal{UserID: uuid.New(), Email: "a@example.com", Source: session.SourceLocal, ActiveOrgID: uuid.New()}
	}

	t.Run("save then load", func(t *testing.T) {
		s, p := newStore(t), person()
		require.NoError(t, s.Save(ctx, "sid-a", p, live))
		got, ok, err := s.Load(ctx, "sid-a")
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, p.UserID, got.UserID)
		assert.Equal(t, p.Email, got.Email)
		assert.Equal(t, p.Source, got.Source)
		assert.Equal(t, p.ActiveOrgID, got.ActiveOrgID)
	})
	t.Run("authority is not stored", func(t *testing.T) {
		s, p := newStore(t), person()
		p.Scopes = []string{"*"}
		p.Operator = true
		p.AllProjects = true
		p.ProjectIDs = []uuid.UUID{uuid.New()}
		p.ResourceIDs = []uuid.UUID{uuid.New()}
		require.NoError(t, s.Save(ctx, "sid-a", p, live))
		got, ok, err := s.Load(ctx, "sid-a")
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, p.UserID, got.UserID)
		assert.Empty(t, got.Scopes)
		assert.False(t, got.Operator)
		assert.False(t, got.AllProjects)
		assert.Empty(t, got.ProjectIDs)
		assert.Empty(t, got.ResourceIDs)
	})
	t.Run("missing sid is not found", func(t *testing.T) {
		_, ok, err := newStore(t).Load(ctx, "nope")
		require.NoError(t, err)
		assert.False(t, ok)
	})
	t.Run("expired is not loaded", func(t *testing.T) {
		s := newStore(t)
		require.NoError(t, s.Save(ctx, "sid-old", person(), past))
		_, ok, err := s.Load(ctx, "sid-old")
		require.NoError(t, err)
		assert.False(t, ok)
	})
	t.Run("save again replaces", func(t *testing.T) {
		s, p := newStore(t), person()
		require.NoError(t, s.Save(ctx, "sid-a", p, live))
		p.ActiveOrgID = uuid.New()
		require.NoError(t, s.Save(ctx, "sid-a", p, live))
		got, ok, err := s.Load(ctx, "sid-a")
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, p.ActiveOrgID, got.ActiveOrgID)
	})
	t.Run("re-save under another user moves the session", func(t *testing.T) {
		s, alice, bob := newStore(t), person(), person()
		require.NoError(t, s.Save(ctx, "sid-x", alice, live))
		require.NoError(t, s.Save(ctx, "sid-x", bob, live))
		require.NoError(t, s.DeleteByUser(ctx, alice.UserID))
		got, ok, err := s.Load(ctx, "sid-x")
		require.NoError(t, err)
		require.True(t, ok, "deleting the previous user's sessions must not touch it")
		assert.Equal(t, bob.UserID, got.UserID)
		require.NoError(t, s.DeleteByUser(ctx, bob.UserID))
		_, ok, err = s.Load(ctx, "sid-x")
		require.NoError(t, err)
		assert.False(t, ok, "deleting the current user's sessions removes it")
	})
	t.Run("delete", func(t *testing.T) {
		s := newStore(t)
		require.NoError(t, s.Save(ctx, "sid-a", person(), live))
		require.NoError(t, s.Delete(ctx, "sid-a"))
		_, ok, err := s.Load(ctx, "sid-a")
		require.NoError(t, err)
		assert.False(t, ok)
	})
	t.Run("delete by user keeps other users", func(t *testing.T) {
		s, alice, bob := newStore(t), person(), person()
		require.NoError(t, s.Save(ctx, "alice-1", alice, live))
		require.NoError(t, s.Save(ctx, "alice-2", alice, live))
		require.NoError(t, s.Save(ctx, "bob-1", bob, live))
		require.NoError(t, s.DeleteByUser(ctx, alice.UserID))
		for _, sid := range []string{"alice-1", "alice-2"} {
			_, ok, err := s.Load(ctx, sid)
			require.NoError(t, err)
			assert.False(t, ok, sid)
		}
		_, ok, err := s.Load(ctx, "bob-1")
		require.NoError(t, err)
		assert.True(t, ok)
	})
	t.Run("delete by user except keeps one session and other users", func(t *testing.T) {
		s, alice, bob := newStore(t), person(), person()
		require.NoError(t, s.Save(ctx, "alice-1", alice, live))
		require.NoError(t, s.Save(ctx, "alice-2", alice, live))
		require.NoError(t, s.Save(ctx, "alice-3", alice, live))
		require.NoError(t, s.Save(ctx, "bob-1", bob, live))
		require.NoError(t, s.DeleteByUserExcept(ctx, alice.UserID, "alice-2"))
		for sid, want := range map[string]bool{"alice-1": false, "alice-2": true, "alice-3": false, "bob-1": true} {
			_, ok, err := s.Load(ctx, sid)
			require.NoError(t, err)
			assert.Equal(t, want, ok, sid)
		}
		require.NoError(t, s.DeleteByUserExcept(ctx, bob.UserID, "alice-2"))
		_, ok, err := s.Load(ctx, "alice-2")
		require.NoError(t, err)
		assert.True(t, ok, "another user's keepSID does not touch it")
		_, ok, err = s.Load(ctx, "bob-1")
		require.NoError(t, err)
		assert.False(t, ok)
		require.NoError(t, s.DeleteByUserExcept(ctx, alice.UserID, ""))
		_, ok, err = s.Load(ctx, "alice-2")
		require.NoError(t, err)
		assert.False(t, ok, "an empty keepSID deletes them all")
	})
	t.Run("delete expired counts and keeps live", func(t *testing.T) {
		s := newStore(t)
		require.NoError(t, s.Save(ctx, "old-1", person(), past))
		require.NoError(t, s.Save(ctx, "old-2", person(), past))
		require.NoError(t, s.Save(ctx, "live", person(), live))
		n, err := s.DeleteExpired(ctx)
		require.NoError(t, err)
		assert.Equal(t, 2, n)
		_, ok, err := s.Load(ctx, "live")
		require.NoError(t, err)
		assert.True(t, ok)
	})
}
