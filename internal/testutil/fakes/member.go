package fakes

import (
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/org/member"
)

// MemberStore is an in-memory member.Store with one membership per org and user; Err, when set, fails every call.
type MemberStore struct {
	mu       sync.Mutex
	Err      error
	rows     []member.Membership
	contacts map[uuid.UUID]MemberContact
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

// AddIfAbsent implements member.Store.
func (f *MemberStore) AddIfAbsent(ctx context.Context, m *member.Membership) (bool, error) {
	err := f.Add(ctx, m)
	if member.IsAlreadyMemberError(err) {
		return false, nil
	}
	return err == nil, err
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

// MemberContact is the public id, email and name the fake joins into ListByOrg rows.
type MemberContact struct{ PublicID, Email, Name string }

// SetContact records the email and name ListByOrg returns for userID.
func (f *MemberStore) SetContact(userID uuid.UUID, c MemberContact) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.contacts == nil {
		f.contacts = map[uuid.UUID]MemberContact{}
	}
	f.contacts[userID] = c
}

// LockOwners implements member.Store; the fake takes no lock.
func (f *MemberStore) LockOwners(_ context.Context, orgID uuid.UUID) ([]uuid.UUID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	out := []uuid.UUID{}
	for _, r := range f.rows {
		if r.OrgID == orgID && r.Role == member.RoleOwner {
			out = append(out, r.UserID)
		}
	}
	slices.SortFunc(out, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	return out, nil
}

// LockMember implements member.Store; the fake takes no lock.
func (f *MemberStore) LockMember(ctx context.Context, orgID, userID uuid.UUID) (*member.Membership, error) {
	return f.Find(ctx, orgID, userID)
}

// UpdateRole implements member.Store.
func (f *MemberStore) UpdateRole(_ context.Context, orgID, userID uuid.UUID, role member.Role) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	for i := range f.rows {
		if f.rows[i].OrgID == orgID && f.rows[i].UserID == userID {
			f.rows[i].Role = role
			return nil
		}
	}
	return &member.NotFoundError{}
}

// Delete implements member.Store.
func (f *MemberStore) Delete(_ context.Context, orgID, userID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	for i, r := range f.rows {
		if r.OrgID == orgID && r.UserID == userID {
			f.rows = slices.Delete(f.rows, i, i+1)
			return nil
		}
	}
	return &member.NotFoundError{}
}

// ListByOrg implements member.Store, ordered owner, admin, member, then by email.
func (f *MemberStore) ListByOrg(_ context.Context, orgID uuid.UUID) ([]member.Member, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	out := []member.Member{}
	for _, r := range f.rows {
		if r.OrgID == orgID {
			c := f.contacts[r.UserID]
			out = append(out, member.Member{Membership: r, UserPublicID: c.PublicID, Email: c.Email, Name: c.Name})
		}
	}
	rank := map[member.Role]int{member.RoleOwner: 0, member.RoleAdmin: 1, member.RoleMember: 2}
	slices.SortStableFunc(out, func(a, b member.Member) int {
		if d := rank[a.Role] - rank[b.Role]; d != 0 {
			return d
		}
		return strings.Compare(a.Email, b.Email)
	})
	return out, nil
}
