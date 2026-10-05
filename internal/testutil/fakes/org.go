package fakes

import (
	"context"

	"github.com/google/uuid"

	"altalune.id/auth-server/internal/org"
)

// OrgStore is an org.Store test double. ByID/BySlug/List return Org/Orgs (or Err); Save records the org and returns SaveFn/Err.
type OrgStore struct {
	Err    error
	Org    *org.Org
	Orgs   []*org.Org
	SaveFn func(*org.Org) error
	Saved  *org.Org
}

var _ org.Store = (*OrgStore)(nil)

// Save implements org.Store.
func (f *OrgStore) Save(_ context.Context, o *org.Org) error {
	f.Saved = o
	if f.SaveFn != nil {
		return f.SaveFn(o)
	}
	return f.Err
}

// ByID implements org.Store.
func (f *OrgStore) ByID(context.Context, uuid.UUID) (*org.Org, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	return f.Org, nil
}

// BySlug implements org.Store.
func (f *OrgStore) BySlug(context.Context, string) (*org.Org, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	return f.Org, nil
}

// List implements org.Store.
func (f *OrgStore) List(context.Context) ([]*org.Org, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	return f.Orgs, nil
}
