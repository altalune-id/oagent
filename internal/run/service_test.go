package run_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/kernel/ratelimit/memory"
	"altalune.id/oagent/internal/kernel/runtime"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/testutil/fakes"
)

type svcFixture struct {
	store                         *fakes.RunStore
	snaps                         *fakes.SnapshotStore
	rt                            *fakes.Runtime
	agents                        *fakes.RunAgents
	providers                     *fakes.RunProviders
	servers                       *fakes.RunMCPServers
	svc                           *run.Service
	ctx                           context.Context
	org, project, thread, agentID uuid.UUID
	provider, server, gated       uuid.UUID
	version                       *run.AgentVersion
}

func stream(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "kernel", "runtime", "testdata", "turn", name+".stream.ndjson"))
	require.NoError(t, err)
	return raw
}

func cut(raw []byte) []byte {
	trimmed := bytes.TrimSpace(raw)
	return trimmed[:bytes.LastIndexByte(trimmed, '\n')+1]
}

func newSvc(t *testing.T) *svcFixture {
	t.Helper()
	f := &svcFixture{
		store: &fakes.RunStore{}, snaps: &fakes.SnapshotStore{}, rt: &fakes.Runtime{},
		org: uuid.New(), project: uuid.New(), thread: uuid.New(), agentID: uuid.New(),
		provider: uuid.New(), server: uuid.New(), gated: uuid.New(),
	}
	f.ctx = tenant.Into(context.Background(), tenant.Context{OrgID: f.org})
	f.version = &run.AgentVersion{
		ID: uuid.New(), AgentID: f.agentID, AgentPublicID: "agent-pub", Version: 3, Instructions: "You record expenses for the user. Be brief.",
		Model: "gpt-4o", Temperature: 0.2, MaxSteps: 1, ProviderID: f.provider,
		MCPServers: []run.MCPGrant{
			{ServerID: f.server, Tools: []run.ToolGrant{{Name: "record_expense"}, {Name: "delete_all", RequireApproval: true}}},
			{ServerID: f.gated, Tools: []run.ToolGrant{{Name: "wipe", RequireApproval: true}}},
		},
	}
	f.agents = &fakes.RunAgents{Versions: map[uuid.UUID]*run.AgentVersion{f.version.ID: f.version}}
	f.providers = &fakes.RunProviders{Models: map[uuid.UUID]map[string]run.ResolvedModel{f.provider: {
		"gpt-4o": {BaseURL: "https://llm.example.com/v1", APIKey: "sk-test", Prices: &run.Prices{
			Input: new(int64(2_500_000)), Output: new(int64(10_000_000)), CacheRead: new(int64(1_250_000)), CacheWrite: new(int64(0)),
		}},
	}}}
	f.servers = &fakes.RunMCPServers{Servers: map[uuid.UUID]run.OpenedServer{
		f.server: {Name: "yasaku", URL: "https://yasaku.example.com/mcp", AuthKind: "bearer", AuthValue: "tok-test", TimeoutMS: 5000},
		f.gated:  {Name: "danger", URL: "https://danger.example.com/mcp", AuthKind: "none", TimeoutMS: 5000},
	}}
	f.store.AddAgentThread(f.thread, f.org, f.project, f.agentID)
	f.store.PublicIDs = map[uuid.UUID]string{f.thread: "thread-pub"}
	f.svc = run.NewService(f.store, fakes.Transactor{}, apperror.NewReporter(nil, false).Unexpected).
		WithRuntime(f.rt).WithAgents(f.agents).WithProviders(f.providers).WithMCPServers(f.servers).
		WithSnapshots(f.snaps).WithTurnTimeout(5 * time.Minute).WithMaxAttempts(3)
	return f
}

func (f *svcFixture) say(t *testing.T, text string) {
	t.Helper()
	_, err := f.store.AppendMessage(f.ctx, f.thread, userMsg(text, ""))
	require.NoError(t, err)
}

