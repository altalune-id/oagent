package webhook

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/kernel/outbox"
	"altalune.id/oagent/sealer"
)

func sealedEndpoint(t *testing.T, sl sealer.Sealer, org, project uuid.UUID, secret string) *Endpoint {
	t.Helper()
	e := &Endpoint{ID: uuid.New(), OrgID: org, ProjectID: project, Active: true, EventTypes: []events.Type{events.AgentCreated}}
	primary, err := sealSecret(sl, e.ID, slotPrimary, secret)
	require.NoError(t, err)
	e.Secrets = SealedSecrets{Primary: primary}
	return e
}

func entryFor(ep *Endpoint, payload string) outbox.Entry {
	return outbox.Entry{
		ID:        uuid.New(),
		EventID:   uuid.New(),
		OrgID:     ep.OrgID,
		ProjectID: ep.ProjectID,
		Target:    ep.ID.String(),
		Payload:   []byte(payload),
		Attempt:   1,
	}
}

func TestDelivererDeliver(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	sl := testSealer(t)
	ep := sealedEndpoint(t, sl, org, project, "whsec_test")

	var signature string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		signature = r.Header.Get(HeaderSignature)
		assert.Equal(t, Sign("whsec_test", r.Header.Get(HeaderTimestamp), body), signature)
		assert.Equal(t, string(events.AgentCreated), r.Header.Get(HeaderEventType))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	ep.URL = srv.URL

	store := &fakeStore{endpoints: []*Endpoint{ep}}
	d := NewDeliverer(store, sl, srv.Client(), nil)

	require.NoError(t, d.Deliver(context.Background(), entryFor(ep, `{"type":"agent.created"}`)))
	require.Len(t, store.attempts, 1)
	assert.Equal(t, http.StatusOK, store.attempts[0].StatusCode)
	assert.Contains(t, signature, "v1=")
}

func TestDelivererFailure(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	sl := testSealer(t)
	ep := sealedEndpoint(t, sl, org, project, "whsec_test")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) }))
	defer srv.Close()
	ep.URL = srv.URL

	store := &fakeStore{endpoints: []*Endpoint{ep}}
	d := NewDeliverer(store, sl, srv.Client(), nil)

	err := d.Deliver(context.Background(), entryFor(ep, `{"type":"agent.created"}`))
	assert.True(t, IsDeliveryFailedError(err))
	require.Len(t, store.attempts, 1)
	assert.Equal(t, http.StatusInternalServerError, store.attempts[0].StatusCode)
}

func TestDelivererInactive(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	sl := testSealer(t)
	ep := sealedEndpoint(t, sl, org, project, "whsec_test")
	ep.Active = false
	d := NewDeliverer(&fakeStore{endpoints: []*Endpoint{ep}}, sl, http.DefaultClient, nil)
	assert.True(t, IsEndpointInactiveError(d.Deliver(context.Background(), entryFor(ep, `{"type":"agent.created"}`))))
}

func TestDelivererEndpointDeleted(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	sl := testSealer(t)
	ep := sealedEndpoint(t, sl, org, project, "whsec_test")
	store := &fakeStore{byIDErr: &NotFoundError{ID: ep.ID.String()}}
	d := NewDeliverer(store, sl, http.DefaultClient, nil)
	assert.NoError(t, d.Deliver(context.Background(), entryFor(ep, `{"type":"agent.created"}`)))
}

func TestDelivererScopeMismatch(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	sl := testSealer(t)
	ep := sealedEndpoint(t, sl, org, project, "whsec_test")
	d := NewDeliverer(&fakeStore{endpoints: []*Endpoint{ep}}, sl, http.DefaultClient, nil)

	e := entryFor(ep, `{"type":"agent.created"}`)
	e.OrgID = uuid.New()
	assert.NoError(t, d.Deliver(context.Background(), e))
}

func TestDelivererBadTarget(t *testing.T) {
	sl := testSealer(t)
	d := NewDeliverer(&fakeStore{}, sl, http.DefaultClient, nil)
	assert.Error(t, d.Deliver(context.Background(), outbox.Entry{Target: "not-a-uuid"}))
}

func TestDelivererSecretUnavailable(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	sl := testSealer(t)
	ep := sealedEndpoint(t, sl, org, project, "whsec_test")
	ep.Secrets.Primary = []byte("junk")
	d := NewDeliverer(&fakeStore{endpoints: []*Endpoint{ep}}, sl, http.DefaultClient, nil)
	assert.True(t, IsSecretUnavailableError(d.Deliver(context.Background(), entryFor(ep, `{"type":"agent.created"}`))))
}

func TestDelivererBadEnvelope(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	sl := testSealer(t)
	ep := sealedEndpoint(t, sl, org, project, "whsec_test")
	d := NewDeliverer(&fakeStore{endpoints: []*Endpoint{ep}}, sl, http.DefaultClient, nil)
	assert.Error(t, d.Deliver(context.Background(), entryFor(ep, `{}`)))
}

func TestDelivererSecondarySecret(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	sl := testSealer(t)
	ep := sealedEndpoint(t, sl, org, project, "whsec_new")
	secondary, err := sealSecret(sl, ep.ID, slotSecondary, "whsec_old")
	require.NoError(t, err)
	ep.Secrets.Secondary = secondary

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer srv.Close()
	ep.URL = srv.URL

	var signatures string
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		signatures = r.Header.Get(HeaderSignature)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv2.Close()
	ep.URL = srv2.URL

	d := NewDeliverer(&fakeStore{endpoints: []*Endpoint{ep}}, sl, srv2.Client(), nil)
	require.NoError(t, d.Deliver(context.Background(), entryFor(ep, `{"type":"agent.created"}`)))
	assert.Contains(t, signatures, " ")
}

func TestRedactedError(t *testing.T) {
	assert.Contains(t, redactedError(&DeliveryFailedError{StatusCode: 500}), "status=500")
	assert.Equal(t, "boom", redactedError(errors.New("boom")))
}
