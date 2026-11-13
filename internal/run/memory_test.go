package run_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/runtime"
	"altalune.id/oagent/internal/memory"
	"altalune.id/oagent/internal/memory/memorytest"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/testutil/fakes"
)

type memFixture struct {
	*svcFixture
	mem     *fakes.RunMemory
	people  *fakes.RunRememberers
	person  uuid.UUID
	storeID uuid.UUID
}

func newMemSvc(t *testing.T) *memFixture {
	t.Helper()
	f := &memFixture{svcFixture: newSvc(t), person: uuid.New(), storeID: uuid.New()}
	f.mem = &fakes.RunMemory{Facts: []run.RecalledFact{{Text: "Lives in Bandung.", CreatedAt: time.Date(2026, 11, 1, 9, 30, 0, 0, time.FixedZone("WIB", 7*3600))}}}
	f.people = &fakes.RunRememberers{People: map[uuid.UUID]bool{f.person: true}}
	f.version.MemoryStoreID, f.version.HelperModel = f.storeID, "gpt-4o-mini"
	f.providers.Models[f.provider]["gpt-4o-mini"] = run.ResolvedModel{BaseURL: "https://llm.example.com/v1", APIKey: "sk-helper", Prices: &run.Prices{
		Input: new(int64(150_000)), Output: new(int64(600_000)), CacheRead: new(int64(75_000)), CacheWrite: new(int64(0)),
	}}
	f.store.Actors = map[uuid.UUID]uuid.UUID{f.thread: f.person}
	f.svc.WithMemory(f.mem, f.people).WithKeyPrefix("oagent_")
	return f
}

func (f *memFixture) speak(t *testing.T, speaker uuid.UUID, text string) {
	t.Helper()
	m := userMsg(text, "")
	m.SpeakerID = &speaker
	_, err := f.store.AppendMessage(f.ctx, f.thread, m)
	require.NoError(t, err)
}

func withTerminal(t *testing.T, raw []byte, extra string) []byte {
	t.Helper()
	trimmed := bytes.TrimSpace(raw)
	i := bytes.LastIndexByte(trimmed, '\n') + 1
	last := bytes.Replace(trimmed[i:], []byte(`,"messagesAppended"`), []byte(","+extra+`,"messagesAppended"`), 1)
	require.NotEqual(t, trimmed[i:], last)
	return append(append([]byte(nil), trimmed[:i]...), last...)
}

func TestRun_MemorySentOnAnEligibleLeg(t *testing.T) {
	f := newMemSvc(t)
	f.version.MaxCostCents = 50
	f.speak(t, f.person, "when can you call me?")
	r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})

	req := f.rt.LastRequest()
	require.NotNil(t, req.Memory)
	assert.Equal(t, runtime.ExtractFacts, req.Memory.Extract)
	assert.Equal(t, []runtime.MemoryFact{{Text: "Lives in Bandung.", CreatedAt: time.Date(2026, 11, 1, 2, 30, 0, 0, time.UTC)}}, req.Memory.Facts, "sent in UTC")
	assert.Equal(t, &runtime.Guardrails{KeyPrefix: "oagent_"}, req.Guardrails)
	require.NotNil(t, req.Agent.HelperModel)
	assert.Equal(t, "gpt-4o-mini", req.Agent.HelperModel.ModelID)
	assert.Equal(t, "sk-helper", req.Agent.HelperModel.APIKey)
	require.NotNil(t, req.Limits.Budget)
	assert.Equal(t, &runtime.Prices{Input: 150_000, Output: 600_000, CacheRead: 75_000}, req.Limits.Budget.HelperPrices)
	assert.Equal(t, []run.MemoryScope{{OrgID: f.org, ProjectID: f.project, StoreID: f.storeID, AgentID: f.agentID, EndUserID: f.person}}, f.mem.Recalled,
		"the scope is the leg's speaker in the version's store")
	assert.Equal(t, []run.MemoryScope{{OrgID: f.org, ProjectID: f.project, StoreID: f.storeID, AgentID: f.agentID}}, f.mem.Stores,
		"the store is asked once per turn how it extracts")
	assert.Contains(t, f.mem.Queries[0], "when can you call me?")
	require.NotNil(t, r.Prices)
	assert.Equal(t, int64(150_000), *r.Prices.HelperInput, "the helper model's prices are pinned at run open")
}

