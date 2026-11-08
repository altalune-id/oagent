package boot

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/apikey"
	"altalune.id/oagent/internal/dataplane"
	"altalune.id/oagent/internal/enduser"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/user"
)

// NOTE: nil when the run domain was not built, so the server answers /api/v1/ as an unknown path.
func buildDataPlane(cfg config.Config, svcs *Services, lims limiters, unexpected apperror.UnexpectedFunc, log *slog.Logger) http.Handler {
	h := newDataPlane(cfg, svcs, lims, unexpected, log)
	if h == nil {
		return nil
	}
	mux := http.NewServeMux()
	h.Register(mux, cfg.HTTP.BasePath)
	return mux
}

func buildConsoleTry(
	cfg config.Config, svcs *Services, lims limiters, unexpected apperror.UnexpectedFunc, log *slog.Logger,
) map[string]http.Handler {
	h := newDataPlane(cfg, svcs, lims, unexpected, log)
	if h == nil {
		return nil
	}
	return h.TryRoutes()
}

func newDataPlane(
	cfg config.Config, svcs *Services, lims limiters, unexpected apperror.UnexpectedFunc, log *slog.Logger,
) *dataplane.Handler {
	if svcs == nil || svcs.Runs == nil || svcs.RunTurns == nil || svcs.KeyAuthn == nil {
		return nil
	}
	return dataplane.New(dataplane.Deps{
		Keys:      svcs.KeyAuthn,
		KeyPrefix: svcs.KeyScheme.Prefix(),
		Agents:    dataPlaneAgents{agents: svcs.Agents, store: svcs.AgentStore},
		Names:     dataPlaneNames{refs: newPublicIDs(svcs)},
		EndUsers:  dataPlaneEndUsers{users: svcs.EndUsers},
		Store:     svcs.RunStore,
		Runner:    svcs.RunTurns,
		Canceller: svcs.Runs,
		Drafts:    dataPlaneDrafts{agents: svcs.Agents},
		Limits: dataplane.Limits{
			Limiter: lims.dataPlane, TrustedProxyHops: cfg.HTTP.TrustedProxyHops,
			PerKey: dataplane.KeyRule(cfg.RateLimit.DataPlane.PerKeyPerMinute), PerIP: dataplane.IPRule(cfg.RateLimit.DataPlane.PerIPPerMinute),
		},
		Rate:         run.RateCap{Limiter: lims.shared, PerMinute: cfg.Run.MaxPerMinutePerProject},
		RuntimeReady: func() bool { return svcs.RuntimeReady },
		LeaseTTL:     cfg.Run.LeaseTTL,
		Unexpected:   unexpected,
		Log:          orDiscard(log),
	})
}

type dataPlaneDrafts struct{ agents *agent.Service }

func (d dataPlaneDrafts) Pin(ctx context.Context, agentID, by uuid.UUID) error {
	_, err := d.agents.DraftVersion(ctx, agentID, by)
	return err
}

type dataPlaneAgents struct {
	agents *agent.Service
	store  agent.Store
}

func (a dataPlaneAgents) Resolve(ctx context.Context, publicID string) (dataplane.AgentRef, bool, error) {
	r, err := a.agents.Resolve(ctx, publicID)
	if agent.IsNotFoundError(err) {
		return dataplane.AgentRef{}, false, nil
	}
	if err != nil {
		return dataplane.AgentRef{}, false, err
	}
	v, err := a.store.LatestVersion(tenant.WithOrg(ctx, r.OrgID), r.ID)
	if err != nil {
		return dataplane.AgentRef{}, false, err
	}
	return dataplane.AgentRef{ID: r.ID, OrgID: r.OrgID, ProjectID: r.ProjectID, Draft: v == nil}, true, nil
}

type dataPlaneNames struct{ refs publicIDs }

func (n dataPlaneNames) UserPublicID(ctx context.Context, userID uuid.UUID) (string, error) {
	return n.refs.UserPublicID(ctx, userID)
}

type dataPlaneEndUsers struct{ users *enduser.Service }

func (e dataPlaneEndUsers) UpsertAPIUser(ctx context.Context, projectID uuid.UUID, value string) (uuid.UUID, error) {
	u, err := e.users.Upsert(ctx, projectID, enduser.Identity{Kind: enduser.KindAPIUser, Value: value})
	if err != nil {
		return uuid.Nil, err
	}
	return u.ID, nil
}

func (e dataPlaneEndUsers) FindAPIUser(ctx context.Context, projectID uuid.UUID, value string) (uuid.UUID, bool, error) {
	u, err := e.users.Lookup(ctx, projectID, enduser.Identity{Kind: enduser.KindAPIUser, Value: value})
	if enduser.IsNotFoundError(err) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, err
	}
	return u.ID, true, nil
}

type memberNames struct{ users *user.Service }

func (m memberNames) MemberName(ctx context.Context, userPublicID string) (string, error) {
	u, err := m.users.ByPublicID(ctx, userPublicID)
	if user.IsNotFoundError(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return u.Name, nil
}

type keyHints struct{ keys apikey.Store }

func (k keyHints) KeyHint(ctx context.Context, keyID uuid.UUID) (string, error) {
	key, err := k.keys.ByID(ctx, keyID)
	if apikey.IsNotFoundError(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return key.SecretHint, nil
}