func (f *svcFixture) claim(t *testing.T) string {
	t.Helper()
	token := uuid.NewString()
	ok, err := f.store.Claim(f.ctx, f.thread, token, time.Minute)
	require.NoError(t, err)
	require.True(t, ok)
	return token
}

func (f *svcFixture) execute(t *testing.T, replies ...fakes.RuntimeReply) *run.Run {
	t.Helper()
	f.rt.Replies = append(f.rt.Replies, replies...)
	token := f.claim(t)
	r, err := f.svc.Execute(f.ctx, f.thread, token)
	require.NoError(t, err)
	require.NoError(t, f.store.Release(f.ctx, f.thread, token))
	return r
}

func TestRun_PlainReplyCompletes(t *testing.T) {
	f := newSvc(t)
	f.say(t, "hello")
	r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	assert.Equal(t, run.StatusCompleted, r.Status)
	assert.Equal(t, int64(120), r.Usage.InputTokens)
	req := f.rt.LastRequest()
	assert.Equal(t, runtime.ContractVersion, req.ContractVersion)
	assert.Equal(t, run.SessionID(f.thread), req.IDs.Thread)
	assert.Equal(t, run.AgentKey(f.agentID), req.IDs.Agent)
	assert.Equal(t, r.PublicID, req.IDs.Run)
	assert.Equal(t, int32(3), req.IDs.AgentVersion)
	assert.Equal(t, "message", req.Input.Kind)
	assert.Contains(t, string(req.Input.Message), "hello")
	assert.Equal(t, int64((5*time.Minute-30*time.Second)/time.Millisecond), req.Limits.DeadlineMs)
	assert.Nil(t, req.Limits.Budget, "an uncapped agent has no budget")
	snap, err := f.snaps.Load(f.ctx, f.thread)
	require.NoError(t, err)
	require.NotNil(t, snap, "the terminal snapshot is saved")
	assert.Equal(t, "1.23.0", snap.SDKVersion)
	assert.Equal(t, f.version.ID, snap.AgentVersionID)
	var assistant int
	for _, m := range f.store.Messages {
		if m.Role == run.RoleAssistant {
			assistant++
		}
		if m.Role == run.RoleUser {
			assert.Equal(t, run.MessageConsumed, m.Status)
		}
	}
	assert.Equal(t, 1, assistant, "the reply is stored once; the echoed user message is not stored again")
}

func TestRun_LegacyVersionRefused(t *testing.T) {
	f := newSvc(t)
	f.version.Legacy = true
	f.say(t, "hello")
	r := f.execute(t)
	assert.Equal(t, run.StatusFailed, r.Status)
	assert.Equal(t, string(apperror.CodeAgentNeedsRepublish), r.ErrorCode)
	assert.Empty(t, f.rt.Requests, "no runtime call")
}

func TestRun_GatedToolNotExposedBeforeApprovals(t *testing.T) {
	f := newSvc(t)
	f.say(t, "record lunch")
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "tool_call")})
	req := f.rt.LastRequest()
	require.Len(t, req.MCPServers, 1, "a server with only gated tools is left out")
	s := req.MCPServers[0]
	assert.Equal(t, []string{"record_expense"}, s.AllowedTools)
	assert.Equal(t, runtime.MCPAuth{Kind: "bearer", Value: "tok-test"}, s.Auth)
	assert.Equal(t, int32(5000), s.CallTimeoutMs)
	assert.Equal(t, []uuid.UUID{f.server}, f.servers.Opened, "the gated server is never opened")
}

func TestRun_CappedUnpricedRefused(t *testing.T) {
	f := newSvc(t)
	f.version.MaxCostCents = 100
	m := f.providers.Models[f.provider]["gpt-4o"]
	m.Prices = nil
	f.providers.Models[f.provider]["gpt-4o"] = m
	f.say(t, "hello")
	r := f.execute(t)
	assert.Equal(t, run.StatusFailed, r.Status)
	assert.Equal(t, string(apperror.CodeAgentInvalidGrant), r.ErrorCode)
	assert.Empty(t, f.rt.Requests)
}

