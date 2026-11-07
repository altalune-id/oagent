package fakes

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/channel"
)

// ChannelStore is an in-memory channel.Store; Err fails every call.
type ChannelStore struct {
	Err         error
	Connections map[uuid.UUID]*channel.Connection
}

var _ channel.Store = (*ChannelStore)(nil)

// Resolve implements channel.Store.
func (f *ChannelStore) Resolve(_ context.Context, publicID string) (channel.Ref, error) {
	if f.Err != nil {
		return channel.Ref{}, f.Err
	}
	for _, c := range f.Connections {
		if c.PublicID == publicID {
			return channel.Ref{
				ID: c.ID, OrgID: c.OrgID, ProjectID: c.ProjectID, AgentID: c.AgentID, Provider: c.Provider, DeviceID: c.DeviceID, Status: c.Status,
				SecretSet: len(c.SigningPrimarySealed) > 0,
			}, nil
		}
	}
	return channel.Ref{}, &channel.NotFoundError{}
}

// ByID implements channel.Store.
func (f *ChannelStore) ByID(_ context.Context, id uuid.UUID) (*channel.Connection, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	c, ok := f.Connections[id]
	if !ok {
		return nil, &channel.NotFoundError{}
	}
	out := *c
	return &out, nil
}

// List implements channel.Store; it returns the project's connections oldest first.
func (f *ChannelStore) List(_ context.Context, projectID uuid.UUID) ([]*channel.Connection, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	out := []*channel.Connection{}
	for _, c := range f.Connections {
		if c.ProjectID == projectID {
			cp := *c
			out = append(out, &cp)
		}
	}
	slices.SortFunc(out, func(a, b *channel.Connection) int {
		return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), slices.Compare(a.ID[:], b.ID[:]))
	})
	return out, nil
}

// Insert implements channel.Store.
func (f *ChannelStore) Insert(_ context.Context, c *channel.Connection) error {
	if f.Err != nil {
		return f.Err
	}
	if f.Connections == nil {
		f.Connections = map[uuid.UUID]*channel.Connection{}
	}
	saved := *c
	f.Connections[c.ID] = &saved
	return nil
}

// Update implements channel.Store.
func (f *ChannelStore) Update(_ context.Context, c *channel.Connection) error {
	if f.Err != nil {
		return f.Err
	}
	if _, ok := f.Connections[c.ID]; !ok {
		return &channel.NotFoundError{}
	}
	saved := *c
	f.Connections[c.ID] = &saved
	return nil
}

// Delete implements channel.Store.
func (f *ChannelStore) Delete(_ context.Context, id uuid.UUID) error {
	if f.Err != nil {
		return f.Err
	}
	if _, ok := f.Connections[id]; !ok {
		return &channel.NotFoundError{}
	}
	delete(f.Connections, id)
	return nil
}

// InsertDelivery implements channel.Store; it always inserts.
func (f *ChannelStore) InsertDelivery(context.Context, *channel.Connection, string, time.Time) (bool, error) {
	return f.Err == nil, f.Err
}

// RecordResult implements channel.Store.
func (f *ChannelStore) RecordResult(_ context.Context, id uuid.UUID, result string, at time.Time) error {
	if f.Err != nil {
		return f.Err
	}
	if c, ok := f.Connections[id]; ok && (c.LastResult != result || c.LastResultAt == nil || at.Sub(*c.LastResultAt) >= time.Minute) {
		c.LastResult, c.LastResultAt = result, &at
	}
	return nil
}

// RecordBadSignature implements channel.Store.
func (f *ChannelStore) RecordBadSignature(_ context.Context, id uuid.UUID, at time.Time) error {
	if f.Err != nil {
		return f.Err
	}
	if c, ok := f.Connections[id]; ok {
		c.LastBadSignatureAt = &at
	}
	return nil
}

// PruneDeliveries implements channel.Store; deliveries are not modeled.
func (f *ChannelStore) PruneDeliveries(context.Context, time.Time) error { return f.Err }
