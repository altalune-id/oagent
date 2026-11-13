//go:build integration

package run_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/runtime"
	"altalune.id/oagent/internal/memory"
	"altalune.id/oagent/internal/memory/native"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/testutil/fakes"
)

type serviceMemory struct{ svc *memory.Service }

func (n serviceMemory) Store(ctx context.Context, s run.MemoryScope) (run.StoreMode, error) {
	mode, err := n.svc.Mode(ctx, memory.Scope(s))
	return run.StoreMode(mode), err
}

func (n serviceMemory) Recall(ctx context.Context, s run.MemoryScope, q string) ([]run.RecalledFact, error) {
	facts, err := n.svc.Recall(ctx, memory.Scope(s), q, runtime.MaxMemoryFacts)
	out := make([]run.RecalledFact, len(facts))
	for i, f := range facts {
		out[i] = run.RecalledFact{Text: f.Text, CreatedAt: f.CreatedAt}
	}
	return out, err
}

func (n serviceMemory) Record(ctx context.Context, s run.MemoryScope, rec run.MemoryRecord) error {
	return n.svc.Record(ctx, memory.Scope(s), memory.Turn{RunID: rec.RunID, Facts: rec.Facts})
}

type memStack struct {
	*pgFixture
	s                scope
	agentY, versionY uuid.UUID
	storeA, storeB   uuid.UUID
	versions         map[uuid.UUID]*run.AgentVersion
	rt               *fakes.Runtime
	svc              *run.Service
	people           *fakes.RunRememberers
}

func newMemStack(t *testing.T) *memStack {
	t.Helper()
	f := newPgFixture(t)
	m := &memStack{pgFixture: f, s: f.seed(t), rt: &fakes.Runtime{}, people: &fakes.RunRememberers{People: map[uuid.UUID]bool{}}}
	m.agentY, m.versionY = uuid.New(), uuid.New()
	versionY := m.versionY
	p := f.schema + "."
	f.exec(t, `INSERT INTO `+p+`agents (id, public_id, org_id, project_id, slug, name, created_at, updated_at) VALUES ($1, $2, $3, $4, 'y', 'Y', now(), now())`,
		m.agentY, "a"+m.agentY.String()[:12], m.s.org, m.s.project)
	f.exec(t, `INSERT INTO `+p+`agent_versions (id, public_id, org_id, project_id, agent_id, version, config, created_at, kind) VALUES ($1, $2, $3, $4, $5, 1, '{}', now(), 'published')`,
		versionY, "v"+versionY.String()[:12], m.s.org, m.s.project, m.agentY)
	m.storeA, m.storeB = uuid.New(), uuid.New()
	for _, st := range []uuid.UUID{m.storeA, m.storeB} {
		f.exec(t, `INSERT INTO `+p+`memory_stores (id, public_id, org_id, project_id, name, backend, created_at, updated_at) VALUES ($1, $2, $3, $4, $2, 'native', now(), now())`,
			st, "m"+st.String()[:12], m.s.org, m.s.project)
	}
	provider := uuid.New()
	m.versions = map[uuid.UUID]*run.AgentVersion{
		m.s.version: {ID: m.s.version, AgentID: m.s.agent, Version: 1, Model: "gpt-4o", ProviderID: provider, MemoryStoreID: m.storeA},
		versionY:    {ID: versionY, AgentID: m.agentY, Version: 1, Model: "gpt-4o", ProviderID: provider, MemoryStoreID: m.storeA},
	}
	versions := m.versions
	mem := memory.NewService(memory.NewPostgresStore(f.pool, f.schema, ""),
		map[memory.Backend]memory.Facts{memory.BackendNative: native.New(f.pool, f.schema, "")}, []memory.Backend{memory.BackendNative},
		memory.Deps{UOW: db.NewTransactor(f.pool)}, apperror.NewReporter(nil, false).Unexpected)
	m.svc = run.NewService(f.store, db.NewTransactor(f.pool), apperror.NewReporter(nil, false).Unexpected).
		WithRuntime(m.rt).WithAgents(&fakes.RunAgents{Versions: versions}).
		WithProviders(&fakes.RunProviders{Models: map[uuid.UUID]map[string]run.ResolvedModel{provider: {
			"gpt-4o": {BaseURL: "https://llm.example.com/v1", APIKey: "sk-test"},
		}}}).
		WithMCPServers(&fakes.RunMCPServers{}).
		WithMemory(serviceMemory{svc: mem}, m.people).WithKeyPrefix("oagent_")
	return m
}

