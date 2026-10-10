//go:build integration

package webhook_test

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/kernel/outbox"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/webhook"
)

func newAttempt(tc tenant.Context, endpointID, deliveryID uuid.UUID, n int, at time.Time) webhook.Attempt {
	return webhook.Attempt{
		ID:                uuid.Must(uuid.NewV7()),
		OrgID:             tc.OrgID,
		ProjectID:         tc.ProjectID,
		EndpointID:        endpointID,
		DeliveryID:        deliveryID,
		EventID:           uuid.New(),
		EventType:         events.AgentCreated,
		Attempt:           n,
		StatusCode:        500,
		Error:             "upstream said no",
		ResponseBody:      `{"ok":false}`,
		ResponseTruncated: false,
		ResponseHeaders:   []webhook.Header{{Name: "Content-Type", Value: "application/json"}, {Name: "X-Req", Value: "1"}},
		Duration:          1500 * time.Millisecond,
		CreatedAt:         at.Truncate(time.Microsecond),
	}
}

func (f *pgFixture) seedEndpoint(t *testing.T, tc tenant.Context) *webhook.Endpoint {
	t.Helper()
	e := newEndpoint(t, tc)
	require.NoError(t, f.store.Save(tenant.Into(t.Context(), tc), e))
	return e
}

func attemptIDs(as []webhook.Attempt) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(as))
	for _, a := range as {
		out = append(out, a.ID)
	}
	return out
}

func TestPostgresAttempt_RoundTrip(t *testing.T) {
	f := newPgFixture(t)
	tc := f.seedTenant(t)
	ctx := tenant.Into(t.Context(), tc)
	e := f.seedEndpoint(t, tc)

	cases := []struct {
		name   string
		mutate func(a *webhook.Attempt)
	}{
		{name: "failed with headers", mutate: func(*webhook.Attempt) {}},
		{name: "delivered with no headers", mutate: func(a *webhook.Attempt) {
			a.StatusCode = 204
			a.Error = ""
			a.ResponseBody = ""
			a.ResponseHeaders = []webhook.Header{}
			a.Duration = 0
		}},
		{name: "transport error and truncated body", mutate: func(a *webhook.Attempt) {
			a.StatusCode = 0
			a.Error = "dial tcp: connection refused"
			a.ResponseTruncated = true
			a.EventType = events.AgentUpdated
		}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			want := newAttempt(tc, e.ID, uuid.New(), 3, time.Date(2026, 5, 6, 7, 8, 9, 987654000, time.FixedZone("WITA", 8*3600)))
			tt.mutate(&want)
			require.NoError(t, f.store.SaveAttempt(ctx, want))

			got, err := f.store.ListAttempts(ctx, e.ID, want.DeliveryID)
			require.NoError(t, err)
			require.Len(t, got, 1)
			g := got[0]
			assert.True(t, want.CreatedAt.Equal(g.CreatedAt), "created_at want %v got %v", want.CreatedAt, g.CreatedAt)
			assert.Equal(t, time.UTC, g.CreatedAt.Location())
			want.CreatedAt = g.CreatedAt
			assert.Equal(t, want, g)
		})
	}
}

func TestPostgresAttempt_SaveFillsDefaultsAndBounds(t *testing.T) {
	f := newPgFixture(t)
	tc := f.seedTenant(t)
	ctx := tenant.Into(t.Context(), tc)
	e := f.seedEndpoint(t, tc)

	a := newAttempt(tc, e.ID, uuid.New(), 1, time.Time{})
	a.ID = uuid.Nil
	a.Error = strings.Repeat("é", outbox.MaxCauseLen)
	a.ResponseBody = strings.Repeat("x", webhook.MaxResponseBodyBytes+10)
	a.ResponseHeaders = nil
	before := time.Now().Add(-time.Second)
	require.NoError(t, f.store.SaveAttempt(ctx, a))

	got, err := f.store.ListAttempts(ctx, e.ID, a.DeliveryID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	g := got[0]
	assert.NotEqual(t, uuid.Nil, g.ID, "a zero id is generated")
	assert.True(t, g.CreatedAt.After(before), "a zero created_at is stamped now")
	assert.LessOrEqual(t, len(g.Error), outbox.MaxCauseLen)
	assert.True(t, utf8.ValidString(g.Error), "the truncated error must stay valid UTF-8")
	assert.Len(t, g.ResponseBody, webhook.MaxResponseBodyBytes)
	assert.True(t, g.ResponseTruncated)
	assert.Empty(t, g.ResponseHeaders)
}

func TestPostgresAttempt_SaveRefusesScopeMismatch(t *testing.T) {
	f := newPgFixture(t)
	a := f.seedTenant(t)
	b := f.seedTenant(t)
	ctxA := tenant.Into(t.Context(), a)
	e := f.seedEndpoint(t, a)

	otherProject := a
	otherProject.ProjectID = f.seedProject(t, a.OrgID)

	cases := []struct {
		name  string
		scope tenant.Context
	}{
		{name: "another org", scope: b},
		{name: "another project", scope: otherProject},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			err := f.store.SaveAttempt(ctxA, newAttempt(tt.scope, e.ID, uuid.New(), 1, time.Now()))
			assert.True(t, webhook.IsInvalidAttemptError(err), "got %T: %v", err, err)
		})
	}
	assert.Zero(t, f.count(t, "SELECT count(*) FROM webhook_deliveries"))
	assert.Error(t, f.store.SaveAttempt(t.Context(), newAttempt(a, e.ID, uuid.New(), 1, time.Now())), "no tenant on ctx")
}

