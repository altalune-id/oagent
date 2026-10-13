package invite_test

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/org/invite"
	"altalune.id/oagent/mailer"
)

type memStore struct {
	mu     sync.Mutex
	rows   map[uuid.UUID]invite.Invite
	Err    error
	Reads  int
	Writes int
}

func newMemStore() *memStore { return &memStore{rows: map[uuid.UUID]invite.Invite{}} }

func (m *memStore) find(match func(*invite.Invite) bool) (*invite.Invite, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Reads++
	if m.Err != nil {
		return nil, m.Err
	}
	for _, r := range m.rows {
		if match(&r) {
			c := r
			return &c, nil
		}
	}
	return nil, &invite.NotFoundError{}
}

func (m *memStore) LockEmail(context.Context, uuid.UUID, string) error { return m.Err }

func (m *memStore) Live(_ context.Context, orgID uuid.UUID, email string) (*invite.Invite, error) {
	return m.find(func(i *invite.Invite) bool {
		return i.OrgID == orgID && i.Email == email && i.AcceptedAt == nil && i.RevokedAt == nil
	})
}

func (m *memStore) Lock(ctx context.Context, id uuid.UUID) (*invite.Invite, error) {
	return m.ByID(ctx, id)
}

func (m *memStore) ByID(_ context.Context, id uuid.UUID) (*invite.Invite, error) {
	return m.find(func(i *invite.Invite) bool { return i.ID == id })
}

func (m *memStore) ByTokenHash(_ context.Context, hash string) (*invite.Invite, error) {
	return m.find(func(i *invite.Invite) bool { return i.TokenHash != "" && i.TokenHash == hash })
}

func (m *memStore) ByPublicID(_ context.Context, publicID string) (*invite.Invite, error) {
	return m.find(func(i *invite.Invite) bool { return i.PublicID == publicID })
}

func (m *memStore) write(fn func() error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Writes++
	if m.Err != nil {
		return m.Err
	}
	return fn()
}

func (m *memStore) Insert(_ context.Context, i *invite.Invite) error {
	return m.write(func() error { m.rows[i.ID] = *i; return nil })
}

func (m *memStore) Update(_ context.Context, i *invite.Invite) error {
	return m.write(func() error {
		if _, ok := m.rows[i.ID]; !ok {
			return &invite.NotFoundError{}
		}
		m.rows[i.ID] = *i
		return nil
	})
}

func (m *memStore) MarkAccepted(_ context.Context, id, userID uuid.UUID, now time.Time) (*invite.Invite, error) {
	var out *invite.Invite
	err := m.write(func() error {
		r, ok := m.rows[id]
		if !ok || r.AcceptedAt != nil || r.RevokedAt != nil || r.Draft || !now.Before(r.ExpiresAt) {
			return &invite.NotFoundError{}
		}
		r.AcceptedAt, r.AcceptedBy = &now, userID
		m.rows[id] = r
		out = &r
		return nil
	})
	return out, err
}

func (m *memStore) Revoke(_ context.Context, id uuid.UUID, now time.Time) error {
	return m.write(func() error {
		r, ok := m.rows[id]
		if !ok || r.AcceptedAt != nil || r.RevokedAt != nil {
			return &invite.NotFoundError{}
		}
		r.RevokedAt = &now
		m.rows[id] = r
		return nil
	})
}

func (m *memStore) ListLive(_ context.Context, orgID uuid.UUID) ([]*invite.Invite, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Reads++
	if m.Err != nil {
		return nil, m.Err
	}
	var out []*invite.Invite
	for _, r := range m.rows {
		if r.OrgID == orgID && r.AcceptedAt == nil && r.RevokedAt == nil {
			c := r
			out = append(out, &c)
		}
	}
	return out, nil
}

type mailBox struct {
	mu   sync.Mutex
	sent []mailer.Message
	err  error
}

func (b *mailBox) Enqueue(_ context.Context, m mailer.Message) (uuid.UUID, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.err != nil {
		return uuid.Nil, b.err
	}
	b.sent = append(b.sent, m)
	return uuid.New(), nil
}

func (b *mailBox) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.sent)
}

type orgNames map[uuid.UUID]string

func (o orgNames) Name(_ context.Context, orgID uuid.UUID) (string, error) {
	if n, ok := o[orgID]; ok {
		return n, nil
	}
	return "", errors.New("unknown org")
}

type contact struct{ email, name string }

type people map[uuid.UUID]contact

func (p people) Contact(_ context.Context, userID uuid.UUID) (string, string, error) {
	if c, ok := p[userID]; ok {
		return c.email, c.name, nil
	}
	return "", "", errors.New("unknown person")
}

func publicIDOf(id uuid.UUID) string { return "pub-" + id.String()[:8] }

type published struct {
	org  uuid.UUID
	t    events.Type
	data any
}

type eventLog struct {
	mu   sync.Mutex
	sent []published
	err  error
}

func (l *eventLog) EnqueueOrg(_ context.Context, orgID uuid.UUID, t events.Type, data any) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.err != nil {
		return l.err
	}
	l.sent = append(l.sent, published{org: orgID, t: t, data: data})
	return nil
}

func (l *eventLog) all() []published {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]published(nil), l.sent...)
}
