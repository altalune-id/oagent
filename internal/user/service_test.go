package user_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/testutil/fakes"
	"altalune.id/oagent/internal/user"
	"altalune.id/oagent/password"
)

func newService(store user.Store) *user.Service {
	return user.NewService(store, apperror.NewReporter(nil, false).Unexpected)
}

func TestService_Create(t *testing.T) {
	store := fakes.NewUserStore()
	u, err := newService(store).Create(context.Background(), user.CreateRequest{Email: "Alice@Example.com", Name: "Alice", Password: "long-enough"})
	require.NoError(t, err)
	assert.Equal(t, "alice@example.com", u.Email)
	assert.NoError(t, password.Verify(u.PasswordHash, "long-enough"))
	assert.Equal(t, 1, store.Len())
}

func TestService_Create_Rejects(t *testing.T) {
	for name, tc := range map[string]struct {
		req   user.CreateRequest
		check func(error) bool
	}{
		"short password": {user.CreateRequest{Email: "a@example.com", Password: "short"}, password.IsPolicyError},
		"bad email":      {user.CreateRequest{Email: "nope", Password: "long-enough"}, user.IsInvalidEmailError},
	} {
		t.Run(name, func(t *testing.T) {
			store := fakes.NewUserStore()
			_, err := newService(store).Create(context.Background(), tc.req)
			assert.True(t, tc.check(err), "got %v", err)
			assert.Equal(t, 0, store.Len(), "nothing saved")
		})
	}
}

func TestService_Create_DuplicateEmail(t *testing.T) {
	svc := newService(fakes.NewUserStore())
	_, err := svc.Create(context.Background(), user.CreateRequest{Email: "a@example.com", Password: "long-enough"})
	require.NoError(t, err)
	_, err = svc.Create(context.Background(), user.CreateRequest{Email: "A@example.com", Password: "long-enough"})
	assert.True(t, user.IsAlreadyExistsError(err), "got %v", err)
}

func TestService_Lookups(t *testing.T) {
	svc := newService(fakes.NewUserStore())
	u, err := svc.Create(context.Background(), user.CreateRequest{Email: "alice@example.com", Password: "long-enough"})
	require.NoError(t, err)

	got, err := svc.ByEmail(context.Background(), "ALICE@example.com")
	require.NoError(t, err)
	assert.Equal(t, u.ID, got.ID)
	got, err = svc.ByID(context.Background(), u.ID)
	require.NoError(t, err)
	assert.Equal(t, u.Email, got.Email)

	_, err = svc.ByID(context.Background(), uuid.New())
	assert.True(t, user.IsNotFoundError(err))
	_, err = svc.ByEmail(context.Background(), "bob@example.com")
	assert.True(t, user.IsNotFoundError(err))
}

func TestService_SetPassword(t *testing.T) {
	store := fakes.NewUserStore()
	svc := newService(store)
	u, err := svc.Create(context.Background(), user.CreateRequest{Email: "a@example.com", Password: "long-enough"})
	require.NoError(t, err)

	require.NoError(t, svc.SetPassword(context.Background(), u.ID, "another-password"))
	got, err := svc.ByID(context.Background(), u.ID)
	require.NoError(t, err)
	assert.NoError(t, password.Verify(got.PasswordHash, "another-password"))
	assert.True(t, password.IsMismatchError(password.Verify(got.PasswordHash, "long-enough")))

	assert.True(t, password.IsPolicyError(svc.SetPassword(context.Background(), u.ID, "short")))
	assert.True(t, user.IsNotFoundError(svc.SetPassword(context.Background(), uuid.New(), "another-password")))
}

func TestService_StoreFailureIsReported(t *testing.T) {
	store := fakes.NewUserStore()
	store.Err = errors.New("db down")
	svc := newService(store)

	_, err := svc.Create(context.Background(), user.CreateRequest{Email: "a@example.com", Password: "long-enough"})
	ae, ok := errors.AsType[*apperror.AppError](err)
	require.True(t, ok, "got %v", err)
	assert.Equal(t, apperror.CodeUnexpected, ae.Code())

	_, err = svc.ByID(context.Background(), uuid.New())
	_, ok = errors.AsType[*apperror.AppError](err)
	assert.True(t, ok)
	_, err = svc.ByEmail(context.Background(), "a@example.com")
	_, ok = errors.AsType[*apperror.AppError](err)
	assert.True(t, ok)
}

func TestService_AcceptTerms(t *testing.T) {
	store := fakes.NewUserStore()
	svc := newService(store)
	u, err := svc.Create(context.Background(), user.CreateRequest{Email: "ada@example.com", Password: "long-enough"})
	require.NoError(t, err)

	got, err := svc.AcceptTerms(context.Background(), u.ID)
	require.NoError(t, err)
	require.NotNil(t, got.TermsAcceptedAt)
	stored, err := svc.ByID(context.Background(), u.ID)
	require.NoError(t, err)
	require.NotNil(t, stored.TermsAcceptedAt)

	_, err = svc.AcceptTerms(context.Background(), uuid.New())
	assert.True(t, user.IsNotFoundError(err))
}
