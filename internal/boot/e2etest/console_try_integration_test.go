//go:build integration

package e2etest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/boot"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/testutil/runtimetest"
	"altalune.id/oagent/internal/user"
)

func TestConsoleTry_RunsTheDraftOnTheRuntime(t *testing.T) {
	model := runtimetest.NewFakeModel(t,
		runtimetest.Reply{Chunks: []string{"First ", "draft."}, InputTokens: 40, OutputTokens: 3},
		runtimetest.Reply{Chunks: []string{"Edited."}, InputTokens: 41, OutputTokens: 2},
	)
	s := newDPStack(t, runtimetest.StartRuntime(t, model))
	u, err := s.svcs.Users.Create(t.Context(), user.CreateRequest{Email: "tryer@example.com", Name: "Tryer", Password: "quiet-forest-lamp-9"})
	require.NoError(t, err)
	who := session.Principal{
		UserID: u.ID, Source: session.SourceLocal, ActiveOrgID: s.org, Scopes: member.BuiltinPermissions(member.RoleMember),
	}
	mux := http.NewServeMux()
	for pattern, route := range boot.BuildConsoleTry(s.cfg, s.svcs, s.lims, s.k.Reporter.Unexpected, testLogger()) {
		method, path, _ := strings.Cut(pattern, " ")
		mux.Handle(method+" /console"+path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			route.ServeHTTP(w, r.WithContext(session.PrincipalInto(r.Context(), who)))
		}))
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	try := func(thread, instructions string) []sseEvent {
		t.Helper()
		_, err := s.svcs.Agents.UpdateDraft(s.in, s.agentID, agent.Config{
			Instructions: instructions, Model: runtimetest.ModelID, ProviderID: s.provider,
		})
		require.NoError(t, err)
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/console/agents/"+s.agentPub+"/try/runs",
			strings.NewReader(`{"threadId":"`+thread+`","messages":[{"id":"`+uuid.NewString()+`","role":"user","content":"hello"}]}`))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		return readSSE(t, resp.Body)
	}

	first := try("try-1", "DRAFT-ONE: never published.")
	require.NotEmpty(t, first)
	runID := first[0].RunID
	require.Eventually(t, func() bool {
		r, err := s.svcs.RunStore.RunByPublicID(s.in, runID)
		return err == nil && r.Status.Terminal()
	}, 15*time.Second, 50*time.Millisecond)
	require.Len(t, model.Requests(), 1)
	assert.Contains(t, string(model.Requests()[0]), "DRAFT-ONE", "the run used the unpublished draft")
	assert.NotContains(t, string(model.Requests()[0]), "Be brief.", "not the published version")

	r, err := s.svcs.RunStore.RunByPublicID(s.in, runID)
	require.NoError(t, err)
	pinned, err := s.svcs.AgentStore.VersionByID(s.in, r.AgentVersionID)
	require.NoError(t, err)
	assert.Equal(t, agent.VersionDraft, pinned.Kind)
	assert.Equal(t, int64(40), r.Usage.InputTokens, "the usage is recorded like any run's")
	th, err := s.svcs.RunStore.Thread(s.in, r.ThreadID)
	require.NoError(t, err)
	assert.Equal(t, "try", th.Kind)
	latest, err := s.svcs.AgentStore.LatestVersion(s.in, s.agentID)
	require.NoError(t, err)
	assert.Equal(t, int32(1), latest.Version, "trying a draft never publishes it")

	second := try("try-2", "DRAFT-TWO: edited.")
	require.NotEmpty(t, second)
	require.Eventually(t, func() bool { return len(model.Requests()) == 2 }, 15*time.Second, 50*time.Millisecond)
	assert.Contains(t, string(model.Requests()[1]), "DRAFT-TWO", "an edited draft is what the next run uses")
}
