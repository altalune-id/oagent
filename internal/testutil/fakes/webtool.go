package fakes

import (
	"cmp"
	"context"
	"slices"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/webtool"
)

// WebToolStore is an in-memory webtool.Store scoped like the real one by the org on ctx; Err fails every call.
type WebToolStore struct {
	Err         error
	Providers   map[uuid.UUID]*webtool.Provider
	HTTPSecrets []*webtool.HTTPSecret
}

var _ webtool.Store = (*WebToolStore)(nil)

func (f *WebToolStore) org(ctx context.Context) (uuid.UUID, error) {
	if f.Err != nil {
		return uuid.Nil, f.Err
	}
	tc, err := tenant.From(ctx)
	return tc.OrgID, err
}

// Provider implements webtool.Store.
func (f *WebToolStore) Provider(ctx context.Context, projectID uuid.UUID) (*webtool.Provider, error) {
	org, err := f.org(ctx)
	if err != nil {
		return nil, err
	}
	p, ok := f.Providers[projectID]
	if !ok || p.OrgID != org {
		return nil, &webtool.NotFoundError{}
	}
	c := *p
	return &c, nil
}

// Secrets implements webtool.Store.
func (f *WebToolStore) Secrets(ctx context.Context, projectID uuid.UUID) ([]*webtool.HTTPSecret, error) {
	return f.secrets(ctx, projectID, nil)
}

// SecretsForHosts implements webtool.Store.
func (f *WebToolStore) SecretsForHosts(ctx context.Context, projectID uuid.UUID, hosts []string) ([]*webtool.HTTPSecret, error) {
	if len(hosts) == 0 {
		return nil, f.Err
	}
	return f.secrets(ctx, projectID, hosts)
}

func (f *WebToolStore) secrets(ctx context.Context, projectID uuid.UUID, hosts []string) ([]*webtool.HTTPSecret, error) {
	org, err := f.org(ctx)
	if err != nil {
		return nil, err
	}
	var out []*webtool.HTTPSecret
	for _, s := range f.HTTPSecrets {
		if s.OrgID == org && s.ProjectID == projectID && (hosts == nil || slices.Contains(hosts, s.Host)) {
			c := *s
			out = append(out, &c)
		}
	}
	slices.SortFunc(out, func(a, b *webtool.HTTPSecret) int {
		return cmp.Or(cmp.Compare(a.Host, b.Host), cmp.Compare(a.Header, b.Header))
	})
	return out, nil
}

// SecretCount implements webtool.Store.
func (f *WebToolStore) SecretCount(ctx context.Context, projectID uuid.UUID, host, header string) (total int, exists bool, err error) {
	list, err := f.Secrets(ctx, projectID)
	if err != nil {
		return 0, false, err
	}
	return len(list), slices.ContainsFunc(list, func(s *webtool.HTTPSecret) bool { return s.Host == host && s.Header == header }), nil
}

// ResolveSecret implements webtool.Store; it needs no org on ctx, like the definer.
func (f *WebToolStore) ResolveSecret(_ context.Context, publicID string) (webtool.Ref, error) {
	if f.Err != nil {
		return webtool.Ref{}, f.Err
	}
	for _, s := range f.HTTPSecrets {
		if s.PublicID == publicID {
			return webtool.Ref{ID: s.ID, OrgID: s.OrgID, ProjectID: s.ProjectID}, nil
		}
	}
	return webtool.Ref{}, &webtool.NotFoundError{}
}

// SaveProvider implements webtool.Store.
func (f *WebToolStore) SaveProvider(ctx context.Context, p *webtool.Provider) error {
	org, err := f.org(ctx)
	if err != nil {
		return err
	}
	if old, ok := f.Providers[p.ProjectID]; p.OrgID != org || (ok && old.OrgID != org) {
		return &webtool.NotFoundError{}
	}
	if f.Providers == nil {
		f.Providers = map[uuid.UUID]*webtool.Provider{}
	}
	c := *p
	f.Providers[p.ProjectID] = &c
	return nil
}

// DeleteProvider implements webtool.Store.
func (f *WebToolStore) DeleteProvider(ctx context.Context, projectID uuid.UUID) error {
	org, err := f.org(ctx)
	if err != nil {
		return err
	}
	if p, ok := f.Providers[projectID]; ok && p.OrgID == org {
		delete(f.Providers, projectID)
	}
	return nil
}

// SaveSecret implements webtool.Store; a secret for a stored host and header replaces its value and keeps its ids.
func (f *WebToolStore) SaveSecret(ctx context.Context, s *webtool.HTTPSecret) (*webtool.HTTPSecret, error) {
	org, err := f.org(ctx)
	if err != nil {
		return nil, err
	}
	if s.OrgID != org {
		return nil, &webtool.NotFoundError{}
	}
	for _, old := range f.HTTPSecrets {
		if old.ProjectID == s.ProjectID && old.Host == s.Host && old.Header == s.Header {
			if old.OrgID != org {
				return nil, &webtool.NotFoundError{}
			}
			old.ValueSealed, old.UpdatedAt = s.ValueSealed, s.UpdatedAt
			c := *old
			return &c, nil
		}
	}
	c := *s
	f.HTTPSecrets = append(f.HTTPSecrets, &c)
	out := c
	return &out, nil
}

// DeleteSecret implements webtool.Store.
func (f *WebToolStore) DeleteSecret(ctx context.Context, id uuid.UUID) error {
	org, err := f.org(ctx)
	if err != nil {
		return err
	}
	for i, s := range f.HTTPSecrets {
		if s.ID == id && s.OrgID == org {
			f.HTTPSecrets = slices.Delete(f.HTTPSecrets, i, i+1)
			return nil
		}
	}
	return &webtool.NotFoundError{}
}