func TestPostgresAttempt_SaveForAMissingEndpointIsNotFound(t *testing.T) {
	f := newPgFixture(t)
	tc := f.seedTenant(t)
	ctx := tenant.Into(t.Context(), tc)

	t.Run("never existed", func(t *testing.T) {
		err := f.store.SaveAttempt(ctx, newAttempt(tc, uuid.New(), uuid.New(), 1, time.Now()))
		assert.True(t, webhook.IsNotFoundError(err), "the endpoint FK must surface as NotFoundError, got %T: %v", err, err)
	})

	t.Run("deleted", func(t *testing.T) {
		e := f.seedEndpoint(t, tc)
		require.NoError(t, f.store.Delete(ctx, e.ID))
		err := f.store.SaveAttempt(ctx, newAttempt(tc, e.ID, uuid.New(), 1, time.Now()))
		assert.True(t, webhook.IsNotFoundError(err), "got %T: %v", err, err)
	})
}

func TestPostgresAttempt_DuplicateIDIsRawError(t *testing.T) {
	f := newPgFixture(t)
	tc := f.seedTenant(t)
	ctx := tenant.Into(t.Context(), tc)
	e := f.seedEndpoint(t, tc)

	a := newAttempt(tc, e.ID, uuid.New(), 1, time.Now())
	require.NoError(t, f.store.SaveAttempt(ctx, a))
	err := f.store.SaveAttempt(ctx, a)
	require.Error(t, err)
	assert.Equal(t, "23505", pgCode(err), "attempts are append-only, got %v", err)
}

func TestPostgresAttempt_ListOrdersAndFilters(t *testing.T) {
	f := newPgFixture(t)
	a := f.seedTenant(t)
	b := f.seedTenant(t)
	ctxA := tenant.Into(t.Context(), a)
	e := f.seedEndpoint(t, a)
	sibling := f.seedEndpoint(t, a)
	delivery := uuid.New()

	base := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	first := newAttempt(a, e.ID, delivery, 1, base)
	tieA := newAttempt(a, e.ID, delivery, 2, base.Add(time.Minute))
	tieB := newAttempt(a, e.ID, delivery, 3, base.Add(time.Minute))
	last := newAttempt(a, e.ID, delivery, 4, base.Add(time.Hour))
	for _, at := range []webhook.Attempt{tieB, last, first, tieA} {
		require.NoError(t, f.store.SaveAttempt(ctxA, at))
	}
	require.NoError(t, f.store.SaveAttempt(ctxA, newAttempt(a, e.ID, uuid.New(), 1, base.Add(2*time.Hour))))
	require.NoError(t, f.store.SaveAttempt(ctxA, newAttempt(a, sibling.ID, delivery, 1, base.Add(2*time.Hour))))

	ties := []uuid.UUID{tieA.ID, tieB.ID}
	if ties[0].String() < ties[1].String() {
		ties[0], ties[1] = ties[1], ties[0]
	}
	got, err := f.store.ListAttempts(ctxA, e.ID, delivery)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{last.ID, ties[0], ties[1], first.ID}, attemptIDs(got), "created_at DESC, then id DESC")

	t.Run("another org sees nothing", func(t *testing.T) {
		got, err := f.store.ListAttempts(tenant.Into(t.Context(), b), e.ID, delivery)
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("unknown delivery is empty, not an error", func(t *testing.T) {
		got, err := f.store.ListAttempts(ctxA, e.ID, uuid.New())
		require.NoError(t, err)
		assert.NotNil(t, got)
		assert.Empty(t, got)
	})

	t.Run("no tenant", func(t *testing.T) {
		_, err := f.store.ListAttempts(t.Context(), e.ID, delivery)
		assert.Error(t, err)
	})
}

func TestPostgresAttempt_DeleteEndpointCascades(t *testing.T) {
	f := newPgFixture(t)
	tc := f.seedTenant(t)
	ctx := tenant.Into(t.Context(), tc)
	doomed := f.seedEndpoint(t, tc)
	kept := f.seedEndpoint(t, tc)
	delivery := uuid.New()

	for i := 1; i <= 3; i++ {
		require.NoError(t, f.store.SaveAttempt(ctx, newAttempt(tc, doomed.ID, delivery, i, time.Now())))
	}
	require.NoError(t, f.store.SaveAttempt(ctx, newAttempt(tc, kept.ID, delivery, 1, time.Now())))

	require.NoError(t, f.store.Delete(ctx, doomed.ID))

	assert.Zero(t, f.count(t, "SELECT count(*) FROM webhook_deliveries WHERE endpoint_id = $1", doomed.ID))
	got, err := f.store.ListAttempts(ctx, kept.ID, delivery)
	require.NoError(t, err)
	assert.Len(t, got, 1, "another endpoint's attempts survive")
}
