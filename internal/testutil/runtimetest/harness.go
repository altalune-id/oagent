//go:build integration

package runtimetest

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	scenarioTimeout = 2 * time.Minute
	checkpointWait  = 10 * time.Second
)

// Env is what a driver needs to point a run at the harness's runtime and fake model; MCPURL is empty when the scenario has no MCP server.
type Env struct {
	RuntimeURL, RuntimeToken string
	ModelBaseURL, ModelID    string
	ModelKey                 string
	MCPURL                   string
	MCPServerName            string
	MCPToolName              string
}

// Usage is a run's token usage by class.
type Usage struct {
	InputTokens, OutputTokens, CacheReadInputTokens, CacheWriteInputTokens int64
}

// Seeded is the thread a driver prepared with one pending user message.
type Seeded struct {
	ThreadID uuid.UUID
}

// Outcome is what one Execute returned.
type Outcome struct {
	RunID   uuid.UUID
	Status  string
	Attempt int
}

// State is a thread as read back from the driver's store.
type State struct {
	RunID         uuid.UUID
	RunStatus     string
	Attempt       int
	Roles         []string
	SnapshotSaved bool
	Usage         Usage
}

// Driver is the seam the run domain implements for the acceptance scenarios.
type Driver interface {
	Seed(ctx context.Context, env Env, text string) (Seeded, error)
	Execute(ctx context.Context, threadID uuid.UUID, token string) (Outcome, error)
	State(ctx context.Context, threadID uuid.UUID) (State, error)
}

func (u Usage) plus(v Usage) Usage {
	return Usage{
		InputTokens: u.InputTokens + v.InputTokens, OutputTokens: u.OutputTokens + v.OutputTokens,
		CacheReadInputTokens:  u.CacheReadInputTokens + v.CacheReadInputTokens,
		CacheWriteInputTokens: u.CacheWriteInputTokens + v.CacheWriteInputTokens,
	}
}

func envFor(rt *Runtime) Env {
	return Env{RuntimeURL: rt.URL(), RuntimeToken: rt.Token(), ModelBaseURL: rt.ModelBaseURL(), ModelID: ModelID, ModelKey: "sk-fake"}
}

