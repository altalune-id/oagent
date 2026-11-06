package run_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/scheduler"
)

func job(t *testing.T, jobs []scheduler.Job, name string) scheduler.Job {
	t.Helper()
	for _, j := range jobs {
		if j.Name == name {
			assert.Equal(t, scheduler.ScopeTenant, j.Scope, "%s runs per tenant", name)
			return j
		}
	}
	t.Fatalf("no job %s", name)
	return scheduler.Job{}
}

func TestJobs_SweepReclaimsAndKicks(t *testing.T) {
	f := newSvc(t)
	f.say(t, "hello")
	k := &kicks{}
	jobs := run.Jobs(f.store, k, 15*time.Second)
	token := f.claim(t)
	r := run.New(f.org, f.project, f.thread, f.version.ID, time.Now())
	_, err := f.store.StartRun(f.ctx, token, r)
	require.NoError(t, err)

	require.NoError(t, job(t, jobs, "run-sweep").Run(f.ctx))
	assert.Empty(t, k.got, "a live lease is left alone")

	f.store.Threads[f.thread].LeaseUntil = time.Now().Add(-time.Second)
	require.NoError(t, job(t, jobs, "run-sweep").Run(f.ctx))
	assert.Equal(t, []run.Kick{f.kick()}, k.got)
	open, err := f.store.OpenRun(f.ctx, f.thread)
	require.NoError(t, err)
	require.NotNil(t, open)
	assert.Equal(t, r.ID, open.ID, "the run is interrupted and waits to resume")
}

func TestJobs_ReconcileKicksPendingThreads(t *testing.T) {
	f := newSvc(t)
	k := &kicks{}
	jobs := run.Jobs(f.store, k, 15*time.Second)
	require.NoError(t, job(t, jobs, "run-reconcile").Run(f.ctx))
	assert.Empty(t, k.got)
	f.say(t, "hello")
	require.NoError(t, job(t, jobs, "run-reconcile").Run(f.ctx))
	assert.Equal(t, []run.Kick{f.kick()}, k.got)
}

func TestJobs_DefaultCadence(t *testing.T) {
	for _, j := range run.Jobs(nil, nil, 0) {
		assert.False(t, j.Singleton, "both jobs are safe on every replica")
		assert.Equal(t, 15*time.Second, j.Timeout)
	}
}