func (m *memStack) person(t *testing.T) uuid.UUID {
	t.Helper()
	id := m.endUser(t, m.s)
	m.people.People[id] = true
	return id
}

func (m *memStack) apiThread(t *testing.T, agentID, actor uuid.UUID) uuid.UUID {
	t.Helper()
	th, err := m.store.UpsertAPIThread(in(m.s.org), m.s.project, agentID, uuid.NewString(), actor)
	require.NoError(t, err)
	return th.ID
}

func (m *memStack) turn(t *testing.T, thread uuid.UUID, terminalFacts string, speakers ...uuid.UUID) {
	t.Helper()
	for _, sp := range speakers {
		msg := userMsg("hello there", "")
		msg.SpeakerID = &sp
		_, err := m.store.AppendMessage(in(m.s.org), thread, msg)
		require.NoError(t, err)
	}
	m.rt.Replies = append(m.rt.Replies, fakes.RuntimeReply{Stream: withTerminal(t, stream(t, "plain_reply"), `"memory":{"facts":[`+terminalFacts+`]}`)})
	token := m.claimed(t, m.s, thread)
	_, err := m.svc.Execute(in(m.s.org), thread, token)
	require.NoError(t, err)
	require.NoError(t, m.store.Release(in(m.s.org), thread, token))
}

func (m *memStack) factsOf(t *testing.T, endUser, agentID uuid.UUID) int {
	t.Helper()
	return m.count(t, `SELECT count(*) FROM `+m.schema+`.memory_facts WHERE end_user_id = $1 AND agent_id = $2`, endUser, agentID)
}

func (m *memStack) factsIn(t *testing.T, store, endUser uuid.UUID) int {
	t.Helper()
	return m.count(t, `SELECT count(*) FROM `+m.schema+`.memory_facts WHERE store_id = $1 AND end_user_id = $2`, store, endUser)
}

func recalled(req runtime.TurnRequest) []string {
	var out []string
	for _, f := range req.Memory.Facts {
		out = append(out, f.Text)
	}
	return out
}

func TestMemory_TwoPeopleNeverSeeEachOther(t *testing.T) {
	m := newMemStack(t)
	a, b := m.person(t), m.person(t)
	m.turn(t, m.apiThread(t, m.s.agent, a), `"Lives in Bandung."`, a)
	require.Equal(t, 1, m.factsOf(t, a, m.s.agent))

	m.turn(t, m.apiThread(t, m.s.agent, b), `"Has two cats."`, b)
	req := m.rt.LastRequest()
	require.NotNil(t, req.Memory)
	assert.Empty(t, req.Memory.Facts, "B's leg carries none of A's facts")
	assert.Equal(t, 1, m.factsOf(t, b, m.s.agent), "B's fact lands under B")
	assert.Equal(t, 1, m.factsOf(t, a, m.s.agent), "and A keeps only A's")
}

func TestMemory_AnotherPersonsThreadIsRefused(t *testing.T) {
	m := newMemStack(t)
	a, b := m.person(t), m.person(t)
	th, err := m.store.UpsertAPIThread(in(m.s.org), m.s.project, m.s.agent, "a-thread", a)
	require.NoError(t, err)
	_, err = m.store.UpsertAPIThread(in(m.s.org), m.s.project, m.s.agent, "a-thread", b)
	var mismatch *run.ThreadActorMismatchError
	require.ErrorAs(t, err, &mismatch, "B cannot post to A's thread, so no run and no recall")
	assert.Zero(t, m.count(t, `SELECT count(*) FROM `+m.schema+`.runs WHERE thread_id = $1`, th.ID))
}

