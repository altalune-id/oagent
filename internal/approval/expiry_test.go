package approval_test

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/approval"
	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/scheduler"
)

func expireJob(t *testing.T, w *world) scheduler.Job {
	t.Helper()
	for _, j := range w.svc.Jobs(15 * time.Second) {
		if j.Name == "approval-expire" {
			assert.Equal(t, scheduler.ScopeTenant, j.Scope)
			return j
		}
	}
	t.Fatal("no approval-expire job")
	return scheduler.Job{}
}

func TestApproval_ExpiryRejects(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := newWorld(t)
		w.now = time.Now()
		group := w.create(t, "group")
		w.thread, w.run = uuid.New(), uuid.New()
		w.refs.threads[w.thread], w.refs.runs[w.run] = "ThQ2wE4rT6yU8jP9pA3sE", "RnZ3xC5vB7nM9qW2eR2tZ"
		dm := w.create(t, "dm")
		job := expireJob(t, w)

		time.Sleep(10*time.Minute + time.Second)
		require.NoError(t, job.Run(w.ctx))
		got, err := w.svc.Get(w.ctx, group.ID)
		require.NoError(t, err)
		assert.Equal(t, approval.StatusExpired, got.Status)
		assert.Equal(t, approval.DeciderSystem, got.DecidedByKind)
		require.NotNil(t, got.DecidedAt)
		assert.Equal(t, []uuid.UUID{group.ThreadID}, w.kicks.threads, "the thread is kicked so the run resumes with the expiry reject")
		var decided int
		for _, e := range w.pub.Events() {
			if e.Type == events.ApprovalDecided {
				decided++
			}
		}
		assert.Equal(t, 1, decided)

		still, err := w.svc.Get(w.ctx, dm.ID)
		require.NoError(t, err)
		assert.Equal(t, approval.StatusPending, still.Status, "a DM approval waits its 24 h")

		require.NoError(t, job.Run(w.ctx))
		assert.Len(t, w.kicks.threads, 1, "an expired row is never expired or kicked twice")
	})
}