func TestRun_RetryResumesSameRun(t *testing.T) {
	f := newSvc(t)
	f.say(t, "record lunch")
	first := f.execute(t, fakes.RuntimeReply{Stream: cut(stream(t, "tool_call"))})
	assert.Equal(t, run.StatusInterrupted, first.Status)
	assert.True(t, first.Usage.Estimated, "a crash is charged from Go's own estimate")
	assert.Positive(t, first.Usage.OutputTokens)

	again := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "tool_call")})
	assert.Equal(t, first.ID, again.ID, "the same run resumes")
	assert.Equal(t, int32(2), again.Attempt)
	assert.Equal(t, run.StatusCompleted, again.Status)
	req := f.rt.LastRequest()
	assert.Equal(t, int32(2), req.IDs.Attempt)
	require.Len(t, req.StoredResults, 1)
	assert.Equal(t, "call_1", req.StoredResults[0].ToolUseID)
	assert.JSONEq(t, `{"status":"success","content":[{"json":{"id":"exp_1","amount":25000,"account":"cash"}}]}`, string(req.StoredResults[0].Result),
		"the stored result is the runtime's own, byte for byte in meaning")
	assert.Equal(t, runtime.InputContinue, req.Input.Kind, "the resumed turn continues after its tool round")
	assert.Contains(t, string(req.History[0]), "record lunch", "the same ask leads its history")
}

func TestRun_FailureKeepsCompletedToolResults(t *testing.T) {
	f := newSvc(t)
	f.say(t, "record lunch")
	f.execute(t, fakes.RuntimeReply{Stream: cut(stream(t, "tool_call"))})
	r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "model_error")})
	assert.Equal(t, run.StatusFailed, r.Status)
	assert.Equal(t, string(apperror.CodeRunRuntimeFailed), r.ErrorCode)
	stored, err := f.store.StoredResults(f.ctx, r.ID)
	require.NoError(t, err)
	require.Len(t, stored, 1, "the finished tool call stays recorded")
}

func TestRun_LimitReachedWritesUsage(t *testing.T) {
	f := newSvc(t)
	f.version.MaxCostCents = 50
	f.say(t, "record lunch")
	r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "limit_reached")})
	assert.Equal(t, run.StatusLimitReached, r.Status)
	assert.Equal(t, run.Usage{InputTokens: 890, OutputTokens: 40}, r.Usage)
	budget := f.rt.LastRequest().Limits.Budget
	require.NotNil(t, budget)
	assert.Equal(t, int64(500_000), budget.RemainingMicros)
	assert.Equal(t, int64(2_500_000), budget.Prices.Input)
}

func (f *svcFixture) priorTurn(t *testing.T) {
	t.Helper()
	f.say(t, "hello")
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
}

func TestRun_SnapshotMismatchRebuildsFromMessages(t *testing.T) {
	f := newSvc(t)
	f.priorTurn(t)
	snap, err := f.snaps.Load(f.ctx, f.thread)
	require.NoError(t, err)
	snap.AgentVersionID = uuid.New()
	require.NoError(t, f.snaps.Save(f.ctx, snap))
	f.say(t, "what did I just record?")
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	req := f.rt.LastRequest()
	assert.Nil(t, req.Snapshot, "a snapshot of another version is not sent")
	require.Len(t, req.History, 2, "the earlier turn is rebuilt from messages")
	assert.Contains(t, string(req.History[0]), "hello")
	assert.Contains(t, string(req.Input.Message), "what did I just record?")
}