func TestMemory_TwoSpeakersInOneLegAreNotRemembered(t *testing.T) {
	m := newMemStack(t)
	a, b := m.person(t), m.person(t)
	thread := m.apiThread(t, m.s.agent, a)
	m.turn(t, thread, `"Lives in Bandung."`, a, b)
	assert.Nil(t, m.rt.LastRequest().Memory, "a leg with two speakers is ineligible")
	assert.Zero(t, m.factsOf(t, a, m.s.agent))
	assert.Zero(t, m.factsOf(t, b, m.s.agent))
}

func TestMemory_AnotherAgentNeverRecalls(t *testing.T) {
	m := newMemStack(t)
	a := m.person(t)
	m.turn(t, m.apiThread(t, m.s.agent, a), `"Lives in Bandung."`, a)
	m.turn(t, m.apiThread(t, m.agentY, a), `"Prefers tea."`, a)
	req := m.rt.LastRequest()
	require.NotNil(t, req.Memory)
	assert.Empty(t, req.Memory.Facts, "agent Y never recalls what agent X learned")
	assert.Equal(t, 1, m.factsOf(t, a, m.agentY))
}

func TestMemory_ASpeakerWhoIsNotTheThreadsActorIsNotRemembered(t *testing.T) {
	m := newMemStack(t)
	a, b := m.person(t), m.person(t)
	thread := m.apiThread(t, m.s.agent, a)
	m.turn(t, thread, `"Lives in Bandung."`, b)
	assert.Nil(t, m.rt.LastRequest().Memory, "the speaker must be the API thread's actor")
	assert.Zero(t, m.factsOf(t, a, m.s.agent))
	assert.Zero(t, m.factsOf(t, b, m.s.agent))
}

func (m *memStack) dm(t *testing.T, c chat, person uuid.UUID, chatKey, text string) uuid.UUID {
	t.Helper()
	body, err := json.Marshal(map[string]any{"id": uuid.NewString(), "role": "user", "content": text})
	require.NoError(t, err)
	at := time.Now()
	got, err := m.store.AppendChannelMessage(in(m.s.org), run.ChannelMessage{
		ProjectID: m.s.project, AgentID: m.s.agent, ConnectionID: c.conn, ChatKey: chatKey, Kind: "dm",
		Message:   run.Message{Body: body, SpeakerID: &person, ExternalMessageID: uuid.NewString(), ChannelTime: &at},
		SenderCap: 1000, Actor: person, ReplyVia: "wa_phone",
	})
	require.NoError(t, err)
	return got.ThreadID
}

func (m *memStack) dmTurn(t *testing.T, thread uuid.UUID, terminalFacts string) {
	t.Helper()
	m.rt.Replies = append(m.rt.Replies, fakes.RuntimeReply{Stream: withTerminal(t, stream(t, "plain_reply"), `"memory":{"facts":[`+terminalFacts+`]}`)})
	token := m.claimed(t, m.s, thread)
	_, err := m.svc.Execute(in(m.s.org), thread, token)
	require.NoError(t, err)
	require.NoError(t, m.store.Release(in(m.s.org), thread, token))
}

func TestMemory_TwoDMPeopleNeverSeeEachOther(t *testing.T) {
	m := newMemStack(t)
	c := m.chat(t, m.s, "dm")
	a, b := c.person, m.person(t)
	m.people.People[a] = true
	threadA := m.dm(t, c, a, "cht_a", "hello, I live in Bandung")
	threadB := m.dm(t, c, b, "cht_b", "hello there")
	require.NotEqual(t, threadA, threadB, "one DM thread per person")

	m.dmTurn(t, threadA, `"Lives in Bandung."`)
	require.Equal(t, 1, m.factsOf(t, a, m.s.agent))

	m.dmTurn(t, threadB, `"Has two cats."`)
	req := m.rt.LastRequest()
	require.NotNil(t, req.Memory, "B's DM leg remembers")
	assert.Empty(t, req.Memory.Facts, "B's leg carries none of A's facts")
	assert.Equal(t, 1, m.factsOf(t, b, m.s.agent), "B's fact lands under B")
	assert.Equal(t, 1, m.factsOf(t, a, m.s.agent), "A still holds only A's")

	m.dm(t, c, a, "cht_a", "when can you call me?")
	m.dmTurn(t, threadA, `"Prefers calls after 6pm."`)
	req = m.rt.LastRequest()
	require.NotNil(t, req.Memory)
	var recalled []string
	for _, f := range req.Memory.Facts {
		recalled = append(recalled, f.Text)
	}
	assert.Equal(t, []string{"Lives in Bandung."}, recalled, "A's next leg recalls only A's facts")
}

