package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/nanoid"
)

var uuidShaped = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

// Z12, Review Focus 1: a delivered request names the event and the delivery by public ids, with no UUID in its headers or body.
func TestDeliveryCarriesPublicIDsOnly(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	sl := testSealer(t)
	ep := sealedEndpoint(t, sl, org, project, "whsec_test")
	ep.PublicID = nanoid.MustNew(21)
	ep.EventTypes = []events.Type{events.WebhookPing}

	var header http.Header
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header, body = r.Header.Clone(), must(io.ReadAll(r.Body))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	ep.URL = srv.URL

	store := &fakeStore{endpoints: []*Endpoint{ep}}
	ob := &fakeOutbox{}
	require.NoError(t, testService(t, store, ob).SendTest(tenantCtx(org, project), ep.ID))
	require.Len(t, ob.enqueued, 1)
	entry := ob.enqueued[0]
	entry.Attempt = 1

	require.NoError(t, NewDeliverer(store, sl, srv.Client(), nil).Deliver(context.Background(), entry))
	var env struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(body, &env))
	assert.Equal(t, entry.EventPublicID, env.ID, "the envelope id is the event's public id, with no prefix")
	assert.Equal(t, entry.EventPublicID, header.Get(HeaderEventID))
	assert.Equal(t, entry.PublicID, header.Get(HeaderDeliveryID))
	assert.True(t, nanoid.IsValid(env.ID, 21), "got %q", env.ID)
	var all strings.Builder
	for name, values := range header {
		all.WriteString(name + ": " + strings.Join(values, ",") + "\n")
	}
	assert.Empty(t, uuidShaped.FindAllString(all.String(), -1), "headers: %s", all.String())
	assert.Empty(t, uuidShaped.FindAllString(string(body), -1), "body: %s", body)
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// Review Focus 1: a failed delivery's error becomes the delivery's last_error, which the API shows.
func TestInactiveEndpointErrorHoldsNoUUID(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	sl := testSealer(t)
	ep := sealedEndpoint(t, sl, org, project, "whsec_test")
	ep.PublicID, ep.Active = nanoid.MustNew(21), false
	d := NewDeliverer(&fakeStore{endpoints: []*Endpoint{ep}}, sl, http.DefaultClient, nil)

	err := d.Deliver(context.Background(), entryFor(ep, `{"type":"agent.created"}`))
	require.True(t, IsEndpointInactiveError(err))
	assert.Empty(t, uuidShaped.FindAllString(err.Error(), -1), err.Error())
	assert.Contains(t, err.Error(), ep.PublicID)
}