func TestRun_SnapshotLoadErrorRebuilds(t *testing.T) {
	f := newSvc(t)
	f.priorTurn(t)
	f.say(t, "again")
	r := f.execute(t,
		fakes.RuntimeReply{Err: &runtime.SnapshotUnloadableError{}},
		fakes.RuntimeReply{Stream: stream(t, "plain_reply")},
	)
	assert.Equal(t, run.StatusCompleted, r.Status)
	assert.Equal(t, int32(1), r.Attempt, "the rebuild is not an attempt")
	require.Len(t, f.rt.Requests, 3)
	assert.NotNil(t, f.rt.Requests[1].Snapshot)
	assert.Nil(t, f.rt.Requests[2].Snapshot)
	assert.Len(t, f.rt.Requests[2].History, 2)
}

func TestResume_InterruptExpiresOnRebuild(t *testing.T) {
	f := newSvc(t)
	f.say(t, "delete everything")
	token := f.claim(t)
	r := run.New(f.org, f.project, f.thread, f.version.ID, time.Now())
	_, err := f.store.StartRun(f.ctx, token, r)
	require.NoError(t, err)
	asked := json.RawMessage(`{"id":"msg_asst_9","role":"assistant","toolCalls":[{"id":"call_9","type":"function","function":{"name":"danger__wipe","arguments":"{}"}}]}`)
	require.NoError(t, r.Move(run.StatusCompleted, time.Now()))
	require.NoError(t, f.store.Settle(f.ctx, token, r, run.Usage{}, []run.Message{{Role: run.RoleAssistant, Body: asked}}))
	require.NoError(t, f.store.Release(f.ctx, f.thread, token))

	f.say(t, "never mind")
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	history := f.rt.LastRequest().History
	require.Len(t, history, 3)
	var expired map[string]any
	require.NoError(t, json.Unmarshal(history[2], &expired))
	assert.Equal(t, "tool", expired["role"])
	assert.Equal(t, "call_9", expired["toolCallId"])
	assert.Contains(t, expired["content"], "expired", "an unanswered tool call is closed with a clear message")
}

func TestRun_CurrentInstructionsAfterSnapshotLoad(t *testing.T) {
	f := newSvc(t)
	f.priorTurn(t)
	f.version.Instructions = "Answer in Indonesian."
	f.say(t, "again")
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	req := f.rt.LastRequest()
	assert.NotNil(t, req.Snapshot)
	assert.Empty(t, req.History, "a snapshot replaces the history")
	assert.Equal(t, "Answer in Indonesian.", req.Agent.Instructions)
}

func TestRun_DeadlineIsFailedTimeout(t *testing.T) {
	f := newSvc(t)
	f.say(t, "hello")
	r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "cancel")})
	assert.Equal(t, run.StatusFailed, r.Status)
	assert.Equal(t, string(apperror.CodeRunTimedOut), r.ErrorCode)
	assert.Equal(t, int64(120), r.Usage.InputTokens)
}

func TestRun_BusyIsInterruptedNoAttempt(t *testing.T) {
	f := newSvc(t)
	f.say(t, "hello")
	first := f.execute(t, fakes.RuntimeReply{Stream: cut(stream(t, "plain_reply"))})
	require.Equal(t, int32(1), first.Attempt)
	r := f.execute(t, fakes.RuntimeReply{Err: &runtime.BusyError{}})
	assert.Equal(t, run.StatusInterrupted, r.Status)
	assert.Equal(t, int32(1), r.Attempt, "a busy runtime does not use an attempt")
	r = f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	assert.Equal(t, int32(2), r.Attempt)
	assert.Equal(t, run.StatusCompleted, r.Status)
}

func TestRun_CancelWritesEstimatedUsage(t *testing.T) {
	f := newSvc(t)
	f.say(t, "hello")
	f.rt.Replies = append(f.rt.Replies, fakes.RuntimeReply{Stream: cut(stream(t, "plain_reply")), Block: true})
	token := f.claim(t)
	ctx, cancel := context.WithCancelCause(f.ctx)
	cancel(run.ErrCancelRequested)
	r, err := f.svc.Execute(ctx, f.thread, token)
	require.NoError(t, err)
	assert.Equal(t, run.StatusCancelled, r.Status)
	assert.True(t, r.Usage.Estimated)
	assert.Positive(t, r.Usage.InputTokens)
	assert.Positive(t, r.Usage.OutputTokens, "streamed text counts as output")
}