func TestRun_NoMemoryNoNewFields(t *testing.T) {
	f := newMemSvc(t)
	f.version.MemoryStoreID = uuid.Nil
	f.version.MaxCostCents = 50
	f.speak(t, f.person, "hello")
	r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	req := f.rt.LastRequest()
	assert.Nil(t, req.Memory)
	assert.Nil(t, req.Guardrails)
	assert.Nil(t, req.Agent.HelperModel)
	assert.Nil(t, req.Limits.Budget.HelperPrices)
	assert.Empty(t, f.mem.Recalled)
	assert.Nil(t, r.Prices.HelperInput, "no helper prices without memory")
}

func TestRun_MemoryLegEligibility(t *testing.T) {
	for name, c := range map[string]struct {
		setup  func(f *memFixture)
		speaks func(t *testing.T, f *memFixture)
	}{
		"a draft (Try it)": {setup: func(f *memFixture) { f.version.Draft = true }},
		"a group thread": {setup: func(f *memFixture) {
			f.store.Kinds = map[uuid.UUID]string{f.thread: "group"}
		}},
		"no speaker": {speaks: func(t *testing.T, f *memFixture) { f.say(t, "hello") }},
		"one speaker plus an unattributed message": {speaks: func(t *testing.T, f *memFixture) {
			f.speak(t, f.person, "hello")
			f.say(t, "who wrote this?")
		}},
		"two speakers": {setup: func(f *memFixture) {
			f.store.Kinds, f.store.Actors = map[uuid.UUID]string{f.thread: "dm"}, nil
		}, speaks: func(t *testing.T, f *memFixture) {
			other := uuid.New()
			f.people.People[other] = true
			f.speak(t, f.person, "hello")
			f.speak(t, other, "me too")
		}},
		"a speaker who is not the API thread's actor": {speaks: func(t *testing.T, f *memFixture) { f.speak(t, uuid.New(), "hello") }},
		"not a person (key or anonymous)":             {setup: func(f *memFixture) { f.people.People = nil }},
		"the person lookup fails":                     {setup: func(f *memFixture) { f.people.Err = errors.New("db down") }},
	} {
		t.Run(name, func(t *testing.T) {
			f := newMemSvc(t)
			if c.setup != nil {
				c.setup(f)
			}
			if c.speaks != nil {
				c.speaks(t, f)
			} else {
				f.speak(t, f.person, "hello")
			}
			f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
			req := f.rt.LastRequest()
			assert.Nil(t, req.Memory, "no memory")
			assert.Nil(t, req.Guardrails)
			assert.Empty(t, f.mem.Recalled, "nothing recalled")
		})
	}
}

func TestRun_DMSpeakerIsRemembered(t *testing.T) {
	f := newMemSvc(t)
	f.store.Kinds = map[uuid.UUID]string{f.thread: "dm"}
	f.store.Actors = nil
	f.speak(t, f.person, "hello")
	f.execute(t, fakes.RuntimeReply{Stream: withTerminal(t, stream(t, "plain_reply"), `"memory":{"facts":["Prefers tea."]}`)})
	require.NotNil(t, f.rt.LastRequest().Memory, "a DM has no actor; its single speaker is the scope")
	assert.Equal(t, f.person, f.mem.Recalled[0].EndUserID)
	require.Len(t, f.mem.Records, 1)
	assert.Equal(t, f.person, f.mem.Records[0].Scope.EndUserID, "the record uses the leg's scope, never one rebuilt from the thread")
}

func TestRun_HelperPrices(t *testing.T) {
	t.Run("no separate helper copies the main prices", func(t *testing.T) {
		f := newMemSvc(t)
		f.version.HelperModel = ""
		f.speak(t, f.person, "hello")
		r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
		assert.Equal(t, int64(2_500_000), *r.Prices.HelperInput)
		req := f.rt.LastRequest()
		assert.Equal(t, runtime.ExtractFacts, req.Memory.Extract)
		assert.Nil(t, req.Agent.HelperModel, "the runtime uses the main model")
	})
	t.Run("a store that extracts pins none and sends user texts", func(t *testing.T) {
		f := newMemSvc(t)
		f.mem.ExtractsFacts = true
		f.speak(t, f.person, "hello")
		r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
		assert.Nil(t, r.Prices.HelperInput)
		req := f.rt.LastRequest()
		assert.Equal(t, runtime.ExtractUserTexts, req.Memory.Extract)
		assert.Nil(t, req.Agent.HelperModel)
	})
}

