package fakes

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/apikey"
)

// APIKeyStore is an apikey.Store test double. ByID/List/BySecretHash return Key/Keys (or Err); Save records the key and returns SaveFn/Err.
type APIKeyStore struct {
	Err        error
	Key        *apikey.APIKey
	Keys       []*apikey.APIKey
	SaveFn     func(*apikey.APIKey) error
	Saved      *apikey.APIKey
	HashLookup int

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

// ByID implements apikey.Store.
func (f *APIKeyStore) ByID(context.Context, uuid.UUID) (*apikey.APIKey, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	return f.Key, nil
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