func TestRun_ShutdownInterrupts(t *testing.T) {
	f := newSvc(t)
	f.say(t, "hello")
	f.rt.Replies = append(f.rt.Replies, fakes.RuntimeReply{Stream: cut(stream(t, "plain_reply")), Block: true})
	token := f.claim(t)
	ctx, cancel := context.WithCancel(f.ctx)
	cancel()
	r, err := f.svc.Execute(ctx, f.thread, token)
	require.NoError(t, err)
	assert.Equal(t, run.StatusInterrupted, r.Status)
	assert.True(t, r.Usage.Estimated)
}

func TestRun_ControlCharsInProviderRefused(t *testing.T) {
	for name, c := range map[string]struct {
		mutate func(m *run.ResolvedModel)
		code   apperror.Code
	}{
		"key":      {func(m *run.ResolvedModel) { m.APIKey = "sk-test\r\nX-Evil: 1" }, apperror.CodeModelProviderSecretRequired},
		"base url": {func(m *run.ResolvedModel) { m.BaseURL = "https://llm.example.com/v1\n" }, apperror.CodeModelProviderInvalidBaseURL},
	} {
		t.Run(name, func(t *testing.T) {
			f := newSvc(t)
			m := f.providers.Models[f.provider]["gpt-4o"]
			c.mutate(&m)
			f.providers.Models[f.provider]["gpt-4o"] = m
			f.say(t, "hello")
			r := f.execute(t)
			assert.Equal(t, run.StatusFailed, r.Status)
			assert.Equal(t, string(c.code), r.ErrorCode)
			assert.Empty(t, f.rt.Requests)
		})
	}
}

func TestRun_MaxAttemptsFails(t *testing.T) {
	f := newSvc(t)
	f.say(t, "hello")
	for range 3 {
		r := f.execute(t, fakes.RuntimeReply{Stream: cut(stream(t, "plain_reply"))})
		require.Equal(t, run.StatusInterrupted, r.Status)
	}
	calls := len(f.rt.Requests)
	r := f.execute(t)
	assert.Equal(t, run.StatusFailed, r.Status)
	assert.Equal(t, string(apperror.CodeRunAttemptsExhausted), r.ErrorCode)
	assert.Len(t, f.rt.Requests, calls, "no fourth runtime call")
}

func TestRun_RateLimitedLeavesMessagesPending(t *testing.T) {
	f := newSvc(t)
	f.svc.WithRateCap(run.RateCap{Limiter: memory.New(10), PerMinute: 1})
	f.say(t, "one")
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	f.say(t, "two")
	token := f.claim(t)
	_, err := f.svc.Execute(f.ctx, f.thread, token)
	assert.True(t, run.IsRateLimitedError(err), "%v", err)
	pending, err := f.store.PendingUnleased(f.ctx, time.Now(), 10)
	require.NoError(t, err)
	assert.Empty(t, pending, "still leased")
	require.NoError(t, f.store.Release(f.ctx, f.thread, token))
	pending, err = f.store.PendingUnleased(f.ctx, time.Now(), 10)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{f.thread}, pending, "the message waits for the next tick")
}

func TestRun_LeaseLostEndsQuietly(t *testing.T) {
	f := newSvc(t)
	f.say(t, "record lunch")
	f.rt.Replies = append(f.rt.Replies, fakes.RuntimeReply{Stream: stream(t, "tool_call")})
	token := f.claim(t)
	f.store.Threads[f.thread].LeaseOwner = "someone-else"
	_, err := f.svc.Execute(f.ctx, f.thread, token)
	assert.True(t, run.IsLeaseLostError(err), "%v", err)
}

