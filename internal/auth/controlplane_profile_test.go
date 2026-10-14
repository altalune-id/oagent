package auth

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authv1 "altalune.id/oagent/gen/go/auth/v1"
	"altalune.id/oagent/internal/kernel/authn"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/user"
)

type recordedProfiles struct {
	user      *user.User
	err       error
	keepSID   string
	current   string
	next      string
	signedOut uuid.UUID
}

func (r *recordedProfiles) UpdateProfile(_ context.Context, userID uuid.UUID, name string) (*user.User, error) {
	if r.err != nil {
		return nil, r.err
	}
	u := *r.user
	u.ID, u.Name = userID, name
	return &u, nil
}

func (r *recordedProfiles) ChangePassword(_ context.Context, _ uuid.UUID, keepSID, current, next string) error {
	r.keepSID, r.current, r.next = keepSID, current, next
	return r.err
}

func (r *recordedProfiles) SignOutEverywhere(_ context.Context, userID uuid.UUID) error {
	r.signedOut = userID
	return r.err
}

func profileCtx(p session.Principal, sid string) context.Context {
	ctx := session.PrincipalInto(context.Background(), p)
	if sid != "" {
		ctx = session.SIDInto(ctx, sid)
	}
	return ctx
}

func TestProfileRPCs(t *testing.T) {
	ada := &user.User{ID: uuid.New(), PublicID: "adaUserPublicId000001", Email: "ada@example.com"}
	person := session.Principal{UserID: ada.ID, Email: ada.Email, Source: session.SourceLocal}
	rec := &recordedProfiles{user: ada}
	s := NewControlPlane(orgByID{}, projectByID{}, userByID{ada.ID: ada}).WithProfiles(rec)

	got, err := s.UpdateProfile(profileCtx(person, "sid-1"), connect.NewRequest(&authv1.UpdateProfileRequest{Name: "Ada Lovelace"}))
	require.NoError(t, err)
	assert.Equal(t, "adaUserPublicId000001", got.Msg.GetUserId(), "a public id, never the UUID")
	assert.NotContains(t, got.Msg.String(), ada.ID.String())
	assert.Equal(t, "Ada Lovelace", got.Msg.GetName())

	_, err = s.ChangePassword(profileCtx(person, "sid-1"), connect.NewRequest(&authv1.ChangePasswordRequest{
		CurrentPassword: "old one", NewPassword: "new one",
	}))
	require.NoError(t, err)
	assert.Equal(t, "sid-1", rec.keepSID, "this session is kept")
	assert.Equal(t, "old one", rec.current)
	assert.Equal(t, "new one", rec.next)

	out, err := s.SignOutEverywhere(profileCtx(person, "sid-1"), connect.NewRequest(&authv1.SignOutEverywhereRequest{}))
	require.NoError(t, err)
	assert.True(t, out.Msg.GetSignedOut())
	assert.Equal(t, ada.ID, rec.signedOut)
}

func TestProfileRPCs_PersonOnly(t *testing.T) {
	rec := &recordedProfiles{user: &user.User{}}
	s := NewControlPlane(orgByID{}, projectByID{}, userByID{}).WithProfiles(rec)
	for name, p := range map[string]session.Principal{
		"api key":      {Source: session.SourceAPIKey, KeyID: uuid.New()},
		"personal key": {Source: session.SourceAPIKey, KeyID: uuid.New(), OwnerID: uuid.New()},
		"basic":        {Source: session.SourceBasic, Name: "ops", Operator: true},
		"system":       session.System(),
	} {
		t.Run(name, func(t *testing.T) {
			ctx := profileCtx(p, "")
			_, err := s.UpdateProfile(ctx, connect.NewRequest(&authv1.UpdateProfileRequest{Name: "x"}))
			assert.True(t, authz.IsForbiddenError(err), "got %v", err)
			_, err = s.ChangePassword(ctx, connect.NewRequest(&authv1.ChangePasswordRequest{}))
			assert.True(t, authz.IsForbiddenError(err), "got %v", err)
			_, err = s.SignOutEverywhere(ctx, connect.NewRequest(&authv1.SignOutEverywhereRequest{}))
			assert.True(t, authz.IsForbiddenError(err), "got %v", err)
		})
	}
	_, err := s.UpdateProfile(context.Background(), connect.NewRequest(&authv1.UpdateProfileRequest{}))
	assert.True(t, authn.IsUnauthorizedError(err))
	assert.Equal(t, uuid.Nil, rec.signedOut)
}

func TestProfileRPCs_Errors(t *testing.T) {
	ada := &user.User{ID: uuid.New(), PublicID: "adaUserPublicId000001"}
	person := session.Principal{UserID: ada.ID, Source: session.SourceLocal}
	rec := &recordedProfiles{user: ada, err: &InvalidCredentialsError{}}
	s := NewControlPlane(orgByID{}, projectByID{}, userByID{}).WithProfiles(rec)
	_, err := s.UpdateProfile(profileCtx(person, ""), connect.NewRequest(&authv1.UpdateProfileRequest{}))
	assert.True(t, IsInvalidCredentialsError(err))
	_, err = s.ChangePassword(profileCtx(person, ""), connect.NewRequest(&authv1.ChangePasswordRequest{}))
	assert.True(t, IsInvalidCredentialsError(err))
	_, err = s.SignOutEverywhere(profileCtx(person, ""), connect.NewRequest(&authv1.SignOutEverywhereRequest{}))
	assert.True(t, IsInvalidCredentialsError(err))
}

func TestWhoami_ShowsTheStoredName(t *testing.T) {
	ada := &user.User{ID: uuid.New(), PublicID: "adaUserPublicId000001", Email: "ada@example.com", Name: "Ada Lovelace"}
	got, err := whoami(t, whoamiClient(t, orgByID{}, projectByID{}, userByID{ada.ID: ada}, session.Principal{
		UserID: ada.ID, Email: ada.Email, Name: "Ada", Source: session.SourceLocal,
	}))
	require.NoError(t, err)
	assert.Equal(t, "Ada Lovelace", got.GetName(), "a renamed person sees the new name without signing in again")
}
