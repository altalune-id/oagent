//go:build integration

package boot

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/runtime"
	rthttp "altalune.id/oagent/internal/kernel/runtime/http"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/mcpserver"
	"altalune.id/oagent/internal/modelprovider"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/run/snapshotstore"
	"altalune.id/oagent/internal/testutil/fakes"
	"altalune.id/oagent/internal/testutil/pgtest"
	"altalune.id/oagent/internal/testutil/runtimetest"
)

type runDriver struct {
	t       *testing.T
	pool    db.Pool
	admin   *sql.DB
	schema  string
	store   run.Store
	svc     *run.Service
	orgs    org.Store
	prjs    project.Store
	agents  agent.Store
	models  *modelprovider.Service
	servers *mcpserver.Service
	org     uuid.UUID
}

func newRunDriver(t *testing.T, env runtimetest.Env) runtimetest.Driver {
	t.Helper()
	pool, admin, schema := pgtest.MigratedRLS(t)
	unexpected := apperror.NewReporter(nil, false).Unexpected
	uow := db.NewTransactor(pool)
	sl := testSealer(t)
	d := &runDriver{
		t: t, pool: pool, admin: admin, schema: schema,
		store:  run.NewStore(pool, schema, ""),
		orgs:   org.NewStore(pool, schema, ""),
		prjs:   project.NewStore(pool, schema, ""),
		agents: agent.NewStore(pool, schema, ""),
		models: modelprovider.NewService(modelprovider.NewStore(pool, schema, ""), uow, unexpected).WithSealer(sl).WithAllowInsecureURL(),
	}
	servers := mcpserver.NewService(mcpserver.NewStore(pool, schema, ""), uow, unexpected).WithSealer(sl).WithAllowInsecureURL()
	d.servers = servers
	rt, err := rthttp.New(runtime.Config{Driver: runtime.DriverHTTP, URL: env.RuntimeURL, Token: env.RuntimeToken})
	require.NoError(t, err)
	d.svc = run.NewService(d.store, uow, unexpected).
		WithRuntime(rt).
		WithAgents(runAgents{store: d.agents}).
		WithProviders(runProviders{providers: d.models}).
		WithMCPServers(runServers{servers: servers}).
		WithSnapshots(snapshotstore.NewPostgres(pool, schema, "")).
		WithPublisher(&fakes.Publisher{}).
		WithTurnTimeout(2 * time.Minute)
	return d
}

func (d *runDriver) ctx(ctx context.Context) context.Context {
	return tenant.Into(ctx, tenant.Context{OrgID: d.org})
}

