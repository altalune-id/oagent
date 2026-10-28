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

// SetPasswordHash implements user.Store, changing only the password hash.
func (f *UserStore) SetPasswordHash(_ context.Context, id uuid.UUID, hash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	u, ok := f.users[id]
	if !ok {
		return &user.NotFoundError{}
	}
	u.PasswordHash, u.UpdatedAt = hash, time.Now().UTC()
	f.users[id] = u
	return nil
}

// ByIDP implements user.Store.
func (f *UserStore) ByIDP(_ context.Context, issuer, subject string) (*user.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	for _, u := range f.users {
		if u.IDPIssuer == issuer && u.IDPSubject == subject {
			return &u, nil
		}
	}
	return nil, &user.NotFoundError{}
}

// LinkIDP implements user.Store: once per user, and never an identity another user holds.
func (f *UserStore) LinkIDP(_ context.Context, id uuid.UUID, issuer, subject string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return false, f.Err
	}
	u, ok := f.users[id]
	if !ok || u.IDPIssuer != "" {
		return false, nil
	}
	for _, other := range f.users {
		if other.IDPIssuer == issuer && other.IDPSubject == subject {
			return false, nil
		}
	}
	u.IDPIssuer, u.IDPSubject = issuer, subject
	f.users[id] = u
	return true, nil
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