func TestRun_BuildErrorSettlesInterrupted(t *testing.T) {
	f := newSvc(t)
	delete(f.servers.Servers, f.server)
	f.say(t, "hello")
	r := f.execute(t)
	assert.Equal(t, run.StatusInterrupted, r.Status, "an MCP server that will not open is retried, not lost")
	assert.Empty(t, f.rt.Requests)
	open, err := f.store.OpenRun(f.ctx, f.thread)
	require.NoError(t, err)
	require.NotNil(t, open, "the next turn resumes it")
}

func TestRun_CheckpointErrorSettlesInterrupted(t *testing.T) {
	f := newSvc(t)
	f.store.CheckpointErr = assertErr{}
	f.say(t, "record lunch")
	r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "tool_call")})
	assert.Equal(t, run.StatusInterrupted, r.Status)
	assert.True(t, r.Usage.Estimated)
}

func TestRun_TransientResolveErrorInterrupts(t *testing.T) {
	f := newSvc(t)
	f.providers.Err = assertErr{}
	f.say(t, "hello")
	r := f.execute(t)
	assert.Equal(t, run.StatusInterrupted, r.Status, "only a domain refusal fails the run")
	assert.Empty(t, r.ErrorCode)
}

func TestRun_SettleFailureKeepsTheLease(t *testing.T) {
	f := newSvc(t)
	f.say(t, "hello")
	f.rt.Replies = append(f.rt.Replies, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	f.store.SettleErr = assertErr{}
	token := f.claim(t)
	_, err := f.svc.Execute(f.ctx, f.thread, token)
	assert.True(t, run.IsSettleError(err), "%v", err)
}

func TestRun_VanishedResumeIsNothingToDo(t *testing.T) {
	f := newSvc(t)
	f.say(t, "hello")
	r := f.execute(t, fakes.RuntimeReply{Stream: cut(stream(t, "plain_reply"))})
	require.Equal(t, run.StatusInterrupted, r.Status)
	f.store.ResumeGone = true
	token := f.claim(t)
	got, err := f.svc.Execute(f.ctx, f.thread, token)
	require.NoError(t, err, "a run cancelled under the claim is not an error")
	assert.Nil(t, got)
}

func TestRun_ZeroMaxStepsLeavesTheDefaultToTheRuntime(t *testing.T) {
	f := newSvc(t)
	f.version.MaxSteps = 0
	f.say(t, "hello")
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	assert.Zero(t, f.rt.LastRequest().Agent.MaxSteps, "0 asks for the runtime's own default; one source of truth")
}

func TestRun_NoRuntimeLeavesMessagesPending(t *testing.T) {
	f := newSvc(t)
	var logs bytes.Buffer
	f.svc = run.NewService(f.store, fakes.Transactor{}, apperror.NewReporter(nil, false).Unexpected).
		WithAgents(f.agents).WithProviders(f.providers).WithMCPServers(f.servers).WithSnapshots(f.snaps).
		WithLogger(slog.New(slog.NewTextHandler(&logs, nil)))
	f.say(t, "hello")
	for range 3 {
		token := f.claim(t)
		_, err := f.svc.Execute(f.ctx, f.thread, token)
		assert.Error(t, err)
		require.NoError(t, f.store.Release(f.ctx, f.thread, token))
	}
	assert.Empty(t, f.store.AllRuns(), "no run is created, so nothing is consumed")
	pending, err := f.store.PendingUnleased(f.ctx, time.Now(), 10)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{f.thread}, pending, "the message waits for a runtime")
	assert.Equal(t, 1, strings.Count(logs.String(), "no runtime configured"), "the warning is rate-limited per thread")
}

func TestRun_ContractRefusalIsAPermanentFailure(t *testing.T) {
	f := newSvc(t)
	f.say(t, "hello")
	r := f.execute(t, fakes.RuntimeReply{Err: &runtime.ContractError{Status: 400, Code: "invalid_request"}})
	assert.Equal(t, run.StatusFailed, r.Status)
	assert.Equal(t, string(apperror.CodeRunRuntimeFailed), r.ErrorCode, "a refused contract will not fix itself on retry")
}

