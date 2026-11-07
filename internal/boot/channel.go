package boot

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/channel"
	"altalune.id/oagent/internal/enduser"
	"altalune.id/oagent/internal/ingest"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/web/layout"
)

func buildIngest(cfg config.Config, svcs *Services, uow db.Transactor, lims limiters, log *slog.Logger) http.Handler {
	if svcs == nil || svcs.Channels == nil || svcs.RunStore == nil || svcs.RunKicker == nil {
		return nil
	}
	in := cfg.RateLimit.Ingest
	openwa := channel.NewIngest(channel.IngestParams{
		Service: svcs.Channels, Store: svcs.ChannelStore, UOW: uow, Provider: svcs.ChannelProvider,
		EndUsers: svcs.EndUsers, Inbox: svcs.RunStore, Kicker: svcs.RunKicker,
		Limiter: lims.ingest, Rule: channel.IngestRule(in.PerConnectionPerMinute, in.Burst), Log: orDiscard(log),
	})
	return ingest.NewHandler(ingest.HandlerParams{
		BasePath:  layout.Path(cfg.HTTP.BasePath, ingest.DefaultBasePath),
		Providers: map[string]ingest.Provider{channel.ProviderOpenWA: openwa.Binding()},
		Limiter:   lims.ingest, IPRule: ingest.IPRule(in.PerIPPerMinute), TrustedProxyHops: cfg.HTTP.TrustedProxyHops,
		Log: orDiscard(log),
	})
}

type endUserNames struct{ users *enduser.Service }

func (n endUserNames) DisplayName(ctx context.Context, id uuid.UUID) (string, error) {
	e, err := n.users.Get(ctx, id)
	if err != nil {
		return "", err
	}
	return e.DisplayName, nil
}

type runPeople struct{ users *enduser.Service }

func (p runPeople) Person(ctx context.Context, id uuid.UUID) (run.Person, error) {
	n, err := p.users.Name(ctx, id)
	if err != nil {
		return run.Person{}, err
	}
	return run.Person{DisplayName: n.DisplayName, Kind: enduser.KindProto(n.Kind), Hint: n.Hint, Label: n.Label()}, nil
}

type endUserActivity struct{ store run.Store }

func (a endUserActivity) Seen(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]enduser.Seen, error) {
	got, err := a.store.EndUserActivity(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]enduser.Seen, len(got))
	for id, act := range got {
		out[id] = enduser.Seen{Last: act.LastSeen, Threads: act.Threads}
	}
	return out, nil
}

func (a endUserActivity) Threads(ctx context.Context, id uuid.UUID) ([]enduser.PersonThread, error) {
	got, err := a.store.EndUserThreads(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]enduser.PersonThread, len(got))
	for i, th := range got {
		out[i] = enduser.PersonThread{PublicID: th.PublicID, OrgID: th.OrgID, AgentID: th.AgentID, Kind: th.Kind, LastActivity: th.LastActivityAt}
	}
	return out, nil
}