func (d *runDriver) Seed(ctx context.Context, env runtimetest.Env, text string) (runtimetest.Seeded, error) {
	o, err := org.NewOrg("org-"+uuid.NewString()[:8], "Org", uuid.Nil)
	if err != nil {
		return runtimetest.Seeded{}, err
	}
	d.org = o.ID
	ctx = d.ctx(ctx)
	if err := d.orgs.Save(ctx, o); err != nil {
		return runtimetest.Seeded{}, err
	}
	p, err := project.NewProject(o.ID, "prj", "Project", uuid.Nil)
	if err != nil {
		return runtimetest.Seeded{}, err
	}
	if err := d.prjs.Save(ctx, p); err != nil {
		return runtimetest.Seeded{}, err
	}
	provider, err := d.models.Create(ctx, p.ID, modelprovider.Input{
		Name: "fake", Kind: modelprovider.KindOpenAICompatible, BaseURL: env.ModelBaseURL,
		Models: []modelprovider.Model{{ID: env.ModelID}},
	}, env.ModelKey)
	if err != nil {
		return runtimetest.Seeded{}, err
	}
	a, err := agent.New(o.ID, p.ID, "bot", "Bot", "", uuid.Nil)
	if err != nil {
		return runtimetest.Seeded{}, err
	}
	if err := d.agents.Save(ctx, a); err != nil {
		return runtimetest.Seeded{}, err
	}
	cfg := agent.Config{Instructions: "Be brief.", Model: env.ModelID, ProviderID: provider.ID}
	if env.MCPURL != "" {
		srv, err := d.servers.Create(ctx, p.ID, mcpserver.Input{
			Name: env.MCPServerName, URL: env.MCPURL, AuthKind: mcpserver.AuthNone, TimeoutMS: 10_000,
		}, "")
		if err != nil {
			return runtimetest.Seeded{}, err
		}
		cfg.MCPServers = []agent.MCPGrant{{ServerID: srv.ID, Tools: []agent.Tool{{Name: env.MCPToolName}}}}
	}
	if err := a.SetDraft(cfg, time.Now()); err != nil {
		return runtimetest.Seeded{}, err
	}
	v, _, err := a.Publish(nil, uuid.Nil, time.Now().UTC())
	if err != nil {
		return runtimetest.Seeded{}, err
	}
	if err := errors.Join(d.agents.InsertVersion(ctx, v), d.agents.MarkPublished(ctx, a), d.agents.SaveDraft(ctx, a)); err != nil {
		return runtimetest.Seeded{}, err
	}
	thread := uuid.New()
	if _, err := d.admin.ExecContext(ctx, `INSERT INTO `+d.schema+`.threads (id, public_id, org_id, project_id, agent_id, kind, external_key, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, 'api', 'k', now(), now())`, thread, "t"+thread.String()[:12], o.ID, p.ID, a.ID); err != nil {
		return runtimetest.Seeded{}, err
	}
	body := []byte(`{"id":"msg_user_1","role":"user","content":` + quoteJSON(text) + `}`)
	if _, err := d.store.AppendMessage(ctx, thread, run.Message{Role: run.RoleUser, Body: body}); err != nil {
		return runtimetest.Seeded{}, err
	}
	return runtimetest.Seeded{ThreadID: thread}, nil
}

func quoteJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func (d *runDriver) Execute(ctx context.Context, threadID uuid.UUID, token string) (runtimetest.Outcome, error) {
	ctx = d.ctx(ctx)
	claim, err := d.store.Claim(ctx, threadID, token, time.Minute)
	ok := claim == run.Claimed
	if err != nil {
		return runtimetest.Outcome{}, err
	}
	if !ok {
		return runtimetest.Outcome{}, errors.New("runDriver: the thread is leased")
	}
	defer func() { _ = d.store.Release(context.WithoutCancel(ctx), threadID, token) }()
	r, err := d.svc.Execute(ctx, threadID, token)
	if err != nil {
		return runtimetest.Outcome{}, err
	}
	if r == nil {
		return runtimetest.Outcome{}, errors.New("runDriver: nothing ran")
	}
	return runtimetest.Outcome{RunID: r.ID, Status: string(r.Status), Attempt: int(r.Attempt)}, nil
}

func (d *runDriver) State(ctx context.Context, threadID uuid.UUID) (runtimetest.State, error) {
	var st runtimetest.State
	err := d.admin.QueryRowContext(ctx, `SELECT id, status, attempt, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens
		FROM `+d.schema+`.runs WHERE thread_id = $1 ORDER BY created_at DESC LIMIT 1`, threadID).
		Scan(&st.RunID, &st.RunStatus, &st.Attempt, &st.Usage.InputTokens, &st.Usage.OutputTokens,
			&st.Usage.CacheReadInputTokens, &st.Usage.CacheWriteInputTokens)
	if err != nil {
		return st, err
	}
	rows, err := d.admin.QueryContext(ctx, `SELECT role FROM `+d.schema+`.messages WHERE thread_id = $1 ORDER BY seq`, threadID)
	if err != nil {
		return st, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return st, err
		}
		st.Roles = append(st.Roles, role)
	}
	if err := rows.Err(); err != nil {
		return st, err
	}
	var snaps int
	err = d.admin.QueryRowContext(ctx, `SELECT count(*) FROM `+d.schema+`.thread_snapshots WHERE thread_id = $1`, threadID).Scan(&snaps)
	st.SnapshotSaved = snaps == 1
	return st, err
}

func TestRun_AgainstRuntimeContainer(t *testing.T) {
	runtimetest.Run(t, newRunDriver)
}