func TestRun_PricedRunNeverTakesUnpricedHelperTokens(t *testing.T) {
	for name, helper := range map[string]run.ResolvedModel{
		"helper unpriced": {BaseURL: "https://llm.example.com/v1", APIKey: "sk-helper"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newMemSvc(t)
			f.version.MaxCostCents = 50
			f.providers.Models[f.provider]["gpt-4o-mini"] = helper
			f.speak(t, f.person, "hello")
			r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
			req := f.rt.LastRequest()
			require.NotNil(t, req.Memory, "recall still runs")
			assert.Equal(t, runtime.ExtractNone, req.Memory.Extract)
			assert.Nil(t, req.Agent.HelperModel)
			assert.Nil(t, req.Limits.Budget.HelperPrices)
			assert.Nil(t, r.Prices.HelperInput)
		})
	}
	t.Run("helper not resolved", func(t *testing.T) {
		f := newMemSvc(t)
		delete(f.providers.Models[f.provider], "gpt-4o-mini")
		f.speak(t, f.person, "hello")
		f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
		req := f.rt.LastRequest()
		assert.Equal(t, runtime.ExtractNone, req.Memory.Extract)
		assert.Nil(t, req.Agent.HelperModel)
	})
}

func TestRun_RecordsCleanedFactsAfterSettle(t *testing.T) {
	f := newMemSvc(t)
	f.speak(t, f.person, "hello")
	facts := []string{`"  Prefers tea.  "`, `""`, `"` + strings.Repeat("é", 600) + `"`}
	for i := range 12 {
		facts = append(facts, `"fact `+string(rune('a'+i))+`"`)
	}
	r := f.execute(t, fakes.RuntimeReply{Stream: withTerminal(t, stream(t, "plain_reply"), `"memory":{"facts":[`+strings.Join(facts, ",")+`]}`)})
	assert.Equal(t, run.StatusCompleted, r.Status)
	require.Len(t, f.mem.Records, 1)
	rec := f.mem.Records[0]
	assert.Equal(t, run.MemoryScope{OrgID: f.org, ProjectID: f.project, StoreID: f.storeID, AgentID: f.agentID, EndUserID: f.person}, rec.Scope, "the leg's scope")
	assert.Equal(t, r.ID, rec.Record.RunID)
	require.Len(t, rec.Record.Facts, 10, "the caps hold at the trust boundary")
	assert.Equal(t, "Prefers tea.", rec.Record.Facts[0])
	assert.Equal(t, strings.Repeat("é", 500), rec.Record.Facts[1])
	assert.False(t, rec.Cancelled, "the record runs on its own context")
}

func TestRun_RecordsUserTextsForAStoreThatExtracts(t *testing.T) {
	f := newMemSvc(t)
	f.mem.ExtractsFacts = true
	f.speak(t, f.person, "hello")
	f.execute(t, fakes.RuntimeReply{Stream: withTerminal(t, stream(t, "plain_reply"), `"memory":{"userTexts":["call after [PHONE]"]}`)})
	require.Len(t, f.mem.Records, 1)
	assert.Equal(t, []string{"call after [PHONE]"}, f.mem.Records[0].Record.UserTexts)
}

func TestRun_MismatchedTerminalMemoryRecordsNothing(t *testing.T) {
	for name, memory := range map[string]string{
		"both":                     `"memory":{"facts":["a"],"userTexts":["b"]}`,
		"user texts in facts mode": `"memory":{"userTexts":["b"]}`,
		"skipped":                  `"memory":{}`,
	} {
		t.Run(name, func(t *testing.T) {
			f := newMemSvc(t)
			f.speak(t, f.person, "hello")
			r := f.execute(t, fakes.RuntimeReply{Stream: withTerminal(t, stream(t, "plain_reply"), memory)})
			assert.Equal(t, run.StatusCompleted, r.Status)
			assert.Empty(t, f.mem.Records)
		})
	}
}

