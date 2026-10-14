package user_test

import (
	"context"
	"errors"
	"strings"
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
	u, err := newService(store).Create(context.Background(), user.CreateRequest{Email: "Alice@Example.com", Name: "Alice", Password: "correct horse battery"})
	require.NoError(t, err)
	assert.Equal(t, "alice@example.com", u.Email)
	assert.NoError(t, password.Verify(u.PasswordHash, "correct horse battery"))
	assert.Equal(t, 1, store.Len())
}

func policyReason(t *testing.T, err error) password.Reason {
	t.Helper()
	pe, ok := errors.AsType[*password.PolicyError](err)
	require.True(t, ok, "got %v", err)
	return pe.Reason
}

func TestService_Create_PasswordRule(t *testing.T) {
	svc := newService(fakes.NewUserStore())
	_, err := svc.Create(context.Background(), user.CreateRequest{Email: "a@example.com", Password: "tea-for-two!"})
	require.NoError(t, err, "12 characters is enough")
	_, err = svc.Create(context.Background(), user.CreateRequest{Email: "b@example.com", Password: "tea-for-two"})
	assert.Equal(t, password.ReasonTooShort, policyReason(t, err))
	_, err = svc.Create(context.Background(), user.CreateRequest{Email: "Alice@Example.com", Password: "alice-is-great-2026"})
	assert.Equal(t, password.ReasonContainsEmail, policyReason(t, err))
	_, err = svc.Create(context.Background(), user.CreateRequest{Email: "c@example.com", Name: "Ana Maria", Password: "maria-loves-tea"})
	assert.Equal(t, password.ReasonContainsName, policyReason(t, err))
}

func TestService_SetPassword_UsesTheUserAsHints(t *testing.T) {
	store := fakes.NewUserStore()
	svc := newService(store)
	u, err := svc.Create(context.Background(), user.CreateRequest{Email: "alice@example.com", Name: "Ana Maria", Password: "correct horse battery"})
	require.NoError(t, err)

	assert.Equal(t, password.ReasonContainsEmail, policyReason(t, svc.SetPassword(context.Background(), u.ID, "alice-is-great-2026")))
	assert.Equal(t, password.ReasonContainsName, policyReason(t, svc.SetPassword(context.Background(), u.ID, "maria-loves-tea")))
	assert.Equal(t, password.ReasonTooShort, policyReason(t, svc.SetPassword(context.Background(), u.ID, "tea-for-two")))
	got, err := svc.ByID(context.Background(), u.ID)
	require.NoError(t, err)
	assert.NoError(t, password.Verify(got.PasswordHash, "correct horse battery"), "a rejected password changes nothing")
}

