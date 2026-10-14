package fakes

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/user"
)

// UserStore is an in-memory user.Store keyed by id with email uniqueness; Err, when set, fails every call.
type UserStore struct {
	mu    sync.Mutex
	Err   error
	users map[uuid.UUID]user.User
}

var _ user.Store = (*UserStore)(nil)

// NewUserStore returns an empty UserStore.
func NewUserStore() *UserStore { return &UserStore{users: map[uuid.UUID]user.User{}} }

// Save implements user.Store.
func (f *UserStore) Save(_ context.Context, u *user.User) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	for id, other := range f.users {
		if id != u.ID && other.Email == u.Email {
			return &user.AlreadyExistsError{Email: u.Email}
		}
	}
	f.users[u.ID] = *u
	return nil
}

// AcceptTerms implements user.Store with the process clock.
func (f *UserStore) AcceptTerms(_ context.Context, id uuid.UUID) (*user.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	u, ok := f.users[id]
	if !ok {
		return nil, &user.NotFoundError{}
	}
	now := time.Now().UTC()
	u.TermsAcceptedAt = &now
	f.users[id] = u
	return &u, nil
}

// SetName implements user.Store, changing only the name.
func (f *UserStore) SetName(_ context.Context, id uuid.UUID, name string) (*user.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	u, ok := f.users[id]
	if !ok {
		return nil, &user.NotFoundError{}
	}
	u.Name, u.UpdatedAt = name, time.Now().UTC()
	f.users[id] = u
	return &u, nil
}

// ByID implements user.Store.
func (f *UserStore) ByID(_ context.Context, id uuid.UUID) (*user.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	u, ok := f.users[id]
	if !ok {
		return nil, &user.NotFoundError{}
	}
	return &u, nil
}

// ByPublicID implements user.Store.
func (f *UserStore) ByPublicID(_ context.Context, publicID string) (*user.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	for _, u := range f.users {
		if u.PublicID == publicID {
			return &u, nil
		}
	}
	return nil, &user.NotFoundError{}
}

// ByEmail implements user.Store.
func (f *UserStore) ByEmail(_ context.Context, email string) (*user.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	e, err := user.NormalizeEmail(email)
	if err != nil {
		return nil, &user.NotFoundError{}
	}
	for _, u := range f.users {
		if u.Email == e {
			return &u, nil
		}
	}
	return nil, &user.NotFoundError{}
}

// Len reports how many users are stored.
func (f *UserStore) Len() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.users)
}