func TestRun_RecordOnlyForCompletedLimitOrAwaiting(t *testing.T) {
	f := newMemSvc(t)
	f.speak(t, f.person, "hello")
	r := f.execute(t, fakes.RuntimeReply{Stream: withTerminal(t, stream(t, "model_error"), `"memory":{"facts":["a"]}`)})
	assert.Equal(t, run.StatusFailed, r.Status)
	assert.Empty(t, f.mem.Records, "a failed leg records nothing")
}

func TestRun_NoRecordWhenSettleFails(t *testing.T) {
	for name, set := range map[string]func(*fakes.RunStore){
		"settle error":         func(s *fakes.RunStore) { s.SettleErr = errors.New("db down") },
		"lease lost at settle": func(s *fakes.RunStore) { s.SettleErr = &run.LeaseLostError{} },
	} {
		t.Run(name, func(t *testing.T) {
			f := newMemSvc(t)
			f.speak(t, f.person, "hello")
			f.rt.Replies = append(f.rt.Replies, fakes.RuntimeReply{Stream: withTerminal(t, stream(t, "plain_reply"), `"memory":{"facts":["a"]}`)})
			token := f.claim(t)
			set(f.store)
			_, err := f.svc.Execute(f.ctx, f.thread, token)
			require.Error(t, err)
			assert.Empty(t, f.mem.Records, "nothing is recorded for a leg that did not settle")
		})
	}
}

func TestRun_MemoryFailuresAreFailOpen(t *testing.T) {
	f := newMemSvc(t)
	f.mem.RecallErr, f.mem.RecordErr = errors.New("recall down"), errors.New("record down")
	f.speak(t, f.person, "hello")
	r := f.execute(t, fakes.RuntimeReply{Stream: withTerminal(t, stream(t, "plain_reply"), `"memory":{"facts":["a"]}`)})
	assert.Equal(t, run.StatusCompleted, r.Status, "memory never fails a turn")
	req := f.rt.LastRequest()
	require.NotNil(t, req.Memory)
	assert.Empty(t, req.Memory.Facts, "a failed recall sends no facts")
	assert.Len(t, f.mem.Records, 1)
}

func TestRun_ResumeAndContinueLegsCarryNoMemory(t *testing.T) {
	f := newMemSvc(t)
	f.speak(t, f.person, "record lunch")
	f.execute(t, fakes.RuntimeReply{Stream: cut(stream(t, "tool_call"))})
	require.NotNil(t, f.rt.LastRequest().Memory, "the first message leg remembers")
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "resume_after_crash")})
	req := f.rt.LastRequest()
	assert.Equal(t, runtime.InputContinue, req.Input.Kind)
	assert.Nil(t, req.Memory, "a crash continue never recalls")
	assert.Nil(t, req.Guardrails)
}

type storeMemory struct{ store memory.Facts }

func (s storeMemory) Store(context.Context, run.MemoryScope) (run.StoreMode, error) {
	return run.StoreMode{Extracts: s.store.Extracts()}, nil
}

func (s storeMemory) Recall(ctx context.Context, sc run.MemoryScope, q string) ([]run.RecalledFact, error) {
	facts, err := s.store.Recall(ctx, memory.Scope(sc), q, runtime.MaxMemoryFacts)
	out := make([]run.RecalledFact, len(facts))
	for i, f := range facts {
		out[i] = run.RecalledFact{Text: f.Text, CreatedAt: f.CreatedAt}
	}
	return out, err
}

func (s storeMemory) Record(ctx context.Context, sc run.MemoryScope, rec run.MemoryRecord) error {
	return s.store.Record(ctx, memory.Scope(sc), memory.Turn{RunID: rec.RunID, Facts: rec.Facts, UserTexts: rec.UserTexts})
}

func TestRun_AStoreThatExtractsGetsTheUserTexts(t *testing.T) {
	f := newMemSvc(t)
	f.version.MaxCostCents = 50
	store := memorytest.NewFake(memorytest.Extracting())
	f.svc.WithMemory(storeMemory{store: store}, f.people)
	f.speak(t, f.person, "my number is 0812 3456 7891, call after 6")
	r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "memory_user_texts")})

	req := f.rt.LastRequest()
	require.NotNil(t, req.Memory)
	assert.Equal(t, runtime.ExtractUserTexts, req.Memory.Extract, "the store extracts, so the runtime sends the person's texts")
	assert.Nil(t, req.Agent.HelperModel, "no helper runs")
	assert.Nil(t, req.Limits.Budget.HelperPrices)
	assert.Nil(t, r.Prices.HelperInput, "and no helper prices are pinned")

	scope := memory.Scope{OrgID: f.org, ProjectID: f.project, StoreID: f.storeID, AgentID: f.agentID, EndUserID: f.person}
	facts, _, _, err := store.List(f.ctx, scope, "", 10)
	require.NoError(t, err)
	require.Len(t, facts, 1, "the terminal's user texts reached Record")
	assert.Equal(t, "my number is [PHONE], call after 6", facts[0].Text)
}

