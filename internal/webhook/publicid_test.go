package webhook

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/outbox"
	"altalune.id/oagent/internal/kernel/tenant"
)

func TestServiceIDByPublicID(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	mine := &Endpoint{ID: uuid.New(), PublicID: "mine", OrgID: org, ProjectID: project}
	sibling := &Endpoint{ID: uuid.New(), PublicID: "sibling", OrgID: org, ProjectID: uuid.New()}
	foreign := &Endpoint{ID: uuid.New(), PublicID: "foreign", OrgID: uuid.New(), ProjectID: project}
	svc := testService(t, &fakeStore{endpoints: []*Endpoint{mine, sibling, foreign}}, &fakeOutbox{})
	ctx := tenantCtx(org, project)

	id, err := svc.IDByPublicID(ctx, "mine")
	require.NoError(t, err)
	assert.Equal(t, mine.ID, id)
	for _, other := range []string{"sibling", "foreign", "nope", mine.ID.String()} {
		_, err = svc.IDByPublicID(ctx, other)
		assert.True(t, IsNotFoundError(err), "%q is out of reach", other)
	}
	_, err = svc.IDByPublicID(context.Background(), "mine")
	assert.True(t, tenant.IsMissingError(err))

	broken := testService(t, &fakeStore{byIDErr: errors.New("down")}, &fakeOutbox{})
	_, err = broken.IDByPublicID(ctx, "mine")
	_, isApp := apperror.AsAppError(err)
	assert.True(t, isApp)
}

func TestServiceDeliveryIDByPublicID(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	ep := &Endpoint{ID: uuid.New(), PublicID: "ep", OrgID: org, ProjectID: project}
	other := &Endpoint{ID: uuid.New(), PublicID: "other", OrgID: org, ProjectID: project}
	entry := outbox.Entry{ID: uuid.New(), PublicID: "dlv", Target: ep.ID.String()}
	store := &fakeStore{endpoints: []*Endpoint{ep, other}}
	ctx := tenantCtx(org, project)

	svc := testService(t, store, &fakeOutbox{entries: []outbox.Entry{entry}})
	id, err := svc.DeliveryIDByPublicID(ctx, ep.ID, "dlv")
	require.NoError(t, err)
	assert.Equal(t, entry.ID, id)

	_, err = svc.DeliveryIDByPublicID(ctx, other.ID, "dlv")
	assert.True(t, IsDeliveryNotFoundError(err), "another endpoint's delivery is not found")
	_, err = svc.DeliveryIDByPublicID(ctx, ep.ID, entry.ID.String())
	assert.True(t, IsDeliveryNotFoundError(err), "a uuid is not a public id")
	_, err = svc.DeliveryIDByPublicID(ctx, uuid.New(), "dlv")
	assert.True(t, IsNotFoundError(err), "an unknown endpoint is not found")

	broken := testService(t, store, &fakeOutbox{listErr: errors.New("down")})
	_, err = broken.DeliveryIDByPublicID(ctx, ep.ID, "dlv")
	_, isApp := apperror.AsAppError(err)
	assert.True(t, isApp)
}
