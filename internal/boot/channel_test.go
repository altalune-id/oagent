package boot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/channel"
	"altalune.id/oagent/internal/channel/openwa"
	"altalune.id/oagent/internal/enduser"
	"altalune.id/oagent/internal/kernel/ratelimit/memory"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/testutil/fakes"
)

type noKicks struct{}

func (noKicks) Kick(context.Context, run.Kick) error { return nil }

func TestBuildIngest_MountsOpenWABehindThePerIPLimit(t *testing.T) {
	assert.Nil(t, buildIngest(config.Config{}, &Services{}, nil, limiters{}, nil), "no channel service, no /hooks")
	svcs := &Services{
		Channels:        channel.NewService(&fakes.ChannelStore{}, fakes.Transactor{}, fakes.NewPublicIDs(), nil),
		ChannelStore:    &fakes.ChannelStore{},
		ChannelProvider: openwa.New(true),
		EndUsers:        enduser.NewService(&fakes.EndUserStore{}, fakes.Transactor{}, nil),
		RunStore:        &fakes.RunStore{},
		RunKicker:       noKicks{},
	}
	cfg := config.Config{HTTP: config.HTTPConfig{BasePath: "/app"}, RateLimit: config.RateLimitConfig{
		Ingest: config.IngestLimitConfig{Enabled: true, PerConnectionPerMinute: 120, Burst: 30, PerIPPerMinute: 1},
	}}
	mem := memory.New(100)
	h := buildIngest(cfg, svcs, fakes.Transactor{}, limiters{ingest: mem}, nil)
	require.NotNil(t, h)
	post := func(id string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/app/hooks/openwa/"+id, strings.NewReader("{}"))
		r.RemoteAddr = "203.0.113.7:1"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec
	}
	assert.Equal(t, http.StatusUnauthorized, post("unknwnunknwnunknwnabc").Code, "an unknown connection")
	assert.Equal(t, http.StatusTooManyRequests, post("othrothrothrothrothra").Code, "a second id from the same IP shares the bucket")
}

func TestEndUserNames_DisplayName(t *testing.T) {
	id := uuid.New()
	store := &fakes.EndUserStore{}
	require.NoError(t, store.Save(context.Background(), &enduser.EndUser{ID: id, DisplayName: "Budi"}))
	n := endUserNames{users: enduser.NewService(store, fakes.Transactor{}, nil)}
	name, err := n.DisplayName(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, "Budi", name)
}