func TestService_Create_Rejects(t *testing.T) {
	for name, tc := range map[string]struct {
		req   user.CreateRequest
		check func(error) bool
	}{
		"short password": {user.CreateRequest{Email: "a@example.com", Password: "short"}, password.IsPolicyError},
		"bad email":      {user.CreateRequest{Email: "nope", Password: "correct horse battery"}, user.IsInvalidEmailError},
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
	_, err := svc.Create(context.Background(), user.CreateRequest{Email: "a@example.com", Password: "correct horse battery"})
	require.NoError(t, err)
	_, err = svc.Create(context.Background(), user.CreateRequest{Email: "A@example.com", Password: "correct horse battery"})
	assert.True(t, user.IsAlreadyExistsError(err), "got %v", err)
}

func TestService_Lookups(t *testing.T) {
	svc := newService(fakes.NewUserStore())
	u, err := svc.Create(context.Background(), user.CreateRequest{Email: "alice@example.com", Password: "correct horse battery"})
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

func TestService_FindByEmail(t *testing.T) {
	svc := newService(fakes.NewUserStore())
	u, err := svc.Create(context.Background(), user.CreateRequest{Email: "alice@example.com", Password: "correct horse battery"})
	require.NoError(t, err)

	got, err := svc.FindByEmail(context.Background(), "  Alice@Example.com ")
	require.NoError(t, err)
	assert.Equal(t, u.ID, got.ID)

	_, err = svc.FindByEmail(context.Background(), "Bob@example.com")
	ue, ok := errors.AsType[*user.UnknownEmailError](err)
	require.True(t, ok, "got %v", err)
	assert.Equal(t, "bob@example.com", ue.Email, "the envelope names the normalized address")
	assert.True(t, user.IsNotFoundError(err))

	_, err = svc.FindByEmail(context.Background(), "not an email")
	assert.True(t, user.IsInvalidEmailError(err), "got %v", err)
}

func TestService_SetPassword(t *testing.T) {
	store := fakes.NewUserStore()
	svc := newService(store)
	u, err := svc.Create(context.Background(), user.CreateRequest{Email: "a@example.com", Password: "correct horse battery"})
	require.NoError(t, err)

	require.NoError(t, svc.SetPassword(context.Background(), u.ID, "another-password"))
	got, err := svc.ByID(context.Background(), u.ID)
	require.NoError(t, err)
	assert.NoError(t, password.Verify(got.PasswordHash, "another-password"))
	assert.True(t, password.IsMismatchError(password.Verify(got.PasswordHash, "correct horse battery")))

	assert.True(t, password.IsPolicyError(svc.SetPassword(context.Background(), u.ID, "short")))
	assert.True(t, user.IsNotFoundError(svc.SetPassword(context.Background(), uuid.New(), "another-password")))
}

func TestService_StoreFailureIsReported(t *testing.T) {
	store := fakes.NewUserStore()
	store.Err = errors.New("db down")
	svc := newService(store)

	_, err := svc.Create(context.Background(), user.CreateRequest{Email: "a@example.com", Password: "correct horse battery"})
	ae, ok := errors.AsType[*apperror.AppError](err)
	require.True(t, ok, "got %v", err)
	assert.Equal(t, apperror.CodeUnexpected, ae.Code())

	_, err = svc.ByID(context.Background(), uuid.New())
	_, ok = errors.AsType[*apperror.AppError](err)
	assert.True(t, ok)
	_, err = svc.ByEmail(context.Background(), "a@example.com")
	_, ok = errors.AsType[*apperror.AppError](err)
	assert.True(t, ok)
	_, err = svc.FindByEmail(context.Background(), "a@example.com")
	assert.False(t, user.IsUnknownEmailError(err), "a store failure is not an unknown email")
}

func TestService_AcceptTerms(t *testing.T) {
	store := fakes.NewUserStore()
	svc := newService(store)
	u, err := svc.Create(context.Background(), user.CreateRequest{Email: "ada@example.com", Password: "correct horse battery"})
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

func TestService_ByPublicID(t *testing.T) {
	store := fakes.NewUserStore()
	u, err := user.New("pub@example.com", "Pub")
	require.NoError(t, err)
	require.NoError(t, store.Save(context.Background(), u))

	got, err := newService(store).ByPublicID(context.Background(), u.PublicID)
	require.NoError(t, err)
	assert.Equal(t, u.ID, got.ID)
	_, err = newService(store).ByPublicID(context.Background(), "nope")
	assert.True(t, user.IsNotFoundError(err))
	store.Err = errors.New("down")
	_, err = newService(store).ByPublicID(context.Background(), u.PublicID)
	_, isApp := apperror.AsAppError(err)
	assert.True(t, isApp)
}

func TestService_SetName(t *testing.T) {
	svc := newService(fakes.NewUserStore())
	ctx := context.Background()
	u, err := svc.Create(ctx, user.CreateRequest{Email: "alice@example.com", Name: "Alice", Password: "correct horse battery"})
	require.NoError(t, err)

	got, err := svc.SetName(ctx, u.ID, "  Ana Maria  ")
	require.NoError(t, err)
	assert.Equal(t, "Ana Maria", got.Name)
	stored, err := svc.ByID(ctx, u.ID)
	require.NoError(t, err)
	assert.Equal(t, "Ana Maria", stored.Name)
	assert.NoError(t, password.Verify(stored.PasswordHash, "correct horse battery"), "the password is kept")

	got, err = svc.SetName(ctx, u.ID, "")
	require.NoError(t, err)
	assert.Empty(t, got.Name, "an empty name is allowed")

	_, err = svc.SetName(ctx, u.ID, strings.Repeat("a", 201))
	assert.True(t, user.IsInvalidNameError(err))
	_, err = svc.SetName(ctx, u.ID, strings.Repeat("a", 200))
	assert.NoError(t, err)

	_, err = svc.SetName(ctx, uuid.New(), "Bob")
	assert.True(t, user.IsNotFoundError(err))
}

type staleReads struct {
	*fakes.UserStore
	stale *user.User
}

func (s staleReads) ByID(context.Context, uuid.UUID) (*user.User, error) {
	c := *s.stale
	return &c, nil
}

func TestService_SetNameNeverWritesAStalePassword(t *testing.T) {
	store := fakes.NewUserStore()
	ctx := context.Background()
	u, err := newService(store).Create(ctx, user.CreateRequest{Email: "ana@example.com", Name: "Ana", Password: "the old passphrase"})
	require.NoError(t, err)
	before := *u
	require.NoError(t, newService(store).SetPassword(ctx, u.ID, "a brand new passphrase"))

	_, err = newService(staleReads{UserStore: store, stale: &before}).SetName(ctx, u.ID, "Ana Maria")
	require.NoError(t, err)
	got, err := store.ByID(ctx, u.ID)
	require.NoError(t, err)
	assert.Equal(t, "Ana Maria", got.Name)
	assert.NoError(t, password.Verify(got.PasswordHash, "a brand new passphrase"), "a rename that read the user before a reset keeps the new password")
}