func (f *svcFixture) transcript(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, m := range f.store.Messages {
		var am struct {
			Role       string `json:"role"`
			ToolCallID string `json:"toolCallId"`
			ToolCalls  []struct {
				ID string `json:"id"`
			} `json:"toolCalls"`
		}
		require.NoError(t, json.Unmarshal(m.Body, &am))
		entry := am.Role
		for _, c := range am.ToolCalls {
			entry += " call:" + c.ID
		}
		if am.ToolCallID != "" {
			entry += " result:" + am.ToolCallID
		}
		out = append(out, entry)
	}
	return out
}

func TestCheckpoint_StoresTheCallBeforeItsResult(t *testing.T) {
	f := newSvc(t)
	f.say(t, "record lunch")
	f.execute(t, fakes.RuntimeReply{Stream: cut(stream(t, "tool_call"))})
	assert.Equal(t, []string{"user", "assistant call:call_1", "tool result:call_1"}, f.transcript(t),
		"a crash right after the checkpoint still leaves the call before its result")

	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "stored_result")})
	f.say(t, "anything else?")
	f.snaps = &fakes.SnapshotStore{}
	f.svc.WithSnapshots(f.snaps)
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	history := f.rt.LastRequest().History
	require.GreaterOrEqual(t, len(history), 3)
	assert.Contains(t, string(history[1]), `"toolCalls"`)
	assert.Contains(t, string(history[2]), `"toolCallId":"call_1"`, "a rebuilt history never has a result before its call")
}

func TestSettle_ReplayedToolRoundIsStoredOnce(t *testing.T) {
	f := newSvc(t)
	f.say(t, "record lunch")
	f.execute(t, fakes.RuntimeReply{Stream: cut(stream(t, "tool_call"))})
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "stored_result")})
	assert.Equal(t, []string{"user", "assistant call:call_1", "tool result:call_1", "assistant"}, f.transcript(t),
		"the replayed call and result are not written a second time")
}

func TestRun_CachedTokensAreChargedOnce(t *testing.T) {
	f := newSvc(t)
	raw := strings.Replace(string(stream(t, "plain_reply")),
		`"usage": {"inputTokens": 120, "outputTokens": 9, "cacheReadInputTokens": 0`,
		`"usage": {"inputTokens": 120, "outputTokens": 9, "cacheReadInputTokens": 100`, 1)
	if !strings.Contains(raw, `"cacheReadInputTokens": 100`) {
		raw = strings.Replace(string(stream(t, "plain_reply")), `"cacheReadInputTokens":0`, `"cacheReadInputTokens":100`, 1)
	}
	require.Contains(t, strings.ReplaceAll(raw, " ", ""), `"cacheReadInputTokens":100`)
	f.say(t, "hello")
	r := f.execute(t, fakes.RuntimeReply{Stream: []byte(raw)})
	assert.Equal(t, int64(20), r.Usage.InputTokens, "prompt tokens include the cached ones; only the rest is billed as input")
	assert.Equal(t, int64(100), r.Usage.CacheReadTokens)
}

func TestRun_GatewayErrorsAreTransient(t *testing.T) {
	for _, status := range []int{502, 504} {
		f := newSvc(t)
		f.say(t, "hello")
		r := f.execute(t, fakes.RuntimeReply{Err: &runtime.StatusError{Status: status}})
		assert.Equal(t, run.StatusInterrupted, r.Status, "%d is retried", status)
		pending, err := f.store.PendingUnleased(f.ctx, time.Now(), 10)
		require.NoError(t, err)
		assert.Equal(t, []uuid.UUID{f.thread}, pending, "%d: the interrupted run is picked up by reconcile", status)
	}
}

func TestRun_RateCapChargedOnlyWhenARunStarts(t *testing.T) {
	f := newSvc(t)
	f.svc.WithRateCap(run.RateCap{Limiter: memory.New(10), PerMinute: 1})
	for range 3 {
		token := f.claim(t)
		got, err := f.svc.Execute(f.ctx, f.thread, token)
		require.NoError(t, err, "an empty pass costs nothing")
		assert.Nil(t, got)
		require.NoError(t, f.store.Release(f.ctx, f.thread, token))
	}
	f.say(t, "hello")
	r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	assert.Equal(t, run.StatusCompleted, r.Status, "the one token is still there for the real run")
}

