package storage

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/scheduler"
)

func TestSweep_RemovesDeletingAndExpiredUploadsOnly(t *testing.T) {
	h := newHarness(t)
	ready := h.begin(t)
	require.NoError(t, h.svc.MarkReady(context.Background(), ready.ID))
	fresh := h.begin(t)
	expired := h.begin(t)
	replaced := h.begin(t)
	for _, f := range []*File{ready, fresh, expired, replaced} {
		h.driver.put(f.ObjectKey, "application/pdf", pdfBody)
	}
	require.NoError(t, h.svc.MarkDeleting(context.Background(), replaced.ID))
	h.store.files[expired.ID].ExpiresAt = h.now.Add(-time.Second)
	ab := &fakeAbandoned{}

	n, err := h.svc.Sweep(context.Background(), ab)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.Nil(t, h.store.get(expired.ID))
	assert.Nil(t, h.store.get(replaced.ID))
	assert.False(t, h.driver.has(expired.ObjectKey))
	assert.False(t, h.driver.has(replaced.ObjectKey))
	assert.NotNil(t, h.store.get(ready.ID))
	assert.NotNil(t, h.store.get(fresh.ID))
	assert.True(t, h.driver.has(ready.ObjectKey))
	assert.Equal(t, []uuid.UUID{expired.ID}, ab.files, "only an abandoned upload is reported, not a file handed over for deletion")
}

func TestSweep_KeepsAnAbandonedUploadWhenItsOwnerCannotHear(t *testing.T) {
	h := newHarness(t)
	f := h.begin(t)
	h.driver.put(f.ObjectKey, "application/pdf", pdfBody)
	h.store.files[f.ID].ExpiresAt = h.now.Add(-time.Second)

	n, err := h.svc.Sweep(context.Background(), &fakeAbandoned{err: errFake})
	assert.Zero(t, n)
	require.ErrorIs(t, err, errFake)
	assert.NotNil(t, h.store.get(f.ID), "retried on the next run, so the waiting document is never left behind")
	assert.True(t, h.driver.has(f.ObjectKey))
}

func TestSweep_KeepsTheRowWhenTheObjectCannotBeDeleted(t *testing.T) {
	h := newHarness(t)
	f := h.begin(t)
	require.NoError(t, h.svc.MarkDeleting(context.Background(), f.ID))
	h.driver.failOn = "delete"
	n, err := h.svc.Sweep(context.Background(), nil)
	assert.Zero(t, n)
	require.ErrorIs(t, err, errFake)
	assert.NotNil(t, h.store.get(f.ID), "retried on the next run")

	h.driver.failOn, h.store.failOn = "", "delete"
	_, err = h.svc.Sweep(context.Background(), nil)
	require.ErrorIs(t, err, errFake)

	h.store.failOn = "sweepable"
	_, err = h.svc.Sweep(context.Background(), nil)
	require.ErrorIs(t, err, errFake)
}

func TestJobs_OneTenantSweep(t *testing.T) {
	h := newHarness(t)
	f := h.begin(t)
	require.NoError(t, h.svc.MarkDeleting(context.Background(), f.ID))
	jobs := h.svc.Jobs(nil)
	require.Len(t, jobs, 1)
	j := jobs[0]
	assert.Equal(t, "storage-sweep", j.Name)
	assert.Equal(t, scheduler.ScopeTenant, j.Scope)
	assert.Positive(t, j.Timeout)

	require.NoError(t, j.Run(context.Background()))
	assert.Nil(t, h.store.get(f.ID))
}
