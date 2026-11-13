// Package snapshotstoretest is the contract every run.SnapshotStore driver passes.
package snapshotstoretest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/run"
)

// Harness is a store under test and a way to make threads it accepts.
type Harness struct {
	Store run.SnapshotStore
	// Thread returns a tenant context and a snapshot naming a fresh thread of that tenant.
	Thread func(t *testing.T) (context.Context, run.Snapshot)
}

// Run runs the contract against a fresh harness per case.
func Run(t *testing.T, newHarness func(t *testing.T) Harness) {
	t.Helper()
	t.Run("missing loads as nil", func(t *testing.T) {
		h := newHarness(t)
		ctx, snap := h.Thread(t)
		got, err := h.Store.Load(ctx, snap.ThreadID)
		require.NoError(t, err)
		assert.Nil(t, got)
	})
	t.Run("save then load round trips", func(t *testing.T) {
		h := newHarness(t)
		ctx, snap := h.Thread(t)
		snap.SDKVersion, snap.Blob = "1.0.0", []byte{0, 1, 2, 255}
		require.NoError(t, h.Store.Save(ctx, &snap))
		got, err := h.Store.Load(ctx, snap.ThreadID)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, snap.ThreadID, got.ThreadID)
		assert.Equal(t, snap.OrgID, got.OrgID)
		assert.Equal(t, snap.ProjectID, got.ProjectID)
		assert.Equal(t, snap.AgentVersionID, got.AgentVersionID)
		assert.Equal(t, "1.0.0", got.SDKVersion)
		assert.Equal(t, []byte{0, 1, 2, 255}, got.Blob)
		assert.False(t, got.UpdatedAt.IsZero())
	})
	t.Run("save overwrites", func(t *testing.T) {
		h := newHarness(t)
		ctx, snap := h.Thread(t)
		snap.SDKVersion, snap.Blob = "1.0.0", []byte("old")
		require.NoError(t, h.Store.Save(ctx, &snap))
		snap.SDKVersion, snap.Blob = "1.1.0", []byte("new")
		require.NoError(t, h.Store.Save(ctx, &snap))
		got, err := h.Store.Load(ctx, snap.ThreadID)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "1.1.0", got.SDKVersion)
		assert.Equal(t, []byte("new"), got.Blob)
	})
	t.Run("delete removes and is idempotent", func(t *testing.T) {
		h := newHarness(t)
		ctx, snap := h.Thread(t)
		snap.SDKVersion, snap.Blob = "1.0.0", []byte("x")
		require.NoError(t, h.Store.Save(ctx, &snap))
		require.NoError(t, h.Store.Delete(ctx, snap.ThreadID))
		got, err := h.Store.Load(ctx, snap.ThreadID)
		require.NoError(t, err)
		assert.Nil(t, got)
		require.NoError(t, h.Store.Delete(ctx, snap.ThreadID))
	})
	t.Run("another tenant is invisible", func(t *testing.T) {
		h := newHarness(t)
		ctx, snap := h.Thread(t)
		other, _ := h.Thread(t)
		snap.SDKVersion, snap.Blob = "1.0.0", []byte("mine")
		require.NoError(t, h.Store.Save(ctx, &snap))
		got, err := h.Store.Load(other, snap.ThreadID)
		require.NoError(t, err)
		assert.Nil(t, got, "another tenant does not see it")
		require.NoError(t, h.Store.Delete(other, snap.ThreadID))
		forged := snap
		forged.Blob = []byte("forged")
		assert.True(t, run.IsThreadNotFoundError(h.Store.Save(other, &forged)), "another tenant cannot write it")
		forged.OrgID = tenant.Peek(other).OrgID
		assert.True(t, run.IsThreadNotFoundError(h.Store.Save(other, &forged)), "a row in the other tenant's name meets the conflict guard")
		got, err = h.Store.Load(ctx, snap.ThreadID)
		require.NoError(t, err)
		require.NotNil(t, got, "nor delete it")
		assert.Equal(t, []byte("mine"), got.Blob)
	})
}