func TestRun_UnpublishedAgentIsSkippedQuietly(t *testing.T) {
	f := newSvc(t)
	f.agents.Versions = map[uuid.UUID]*run.AgentVersion{}
	reported := 0
	f.svc = run.NewService(f.store, fakes.Transactor{}, func(context.Context, string, error, ...any) *apperror.AppError {
		reported++
		return nil
	}).WithRuntime(f.rt).WithAgents(f.agents).WithProviders(f.providers).WithMCPServers(f.servers).WithSnapshots(f.snaps)
	f.say(t, "hello")
	token := f.claim(t)
	got, err := f.svc.Execute(f.ctx, f.thread, token)
	require.NoError(t, err)
	assert.Nil(t, got)
	assert.Zero(t, reported, "no error report on every reconcile tick")
	assert.Empty(t, f.store.AllRuns())
}

func TestCancel_UnderTheProjectCapStillEmits(t *testing.T) {
	f := newSvc(t)
	pub := &fakes.Publisher{}
	f.svc.WithPublisher(pub)
	f.say(t, "hello")
	r := f.execute(t, fakes.RuntimeReply{Stream: cut(stream(t, "plain_reply"))})
	require.Equal(t, run.StatusInterrupted, r.Status)
	f.store.Threads[f.thread].LeaseOwner, f.store.Threads[f.thread].LeaseUntil = "busy", time.Now().Add(-time.Second)
	f.store.ClaimRefused = true
	require.NoError(t, f.svc.Cancel(f.ctx, f.thread))
	assert.Equal(t, run.StatusInterrupted, f.store.AllRuns()[0].Status, "the store never settles a cancel on its own")

	f.store.ClaimRefused = false
	f.store.Threads[f.thread].LeaseOwner = ""
	calls := f.rt.Calls()
	got := f.execute(t)
	assert.Equal(t, run.StatusCancelled, got.Status, "the next claim settles the requested cancel")
	require.Len(t, pub.Events(), 1)
	assert.Equal(t, events.RunFailed, pub.Events()[0].Type)
	assert.Equal(t, calls, f.rt.Calls(), "a cancelled run never reaches the runtime")
}

func TestResume_AfterACrashSendsTheToolRoundNotTheAskAgain(t *testing.T) {
	f := newSvc(t)
	f.priorTurn(t)
	f.say(t, "record lunch")
	f.execute(t, fakes.RuntimeReply{Stream: cut(stream(t, "tool_call"))})
	r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "resume_after_crash")})
	assert.Equal(t, run.StatusCompleted, r.Status)
	assert.Equal(t, int32(2), r.Attempt)

	req := f.rt.LastRequest()
	assert.Equal(t, runtime.InputContinue, req.Input.Kind, "nothing new is asked; the model continues after the tool round")
	assert.Empty(t, req.Input.Message)
	assert.Nil(t, req.Snapshot, "the snapshot predates the tool round, so history carries it")
	var roles []string
	for _, h := range req.History {
		var m struct {
			Role       string `json:"role"`
			ToolCallID string `json:"toolCallId"`
		}
		require.NoError(t, json.Unmarshal(h, &m))
		roles = append(roles, m.Role+m.ToolCallID)
	}
	assert.Equal(t, []string{"user", "assistant", "user", "assistant", "toolcall_1"}, roles,
		"the earlier turn, this run's ask, its call and its result, in order")
	require.Len(t, req.StoredResults, 1)
	assert.Equal(t, "call_1", req.StoredResults[0].ToolUseID)
	assert.Equal(t, []string{"user", "assistant", "user", "assistant call:call_1", "tool result:call_1", "assistant"}, f.transcript(t))
}
