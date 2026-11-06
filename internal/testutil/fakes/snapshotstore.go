package fakes

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/run"
)

// SnapshotStore is an in-memory run.SnapshotStore, safe for concurrent use; Err fails every call.
type SnapshotStore struct {
	Err   error
	mu    sync.Mutex
	snaps map[uuid.UUID]run.Snapshot
}

var _ run.SnapshotStore = (*SnapshotStore)(nil)

func (f *SnapshotStore) org(ctx context.Context) (uuid.UUID, error) {
	if f.Err != nil {
		return uuid.Nil, f.Err
	}
	tc, err := tenant.From(ctx)
	return tc.OrgID, err
}

// Load implements run.SnapshotStore.
func (f *SnapshotStore) Load(ctx context.Context, threadID uuid.UUID) (*run.Snapshot, error) {
	org, err := f.org(ctx)
	if err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	snap, ok := f.snaps[threadID]
	if !ok || snap.OrgID != org {
		return nil, nil
	}
	snap.Blob = append([]byte(nil), snap.Blob...)
	return &snap, nil
}

// Save implements run.SnapshotStore; a snapshot of another org's thread is refused.
func (f *SnapshotStore) Save(ctx context.Context, snap *run.Snapshot) error {
	org, err := f.org(ctx)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if known, ok := f.snaps[snap.ThreadID]; snap.OrgID != org || (ok && known.OrgID != org) {
		return &run.ThreadNotFoundError{ID: snap.ThreadID.String()}
	}
	if f.snaps == nil {
		f.snaps = map[uuid.UUID]run.Snapshot{}
	}
	snap.UpdatedAt = time.Now().UTC()
	saved := *snap
	saved.Blob = append([]byte(nil), snap.Blob...)
	f.snaps[snap.ThreadID] = saved
	return nil
}

// Delete implements run.SnapshotStore.
func (f *SnapshotStore) Delete(ctx context.Context, threadID uuid.UUID) error {
	org, err := f.org(ctx)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if snap, ok := f.snaps[threadID]; ok && snap.OrgID == org {
		delete(f.snaps, threadID)
	}
	return nil
}
