//go:build integration

package e2etest

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/boot"
	"altalune.id/oagent/internal/mcpserver"
	"altalune.id/oagent/internal/testutil/runtimetest"
)

const (
	aprServer = "ledger"
	aprTool   = "record"
	// NOTE: unknownText is the tool result the model gets for a call whose outcome a crash lost; run.callUnknownText.
	unknownText = "the outcome of this call is unknown after a restart"
)

type aprEvent struct {
	Type    string `json:"type"`
	RunID   string `json:"runId"`
	Code    string `json:"code"`
	Outcome *struct {
		Type       string `json:"type"`
		Interrupts []struct {
			ID string `json:"id"`
		} `json:"interrupts"`
	} `json:"outcome"`
}

func readAprSSE(t *testing.T, body io.Reader) []aprEvent {
	t.Helper()
	var out []aprEvent
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		data, ok := strings.CutPrefix(sc.Text(), "data: ")
		if !ok {
			continue
		}
		var e aprEvent
		require.NoError(t, json.Unmarshal([]byte(data), &e))
		out = append(out, e)
	}
	return out
}

func gate(in context.Context, t *testing.T, svcs *boot.Services, agentID, provider uuid.UUID, mcpURL string) {
	t.Helper()
	a, err := svcs.AgentStore.ByID(in, agentID)
	require.NoError(t, err)
	srv, err := svcs.MCPServers.Create(in, a.ProjectID, mcpserver.Input{
		Name: aprServer, URL: mcpURL, AuthKind: mcpserver.AuthNone, TimeoutMS: 10_000,
	}, "")
	require.NoError(t, err)
	latest, err := svcs.AgentStore.LatestVersion(in, agentID)
	require.NoError(t, err)
	require.NoError(t, a.SetDraft(agent.Config{
		Instructions: "Be brief.", Model: runtimetest.ModelID, ProviderID: provider,
		MCPServers: []agent.MCPGrant{{ServerID: srv.ID, Tools: []agent.Tool{{Name: aprTool, RequireApproval: true}}}},
	}, time.Now()))
	v, created, err := a.Publish(latest, agentID, time.Now().UTC())
	require.NoError(t, err)
	require.True(t, created)
	require.NoError(t, errors.Join(svcs.AgentStore.InsertVersion(in, v), svcs.AgentStore.MarkPublished(in, a), svcs.AgentStore.SaveDraft(in, a)))
}

func (s *dpStack) gate(t *testing.T, rt *runtimetest.Runtime, mcp *runtimetest.FakeMCP) {
	t.Helper()
	gate(s.in, t, s.svcs, s.agentID, s.provider, rt.HostURL(mcp.Port(), "/mcp"))
}

func interrupted(t *testing.T, s *dpStack, thread, text string) (string, string) {
	t.Helper()
	resp := s.post(t.Context(), t, thread, text)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	events := readAprSSE(t, resp.Body)
	require.NotEmpty(t, events)
	last := events[len(events)-1]
	require.Equal(t, "RUN_FINISHED", last.Type)
	require.NotNil(t, last.Outcome, "the stream ends with an interrupt outcome")
	require.Equal(t, "interrupt", last.Outcome.Type)
	require.Len(t, last.Outcome.Interrupts, 1)
	return last.RunID, last.Outcome.Interrupts[0].ID
}

func (s *dpStack) resume(t *testing.T, thread, entry string) *http.Response {
	t.Helper()
	return s.do(t.Context(), t, http.MethodPost, "/runs", `{"threadId":"`+thread+`","messages":[],"resume":[`+entry+`]}`)
}

func approved(id string) string {
	return `{"interruptId":"` + id + `","status":"resolved","payload":{"approved":true}}`
}

func (s *chStack) gate(t *testing.T, rt *runtimetest.Runtime, mcp *runtimetest.FakeMCP) {
	t.Helper()
	gate(s.in, t, s.svcs, s.agentID, s.provider, rt.HostURL(mcp.Port(), "/mcp"))
}

type aprRow struct{ publicID, status, text string }

func (s *chStack) approval(t *testing.T) aprRow {
	t.Helper()
	var r aprRow
	require.NoError(t, s.admin.QueryRowContext(t.Context(),
		`SELECT public_id, status, coalesce(decision_text, '') FROM `+s.sch+`.approvals`).Scan(&r.publicID, &r.status, &r.text))
	return r
}

// NOTE: prompted delivers the request and waits for the approval prompt, the only send while the run waits.
func (s *chStack) prompted(t *testing.T, wa *fakeOpenWA, mcp *runtimetest.FakeMCP) aprRow {
	t.Helper()
	s.deliver(t, fixture(t, "text", "msg_AprAsk000000001", "catat 20000", time.Now()))
	require.Eventually(t, func() bool { return len(wa.sent()) == 1 }, time.Minute, 100*time.Millisecond, "the approval prompt")
	row := s.approval(t)
	prompt := wa.sent()[0]
	assert.Equal(t, "approval:"+row.publicID, prompt.Key, "the prompt's idempotency key")
	assert.Equal(t, "628111222333", prompt.To, "the prompt goes to the requester")
	assert.Equal(t, "pending", row.status)
	assert.Equal(t, 0, mcp.Calls(), "nothing runs before the answer")
	return row
}

