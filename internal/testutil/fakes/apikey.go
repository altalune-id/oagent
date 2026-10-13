package fakes

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/apikey"
)

// APIKeyStore is an apikey.Store test double: reads return Key/Keys or Err, writes record their input.
type APIKeyStore struct {
	Err         error
	Key         *apikey.APIKey
	Keys        []*apikey.APIKey
	SaveFn      func(*apikey.APIKey) error
	Saved       *apikey.APIKey
	HashLookup  int
	RevokeCount int
	Revocations []PersonalRevocation

	mu      sync.Mutex
	Touched []map[uuid.UUID]time.Time
}

var _ apikey.Store = (*APIKeyStore)(nil)

// Save implements apikey.Store.
func (f *APIKeyStore) Save(_ context.Context, k *apikey.APIKey) error {
	f.Saved = k
	if f.SaveFn != nil {
		return f.SaveFn(k)
	}
	return f.Err
}

// PersonalRevocation is one recorded RevokePersonalOf call.
type PersonalRevocation struct {
	OrgID, OwnerID uuid.UUID
	At             time.Time
}

// RevokePersonalOf implements apikey.Store and records the call.
func (f *APIKeyStore) RevokePersonalOf(_ context.Context, orgID, ownerID uuid.UUID, at time.Time) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return 0, f.Err
	}
	f.Revocations = append(f.Revocations, PersonalRevocation{OrgID: orgID, OwnerID: ownerID, At: at})
	return f.RevokeCount, nil
}

// ByID implements apikey.Store.
func (f *APIKeyStore) ByID(context.Context, uuid.UUID) (*apikey.APIKey, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	return f.Key, nil
}

// ByHint implements apikey.Store; it finds a live Key or one of Keys in orgID by hint.
func (f *APIKeyStore) ByHint(_ context.Context, orgID uuid.UUID, hint string) (*apikey.APIKey, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	for _, k := range append([]*apikey.APIKey{f.Key}, f.Keys...) {
		if k != nil && k.OrgID == orgID && k.SecretHint == hint && k.RevokedAt == nil {
			return k, nil
		}
	}
	return nil, &apikey.NotFoundError{}
}

// List implements apikey.Store.
func (f *APIKeyStore) List(context.Context, uuid.UUID) ([]*apikey.APIKey, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	return f.Keys, nil
}

// BySecretHash implements apikey.Store; a nil Key is not found.
func (f *APIKeyStore) BySecretHash(_ context.Context, hash [32]byte) (*apikey.APIKey, error) {
	f.HashLookup++
	if f.Err != nil {
		return nil, f.Err
	}
	if f.Key == nil || f.Key.SecretHash != hash {
		return nil, &apikey.NotFoundError{}
	}
	return f.Key, nil
}

// TouchLastUsed implements apikey.Store and records each batch.
func (f *APIKeyStore) TouchLastUsed(_ context.Context, used map[uuid.UUID]time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Touched = append(f.Touched, used)
	return f.Err
}

// Batches returns the TouchLastUsed batches recorded so far.
func (f *APIKeyStore) Batches() []map[uuid.UUID]time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]map[uuid.UUID]time.Time(nil), f.Touched...)
}
