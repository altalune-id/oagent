package fakes

import (
	"context"
	"slices"
	"sync"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/org/member"
)

// MemberStore is an in-memory member.Store with one membership per org and user; Err, when set, fails every call.
type MemberStore struct {
	mu   sync.Mutex
	Err  error
	rows []member.Membership
}

var _ member.Store = (*MemberStore)(nil)

// Add implements member.Store.
func (f *MemberStore) Add(_ context.Context, m *member.Membership) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	for _, r := range f.rows {
		if r.OrgID == m.OrgID && r.UserID == m.UserID {
			return &member.AlreadyMemberError{OrgID: m.OrgID, UserID: m.UserID}
		}
	}
	f.rows = append(f.rows, *m)
	return nil
}

// Find implements member.Store.
func (f *MemberStore) Find(_ context.Context, orgID, userID uuid.UUID) (*member.Membership, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	for _, r := range f.rows {
		if r.OrgID == orgID && r.UserID == userID {
			return &r, nil
		}
	}
	return nil, &member.NotFoundError{}
}

// ListByUser implements member.Store.
func (f *MemberStore) ListByUser(_ context.Context, userID uuid.UUID) ([]*member.Membership, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	out := []*member.Membership{}
	for _, r := range f.rows {
		if r.UserID == userID {
			out = append(out, &r)
		}
	}
	slices.SortStableFunc(out, func(a, b *member.Membership) int { return a.CreatedAt.Compare(b.CreatedAt) })
	return out, nil
}

// CountOwners implements member.Store.
func (f *MemberStore) CountOwners(_ context.Context, orgID uuid.UUID) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return 0, f.Err
	}
	n := 0
	for _, r := range f.rows {
		if r.OrgID == orgID && r.Role == member.RoleOwner {
			n++
		}
	}
	return n, nil
}