// Run runs the happy path, the crash and the orphan scenarios against the drivers newDriver builds.
func Run(t *testing.T, newDriver func(t *testing.T, env Env) Driver) {
	t.Run("happy path", func(t *testing.T) {
		model := NewFakeModel(t, Reply{Chunks: []string{"Hi, ", "how can I help?"}, InputTokens: 120, OutputTokens: 9})
		rt := StartRuntime(t, model)
		env := envFor(rt)
		d := newDriver(t, env)
		ctx, cancel := context.WithTimeout(t.Context(), scenarioTimeout)
		defer cancel()

		seeded, err := d.Seed(ctx, env, "hello")
		require.NoError(t, err)
		out, err := d.Execute(ctx, seeded.ThreadID, "lease-1")
		require.NoError(t, err)
		assert.Equal(t, "completed", out.Status)

		st, err := d.State(ctx, seeded.ThreadID)
		require.NoError(t, err)
		assert.Equal(t, out.RunID, st.RunID)
		assert.Equal(t, "completed", st.RunStatus)
		assert.Equal(t, []string{"user", "assistant"}, st.Roles)
		assert.True(t, st.SnapshotSaved)
		assert.Equal(t, Usage{InputTokens: 120, OutputTokens: 9}, st.Usage)
		assert.Equal(t, 1, model.Calls())
	})

	t.Run("crash mid-turn resumes the same run", func(t *testing.T) {
		model := NewFakeModel(t, Reply{Hang: true}, Reply{Chunks: []string{"Back."}, InputTokens: 50, OutputTokens: 2})
		rt := StartRuntime(t, model)
		env := envFor(rt)
		d := newDriver(t, env)
		ctx, cancel := context.WithTimeout(t.Context(), scenarioTimeout)
		defer cancel()

		seeded, err := d.Seed(ctx, env, "hello")
		require.NoError(t, err)
		first := make(chan Outcome, 1)
		go func() {
			out, _ := d.Execute(ctx, seeded.ThreadID, "lease-1")
			first <- out
		}()
		select {
		case <-model.Hung():
		case <-ctx.Done():
			t.Fatal("runtimetest: the model never got the turn")
		}
		rt.Kill()
		<-first
		st, err := d.State(ctx, seeded.ThreadID)
		require.NoError(t, err)
		assert.Equal(t, "interrupted", st.RunStatus)
		crashed, interruptedUsage := st.RunID, st.Usage

		rt.Restart()
		out, err := d.Execute(ctx, seeded.ThreadID, "lease-2")
		require.NoError(t, err)
		assert.Equal(t, crashed, out.RunID)
		assert.Equal(t, 2, out.Attempt)
		assert.Equal(t, "completed", out.Status)
		st, err = d.State(ctx, seeded.ThreadID)
		require.NoError(t, err)
		assert.Equal(t, []string{"user", "assistant"}, st.Roles)
		assert.True(t, st.SnapshotSaved)
		final := Usage{InputTokens: 50, OutputTokens: 2}
		assert.Equal(t, interruptedUsage.plus(final), st.Usage, "interrupted attempt plus the final one, counted once")
		assert.Equal(t, 2, model.Calls())
	})

	t.Run("an error after the run starts leaves no running orphan", func(t *testing.T) {
		model := NewFakeModel(t, Reply{Chunks: []string{"never"}, InputTokens: 1, OutputTokens: 1})
		rt := StartRuntime(t, model)
		env := envFor(rt)
		d := newDriver(t, env)
		ctx, cancel := context.WithTimeout(t.Context(), scenarioTimeout)
		defer cancel()

		seeded, err := d.Seed(ctx, env, "hello")
		require.NoError(t, err)
		rt.Kill()
		_, _ = d.Execute(ctx, seeded.ThreadID, "lease-1")
		st, err := d.State(ctx, seeded.ThreadID)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, st.RunID, "the run was started")
		assert.NotEqual(t, "running", st.RunStatus)
		assert.Equal(t, 0, model.Calls())
	})

	t.Run("a crash after the tool ran resumes without running it again", func(t *testing.T) {
		const server, tool = "ledger", "record"
		prefixed := server + "__" + tool
		model := NewFakeModel(t,
			Reply{ToolUnlessResult: prefixed, InputTokens: 40, OutputTokens: 5},
			Reply{Hang: true},
			Reply{ToolUnlessResult: prefixed, Chunks: []string{"Recorded."}, InputTokens: 60, OutputTokens: 2},
		)
		mcp := NewFakeMCP(t, tool)
		rt := StartRuntime(t, model, mcp)
		env := envFor(rt)
		env.MCPURL, env.MCPServerName, env.MCPToolName = rt.HostURL(mcp.Port(), "/mcp"), server, tool
		d := newDriver(t, env)
		ctx, cancel := context.WithTimeout(t.Context(), scenarioTimeout)
		defer cancel()

		seeded, err := d.Seed(ctx, env, "record it")
		require.NoError(t, err)
		first := make(chan Outcome, 1)
		go func() {
			out, _ := d.Execute(ctx, seeded.ThreadID, "lease-1")
			first <- out
		}()
		select {
		case <-model.Hung():
		case <-ctx.Done():
			t.Fatal("runtimetest: the model never got the second call")
		}
		require.Equal(t, 1, mcp.Calls(), "the tool ran once before the crash")
		require.Eventually(t, func() bool {
			st, err := d.State(ctx, seeded.ThreadID)
			return err == nil && slices.Contains(st.Roles, "tool")
		}, checkpointWait, 20*time.Millisecond, "the tool result was never checkpointed")
		rt.Kill()
		<-first
		st, err := d.State(ctx, seeded.ThreadID)
		require.NoError(t, err)
		assert.Equal(t, "interrupted", st.RunStatus)

		rt.Restart()
		out, err := d.Execute(ctx, seeded.ThreadID, "lease-2")
		require.NoError(t, err)
		assert.Equal(t, st.RunID, out.RunID)
		assert.Equal(t, "completed", out.Status)
		assert.Equal(t, 1, mcp.Calls(), "the resumed run must not run the tool again")
		assert.Equal(t, 3, model.Calls())
	})
}