// NOTE: callStates reads the run's approval status and its calls' states, in call order.
func callStates(t *testing.T, admin *sql.DB, sch, runPublicID string) (string, []string) {
	t.Helper()
	var status string
	var raw []byte
	require.NoError(t, admin.QueryRowContext(t.Context(), `SELECT a.status, a.calls FROM `+sch+`.approvals a
		JOIN `+sch+`.runs r ON r.id = a.run_id WHERE r.public_id = $1`, runPublicID).Scan(&status, &raw))
	var calls []struct {
		State string `json:"state"`
	}
	require.NoError(t, json.Unmarshal(raw, &calls))
	states := make([]string, len(calls))
	for i, c := range calls {
		states[i] = c.State
	}
	return status, states
}

func modelSaw(model *runtimetest.FakeModel, text string) bool {
	for _, b := range model.Requests() {
		if strings.Contains(string(b), text) {
			return true
		}
	}
	return false
}

func TestApproval_AgainstRuntimeContainer(t *testing.T) {
	prefixed := aprServer + "__" + aprTool

	t.Run("an approved call runs exactly once", func(t *testing.T) {
		model := runtimetest.NewFakeModel(t,
			runtimetest.Reply{ToolUnlessResult: prefixed, InputTokens: 40, OutputTokens: 5},
			runtimetest.Reply{ToolUnlessResult: prefixed, Chunks: []string{"Recorded."}, InputTokens: 60, OutputTokens: 2},
		)
		mcp := runtimetest.NewFakeMCP(t, aprTool)
		rt := runtimetest.StartRuntime(t, model, mcp)
		s := newDPStack(t, rt)
		s.gate(t, rt, mcp)

		runID, id := interrupted(t, s, "apr-1", "record it")
		assert.Equal(t, 0, mcp.Calls(), "nothing runs before the approval")
		assert.Equal(t, "awaiting_approval", s.runStatus(t, runID))

		resp := s.resume(t, "apr-1", approved(id))
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		events := readAprSSE(t, resp.Body)
		require.NotEmpty(t, events)
		assert.Equal(t, runID, events[0].RunID, "the resume streams the same run")
		assert.Equal(t, "RUN_FINISHED", events[len(events)-1].Type)
		require.Eventually(t, func() bool { return s.runStatus(t, runID) == "completed" }, 20*time.Second, 100*time.Millisecond)
		assert.Equal(t, 1, mcp.Calls(), "the approved call ran once")

		status, states := callStates(t, s.admin, s.sch, runID)
		assert.Equal(t, "approved", status)
		assert.Equal(t, []string{"executed"}, states)

		again := s.resume(t, "apr-1", approved(id))
		defer func() { _ = again.Body.Close() }()
		assert.Equal(t, http.StatusNotFound, again.StatusCode, "a second answer finds nothing pending")
		var env struct {
			Code string `json:"code"`
		}
		require.NoError(t, json.NewDecoder(again.Body).Decode(&env))
		assert.Equal(t, "APR002", env.Code)
		assert.Equal(t, 1, mcp.Calls(), "a repeated answer never runs the call again")
		assert.Equal(t, 2, model.Calls())
	})

	t.Run("a rejected call never runs", func(t *testing.T) {
		model := runtimetest.NewFakeModel(t,
			runtimetest.Reply{ToolUnlessResult: prefixed, InputTokens: 40, OutputTokens: 5},
			runtimetest.Reply{ToolUnlessResult: prefixed, Chunks: []string{"Not recorded."}, InputTokens: 60, OutputTokens: 2},
		)
		mcp := runtimetest.NewFakeMCP(t, aprTool)
		rt := runtimetest.StartRuntime(t, model, mcp)
		s := newDPStack(t, rt)
		s.gate(t, rt, mcp)

		runID, id := interrupted(t, s, "apr-2", "record it")
		resp := s.resume(t, "apr-2", `{"interruptId":"`+id+`","status":"cancelled"}`)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		_ = readAprSSE(t, resp.Body)
		require.Eventually(t, func() bool { return s.runStatus(t, runID) == "completed" }, 20*time.Second, 100*time.Millisecond)
		assert.Equal(t, 0, mcp.Calls(), "a rejected call never runs")
		assert.Equal(t, 2, model.Calls(), "the model hears the rejection and answers")
	})

	t.Run("a crash during an approved call leaves it unknown, never run again", func(t *testing.T) {
		model := runtimetest.NewFakeModel(t,
			runtimetest.Reply{ToolUnlessResult: prefixed, InputTokens: 40, OutputTokens: 5},
			runtimetest.Reply{ToolUnlessResult: prefixed, Chunks: []string{"I could not confirm it."}, InputTokens: 60, OutputTokens: 2},
			runtimetest.Reply{ToolUnlessResult: prefixed, Chunks: []string{"Yes."}, InputTokens: 70, OutputTokens: 1},
		)
		mcp := runtimetest.NewFakeMCP(t, aprTool)
		rt := runtimetest.StartRuntime(t, model, mcp)
		s := newDPStack(t, rt)
		s.gate(t, rt, mcp)

		runID, id := interrupted(t, s, "apr-3", "record it")
		mcp.HoldCalls()
		resp := s.resume(t, "apr-3", approved(id))
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		select {
		case <-mcp.Held():
		case <-time.After(time.Minute):
			t.Fatal("the approved call never reached the MCP server")
		}
		rt.Kill()
		events := readAprSSE(t, resp.Body)
		require.NotEmpty(t, events)
		assert.Equal(t, "interrupted", events[len(events)-1].Code)
		assert.Equal(t, "interrupted", s.runStatus(t, runID))

		rt.Restart()
		next := s.post(t.Context(), t, "apr-3", "are you there")
		defer func() { _ = next.Body.Close() }()
		require.Equal(t, http.StatusOK, next.StatusCode)
		_ = readAprSSE(t, next.Body)
		require.Eventually(t, func() bool { return s.runStatus(t, runID) == "completed" }, 20*time.Second, 100*time.Millisecond)
		assert.Equal(t, 1, mcp.Calls(), "the call whose outcome was lost never runs again")
		assert.True(t, modelSaw(model, unknownText), "the model is told the outcome is unknown")
		status, states := callStates(t, s.admin, s.sch, runID)
		assert.Equal(t, "approved", status)
		assert.Equal(t, []string{"unknown"}, states)
	})

	t.Run("whatsapp: the requester's ya runs the call exactly once", func(t *testing.T) {
		model := runtimetest.NewFakeModel(t,
			runtimetest.Reply{ToolUnlessResult: prefixed, InputTokens: 40, OutputTokens: 5},
			runtimetest.Reply{ToolUnlessResult: prefixed, Chunks: []string{"Tercatat."}, InputTokens: 60, OutputTokens: 2},
		)
		mcp := runtimetest.NewFakeMCP(t, aprTool)
		rt := runtimetest.StartRuntime(t, model, mcp)
		wa := newFakeOpenWA(t)
		s := newChannelStack(t, rt, wa)
		s.gate(t, rt, mcp)

		s.prompted(t, wa, mcp)
		s.deliver(t, fixture(t, "text", "msg_AprYes000000001", "ya", time.Now()))
		require.Eventually(t, func() bool { return len(wa.sent()) == 2 }, time.Minute, 100*time.Millisecond, "the reply after the call")
		require.Never(t, func() bool { return mcp.Calls() != 1 || len(wa.sent()) != 2 }, time.Second, 50*time.Millisecond,
			"the approved call ran exactly once, with the prompt and one reply")
		assert.Equal(t, "Tercatat.", wa.sent()[1].Text)
		assert.Equal(t, "approved", s.approval(t).status)
		assert.Equal(t, 2, model.Calls(), "the ya answers the approval; it is not a new turn")
	})

	t.Run("whatsapp: tidak with a reason reaches the model, the call never runs", func(t *testing.T) {
		model := runtimetest.NewFakeModel(t,
			runtimetest.Reply{ToolUnlessResult: prefixed, InputTokens: 40, OutputTokens: 5},
			runtimetest.Reply{ToolUnlessResult: prefixed, Chunks: []string{"Baik, tidak dicatat."}, InputTokens: 60, OutputTokens: 2},
		)
		mcp := runtimetest.NewFakeMCP(t, aprTool)
		rt := runtimetest.StartRuntime(t, model, mcp)
		wa := newFakeOpenWA(t)
		s := newChannelStack(t, rt, wa)
		s.gate(t, rt, mcp)

		s.prompted(t, wa, mcp)
		s.deliver(t, fixture(t, "text", "msg_AprNo0000000001", "tidak, 20000", time.Now()))
		require.Eventually(t, func() bool { return len(wa.sent()) == 2 }, time.Minute, 100*time.Millisecond, "the reply after the denial")
		require.Never(t, func() bool { return mcp.Calls() != 0 || len(wa.sent()) != 2 }, time.Second, 50*time.Millisecond,
			"a denied call never runs, and only the prompt and one reply go out")
		assert.Equal(t, "Baik, tidak dicatat.", wa.sent()[1].Text)
		row := s.approval(t)
		assert.Equal(t, "rejected", row.status)
		assert.Equal(t, "tidak, 20000", row.text)
		assert.True(t, modelSaw(model, "tidak, 20000"), "the model reads the denial text")
	})
}
