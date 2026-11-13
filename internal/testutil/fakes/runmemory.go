package fakes

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/run"
)

// RunMemory is an in-memory run.Memory; it records every recall and record it gets.
type RunMemory struct {
	mu sync.Mutex
	// ExtractsFacts and APIUsers are the store mode Store reports; StoreErr fails it, as a gone store or a disabled backend does.
	ExtractsFacts bool
	APIUsers      bool
	StoreErr      error
	Stores        []run.MemoryScope
	Facts         []run.RecalledFact
	RecallErr     error
	RecordErr     error
	Recalled      []run.MemoryScope
	Queries       []string
	Records       []RecordedMemory
}

// RecordedMemory is one Record call, with whether its context was already done.
type RecordedMemory struct {
	Scope     run.MemoryScope
	Record    run.MemoryRecord
	Cancelled bool
}

var _ run.Memory = (*RunMemory)(nil)

// Store implements run.Memory.
func (f *RunMemory) Store(_ context.Context, scope run.MemoryScope) (run.StoreMode, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Stores = append(f.Stores, scope)
	return run.StoreMode{Extracts: f.ExtractsFacts, APIUsers: f.APIUsers}, f.StoreErr
}

// Recall implements run.Memory.
func (f *RunMemory) Recall(_ context.Context, scope run.MemoryScope, query string) ([]run.RecalledFact, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Recalled, f.Queries = append(f.Recalled, scope), append(f.Queries, query)
	return f.Facts, f.RecallErr
}

// Record implements run.Memory.
func (f *RunMemory) Record(ctx context.Context, scope run.MemoryScope, rec run.MemoryRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Records = append(f.Records, RecordedMemory{Scope: scope, Record: rec, Cancelled: ctx.Err() != nil})
	return f.RecordErr
}

// RunRememberers is an in-memory run.Rememberers: the end users in People are always remembered, those in Named are named API users.
type RunRememberers struct {
	People map[uuid.UUID]bool
	Named  map[uuid.UUID]bool
	Err    error
}

var _ run.Rememberers = (*RunRememberers)(nil)

// Rememberable implements run.Rememberers.
func (f *RunRememberers) Rememberable(_ context.Context, endUserID uuid.UUID) (run.Rememberer, error) {
	switch {
	case f.People[endUserID]:
		return run.RememberAlways, f.Err
	case f.Named[endUserID]:
		return run.RememberNamedAPIUser, f.Err
	}
	return run.RememberNever, f.Err
}