func TestRun_AnUnusableStoreGivesNoMemory(t *testing.T) {
	f := newMemSvc(t)
	f.version.MaxCostCents = 50
	f.mem.StoreErr = errors.New("memory: invalid store: backend_disabled")
	f.speak(t, f.person, "hello")
	r := f.execute(t, fakes.RuntimeReply{Stream: withTerminal(t, stream(t, "plain_reply"), `"memory":{"facts":["Prefers tea."]}`)})
	assert.Equal(t, run.StatusCompleted, r.Status, "the turn still runs")
	req := f.rt.LastRequest()
	assert.Nil(t, req.Memory, "a gone store or a disabled backend fails closed")
	assert.Nil(t, req.Agent.HelperModel)
	assert.Nil(t, r.Prices.HelperInput, "no helper prices are pinned for a leg that cannot remember")
	assert.Empty(t, f.mem.Recalled)
	assert.Empty(t, f.mem.Records)
	assert.Len(t, f.mem.Stores, 1, "the store is asked once per turn")
}

func TestRun_ANamedAPIUserIsRememberedOnlyOnAStoreThatOptsIn(t *testing.T) {
	f := newMemSvc(t)
	f.people.People, f.people.Named = nil, map[uuid.UUID]bool{f.person: true}
	f.speak(t, f.person, "hello")
	f.execute(t, fakes.RuntimeReply{Stream: withTerminal(t, stream(t, "plain_reply"), `"memory":{"facts":["Prefers tea."]}`)})
	assert.Nil(t, f.rt.LastRequest().Memory, "any key of the project can name this user, so a store that does not opt in remembers nothing")
	assert.Empty(t, f.mem.Recalled)
	assert.Empty(t, f.mem.Records)

	f.mem.APIUsers = true
	f.speak(t, f.person, "hello again")
	f.execute(t, fakes.RuntimeReply{Stream: withTerminal(t, stream(t, "plain_reply"), `"memory":{"facts":["Prefers tea."]}`)})
	require.NotNil(t, f.rt.LastRequest().Memory, "a store that opts in remembers the named user")
	assert.Len(t, f.mem.Records, 1)
}

func TestRun_APersonIsRememberedWithoutTheOptIn(t *testing.T) {
	f := newMemSvc(t)
	require.False(t, f.mem.APIUsers)
	f.speak(t, f.person, "hello")
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	assert.NotNil(t, f.rt.LastRequest().Memory, "a WhatsApp sender, a signed widget user or a person: key needs no opt-in")
}

func TestRun_AKeyActingForItselfIsNeverRememberedEvenOnAnOptedStore(t *testing.T) {
	f := newMemSvc(t)
	f.people.People = nil
	f.mem.APIUsers = true
	f.speak(t, f.person, "hello")
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	assert.Nil(t, f.rt.LastRequest().Memory)
}

func TestRun_AMemoryLegLoadsHistoryOnce(t *testing.T) {
	f := newMemSvc(t)
	f.speak(t, f.person, "earlier")
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	f.speak(t, f.person, "when can you call me?")
	before := f.store.HistoryCalls
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	require.NotNil(t, f.rt.LastRequest().Memory)
	assert.Contains(t, f.mem.Queries[len(f.mem.Queries)-1], "earlier", "the recall query still reads the previous message")
	assert.Equal(t, 1, f.store.HistoryCalls-before, "recall and the request share one history read")
}

func TestRun_TheFallbackRecallQueryIsCut(t *testing.T) {
	f := newMemSvc(t)
	long := strings.Repeat("a", 8000) + strings.Repeat("é", 2000)
	f.speak(t, f.person, long)
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	require.Len(t, f.mem.Queries, 1)
	assert.Equal(t, strings.Repeat("é", 2000), f.mem.Queries[0], "a first message recalls with its last 2000 runes, the knowledge query cap")
}
