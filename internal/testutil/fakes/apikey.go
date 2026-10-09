package fakes

import (
	"context"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/apikey"
)

// APIKeyStore is an apikey.Store test double. ByID/List return Key/Keys (or Err); Save records the key and returns SaveFn/Err.
type APIKeyStore struct {
	Err    error
	Key    *apikey.APIKey
	Keys   []*apikey.APIKey
	SaveFn func(*apikey.APIKey) error
	Saved  *apikey.APIKey
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
