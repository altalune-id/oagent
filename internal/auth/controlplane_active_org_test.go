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
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/user"
)

type switchingProfiles struct {
	recordedProfiles
	sid   string
	orgID uuid.UUID
}

func (s *switchingProfiles) SetActiveOrg(_ context.Context, sid string, p session.Principal, orgID uuid.UUID) (session.Principal, error) {
	if s.err != nil {
		return session.Principal{}, s.err
	}
	s.sid, s.orgID = sid, orgID
	p.ActiveOrgID = orgID
	return p, nil
}

func TestSetActiveOrgRPC(t *testing.T) {
	ada := &user.User{ID: uuid.New(), PublicID: "adaUserPublicId000001", Email: "ada@example.com"}
	acme := &org.Org{ID: uuid.New(), PublicID: "acmeOrgPublicId000001", Slug: "acme"}
	person := session.Principal{UserID: ada.ID, Email: ada.Email, Source: session.SourceLocal}
	plane := func(rec Profiles) *AuthService {
		return NewControlPlane(orgByID{acme.ID: acme}, projectByID{}, userByID{ada.ID: ada}).WithProfiles(rec)
	}
	call := func(s *AuthService, ctx context.Context, orgID string) (*authv1.SetActiveOrgResponse, error) {
		resp, err := s.SetActiveOrg(ctx, connect.NewRequest(&authv1.SetActiveOrgRequest{OrgId: orgID}))
		if err != nil {
			return nil, err
		}
		return resp.Msg, nil
	}

	t.Run("a person switches and gets the new whoami", func(t *testing.T) {
		rec := &switchingProfiles{}
		got, err := call(plane(rec), profileCtx(person, "sid-1"), acme.PublicID)
		require.NoError(t, err)
		assert.Equal(t, "sid-1", rec.sid)
		assert.Equal(t, acme.ID, rec.orgID)
		assert.Equal(t, acme.PublicID, got.GetMe().GetActiveOrgId())
		assert.Equal(t, "acme", got.GetMe().GetActiveOrgSlug())
		assert.Equal(t, ada.PublicID, got.GetMe().GetUserId())
	})
	t.Run("an unknown org is not found", func(t *testing.T) {
		rec := &switchingProfiles{}
		_, err := call(plane(rec), profileCtx(person, "sid-1"), "unknownOrgPublicId001")
		assert.True(t, org.IsNotFoundError(err), "%v", err)
		assert.Equal(t, uuid.Nil, rec.orgID, "nothing is switched")
	})
	t.Run("the service's refusal passes through", func(t *testing.T) {
		rec := &switchingProfiles{recordedProfiles: recordedProfiles{err: &org.NotFoundError{}}}
		_, err := call(plane(rec), profileCtx(person, "sid-1"), acme.PublicID)
		assert.True(t, org.IsNotFoundError(err), "%v", err)
	})
	t.Run("only a signed-in person with a session", func(t *testing.T) {
		s := plane(&switchingProfiles{})
		_, err := call(s, context.Background(), acme.PublicID)
		assert.True(t, authn.IsUnauthorizedError(err), "%v", err)
		_, err = call(s, profileCtx(session.Principal{Source: session.SourceAPIKey}, ""), acme.PublicID)
		assert.True(t, authz.IsForbiddenError(err), "%v", err)
		_, err = call(s, profileCtx(person, ""), acme.PublicID)
		assert.True(t, authn.IsUnauthorizedError(err), "a person without a session cookie cannot switch: %v", err)
	})
}