func TestMemory_TwoAgentsOnDifferentStoresNeverCross(t *testing.T) {
	m := newMemStack(t)
	m.versions[m.versionY].MemoryStoreID = m.storeB
	a := m.person(t)
	m.turn(t, m.apiThread(t, m.s.agent, a), `"Lives in Bandung."`, a)
	require.Equal(t, 1, m.factsIn(t, m.storeA, a))

	m.turn(t, m.apiThread(t, m.agentY, a), `"Prefers tea."`, a)
	req := m.rt.LastRequest()
	require.NotNil(t, req.Memory)
	assert.Empty(t, req.Memory.Facts, "agent Y's store holds nothing agent X learned about the same person")
	assert.Equal(t, 1, m.factsIn(t, m.storeB, a), "Y's fact lands in Y's store")
	assert.Equal(t, 1, m.factsIn(t, m.storeA, a), "and X's store is unchanged")
}

func TestMemory_AfterAStoreSwitchTheOldFactsAreNotRecalled(t *testing.T) {
	m := newMemStack(t)
	a := m.person(t)
	thread := m.apiThread(t, m.s.agent, a)
	m.turn(t, thread, `"Lives in Bandung."`, a)
	require.Equal(t, 1, m.factsIn(t, m.storeA, a))

	m.versions[m.s.version].MemoryStoreID = m.storeB
	m.turn(t, thread, `"Prefers tea."`, a)
	req := m.rt.LastRequest()
	require.NotNil(t, req.Memory)
	assert.Empty(t, req.Memory.Facts, "the new store starts empty; the old store's facts stay out")
	assert.Equal(t, 1, m.factsIn(t, m.storeB, a))

	m.versions[m.s.version].MemoryStoreID = m.storeA
	m.turn(t, thread, ``, a)
	assert.Equal(t, []string{"Lives in Bandung."}, recalled(m.rt.LastRequest()), "switching back recalls the old store again")
}

func TestMemory_AStoreGoneMidwayGivesNoMemory(t *testing.T) {
	m := newMemStack(t)
	a := m.person(t)
	thread := m.apiThread(t, m.s.agent, a)
	m.exec(t, `DELETE FROM `+m.schema+`.memory_stores WHERE id = $1`, m.storeA)
	m.turn(t, thread, `"Lives in Bandung."`, a)
	assert.Nil(t, m.rt.LastRequest().Memory, "a deleted store fails closed")
	assert.Zero(t, m.factsIn(t, m.storeA, a))
}

func TestMemory_ASecondKeyNamingTheSameUserRecallsNothingOnAStoreThatDoesNotOptIn(t *testing.T) {
	m := newMemStack(t)
	named := m.endUser(t, m.s)
	m.people.Named = map[uuid.UUID]bool{named: true}
	m.exec(t, `INSERT INTO `+m.schema+`.memory_facts (id, public_id, org_id, project_id, store_id, agent_id, end_user_id, text, text_hash, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'Card ends 4242.', '\x01', now())`,
		uuid.New(), "f"+uuid.NewString()[:12], m.s.org, m.s.project, m.storeA, m.s.agent, named)

	m.turn(t, m.apiThread(t, m.s.agent, named), `"Prefers tea."`, named)
	assert.Nil(t, m.rt.LastRequest().Memory, "another integration's key names the user, and the store never answers it")
	assert.Equal(t, 1, m.factsIn(t, m.storeA, named), "nor records for it")

	m.exec(t, `UPDATE `+m.schema+`.memory_stores SET remember_api_users = true WHERE id = $1`, m.storeA)
	m.turn(t, m.apiThread(t, m.s.agent, named), ``, named)
	assert.Equal(t, []string{"Card ends 4242."}, recalled(m.rt.LastRequest()), "once the store opts in, the named user is remembered")
}
