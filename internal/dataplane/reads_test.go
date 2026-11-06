package dataplane_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/dataplane"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/testutil/fakes"
)

func (p *plane) call(t *testing.T, method, key, path string, headers ...string) result {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, p.srv.URL+path, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+key)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := p.srv.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return result{StatusCode: resp.StatusCode, Header: resp.Header, Body: string(raw)}
}

func runPath(id string) string {
	return "/api/v1/agents/AGENTpub/runs/" + id
}

type runJSON struct {
	ID         string `json:"id"`
	ThreadID   string `json:"thread_id"`
	Status     string `json:"status"`
	CostMicros *int64 `json:"cost_micros"`
	Usage      struct {
		InputTokens int64 `json:"input_tokens"`
	} `json:"usage"`
}

func (p *plane) settledTurn(t *testing.T, key string, headers ...string) *run.Run {
	t.Helper()
	p.rt.Replies = append(p.rt.Replies, fakes.RuntimeReply{Stream: runtimeStream(t, "plain_reply")})
	resp := p.post(t, key, "AGENTpub", body(clientThread, "hello"), headers...)
	require.Equal(t, http.StatusOK, resp.StatusCode, resp.Body)
	return p.settled(t)
}

func TestGETRun_OtherActorIs404(t *testing.T) {
	p := newPlane(t, 4)
	r := p.settledTurn(t, orgKey, dataplane.UserHeader, "alice")
	mine := p.call(t, http.MethodGet, orgKey, runPath(r.PublicID), dataplane.UserHeader, "alice")
	require.Equal(t, http.StatusOK, mine.StatusCode, mine.Body)
	var view runJSON
	require.NoError(t, json.Unmarshal([]byte(mine.Body), &view))
	assert.Equal(t, r.PublicID, view.ID)
	assert.Equal(t, clientThread, view.ThreadID, "the client's own thread key, never an internal id")
	assert.Equal(t, "completed", view.Status)
	assert.Positive(t, view.Usage.InputTokens)
	assert.Nil(t, view.CostMicros, "an unpriced run has no cost")

	missing := p.call(t, http.MethodGet, orgKey, runPath("nope"), dataplane.UserHeader, "alice")
	require.Equal(t, http.StatusNotFound, missing.StatusCode)
	missingCode, _ := errorCode(t, missing)
	assert.Equal(t, apperror.CodeRunNotFound, missingCode)
	for name, c := range map[string]struct {
		key     string
		headers []string
	}{
		"another actor":    {orgKey, []string{dataplane.UserHeader, "bob"}},
		"the key as actor": {orgKey, nil},
	} {
		resp := p.call(t, http.MethodGet, c.key, runPath(r.PublicID), c.headers...)
		require.Equal(t, http.StatusNotFound, resp.StatusCode, name)
		code, _ := errorCode(t, resp)
		assert.Equal(t, missingCode, code, "%s: reads exactly like a missing run", name)
	}
	foreign := p.call(t, http.MethodGet, otherKey, runPath(r.PublicID))
	require.Equal(t, http.StatusNotFound, foreign.StatusCode)
	foreignCode, _ := errorCode(t, foreign)
	assert.Equal(t, apperror.CodeAgentNotFound, foreignCode, "a key of another project cannot reach the agent")
}

func TestGETMessages_CursorBySeq(t *testing.T) {
	p := newPlane(t, 4)
	p.settledTurn(t, projectKey)
	path := "/api/v1/agents/AGENTpub/threads/" + clientThread + "/messages"
	type page struct {
		Messages []struct {
			Seq     int64           `json:"seq"`
			Status  string          `json:"status"`
			Message json.RawMessage `json:"message"`
		} `json:"messages"`
		Next int64 `json:"next_after"`
	}
	first := p.call(t, http.MethodGet, projectKey, path+"?limit=1")
	require.Equal(t, http.StatusOK, first.StatusCode, first.Body)
	var got page
	require.NoError(t, json.Unmarshal([]byte(first.Body), &got))
	require.Len(t, got.Messages, 1)
	assert.Equal(t, int64(1), got.Messages[0].Seq)
	assert.JSONEq(t, `{"id":"m1","role":"user","content":"hello"}`, string(got.Messages[0].Message))
	assert.Equal(t, int64(1), got.Next)

	rest := p.call(t, http.MethodGet, projectKey, path+"?after=1")
	require.Equal(t, http.StatusOK, rest.StatusCode, rest.Body)
	var more page
	require.NoError(t, json.Unmarshal([]byte(rest.Body), &more))
	require.NotEmpty(t, more.Messages)
	assert.Greater(t, more.Messages[0].Seq, int64(1))
	assert.Contains(t, string(more.Messages[0].Message), `"role":"assistant"`)

	assert.Equal(t, http.StatusNotFound, p.call(t, http.MethodGet, projectKey, "/api/v1/agents/AGENTpub/threads/unknown/messages").StatusCode)
	assert.Equal(t, http.StatusBadRequest, p.call(t, http.MethodGet, projectKey, path+"?limit=9999").StatusCode)
	assert.Equal(t, http.StatusNotFound, p.call(t, http.MethodGet, orgKey, path).StatusCode, "another actor's thread is not found")
}

func TestCancel_LiveStreamEndsCancelled(t *testing.T) {
	p := newPlane(t, 4)
	p.rt.Replies = []fakes.RuntimeReply{{Stream: cut(runtimeStream(t, "plain_reply")), Block: true}}
	ctx, stop := context.WithTimeout(t.Context(), 10*time.Second)
	defer stop()
	resp, err := p.srv.Client().Do(p.request(t, ctx, projectKey, "AGENTpub", body(clientThread, "hello")))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Eventually(t, func() bool { return len(p.store.AllRuns()) == 1 }, 5*time.Second, 10*time.Millisecond)
	accepted := p.call(t, http.MethodPost, projectKey, runPath(p.store.AllRuns()[0].PublicID)+"/cancel")
	require.Equal(t, http.StatusAccepted, accepted.StatusCode, accepted.Body)
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	stream := string(raw)
	assert.Contains(t, stream, `"type":"RUN_ERROR"`)
	assert.Contains(t, stream, `"code":"cancelled"`)
	assert.NotContains(t, stream, `"type":"RUN_FINISHED"`)
	assert.Equal(t, run.StatusCancelled, p.settled(t).Status)
}

func TestCancel_SettledIs202WithStatus(t *testing.T) {
	p := newPlane(t, 4)
	r := p.settledTurn(t, projectKey)
	resp := p.call(t, http.MethodPost, projectKey, runPath(r.PublicID)+"/cancel")
	require.Equal(t, http.StatusAccepted, resp.StatusCode, resp.Body)
	var view runJSON
	require.NoError(t, json.Unmarshal([]byte(resp.Body), &view))
	assert.Equal(t, "completed", view.Status, "cancel is idempotent and says how the run ended")
	assert.Equal(t, http.StatusForbidden, p.call(t, http.MethodPost, readOnlyKey, runPath(r.PublicID)+"/cancel").StatusCode)
	assert.Equal(t, http.StatusNotFound, p.call(t, http.MethodPost, orgKey, runPath(r.PublicID)+"/cancel").StatusCode)
}

func TestIdempotency_SettledReplays(t *testing.T) {
	p := newPlane(t, 4)
	r := p.settledTurn(t, projectKey, dataplane.IdempotencyHeader, "k-replay")
	again := p.post(t, projectKey, "AGENTpub", body(clientThread, "hello"), dataplane.IdempotencyHeader, "k-replay")
	require.Equal(t, http.StatusOK, again.StatusCode, again.Body)
	assert.Equal(t, "text/event-stream", again.Header.Get("Content-Type"))
	assert.Contains(t, again.Body, `"runId":"`+r.PublicID+`"`, "the replay names the original run")
	assert.Contains(t, again.Body, `"type":"TEXT_MESSAGE_CONTENT"`)
	assert.Contains(t, again.Body, `"type":"RUN_FINISHED"`)
	assert.Len(t, p.store.AllRuns(), 1, "no second run")
	assert.Equal(t, 1, p.rt.Calls(), "no second turn")
}

func TestActor_ReservedPrefixesAreRefused(t *testing.T) {
	p := newPlane(t, 4)
	p.settledTurn(t, orgKey, dataplane.UserHeader, "customer:42")
	stored := len(p.store.Messages)
	for _, named := range []string{"person:Uabc12345", "key:Kabc12345", "Person:Uabc12345", "KEY:Kabc12345"} {
		post := p.post(t, orgKey, "AGENTpub", body(clientThread, "hello"), dataplane.UserHeader, named)
		require.Equal(t, http.StatusBadRequest, post.StatusCode, "POST as %s", named)
		assert.NotContains(t, post.Body, named, "the value is never echoed")
		code, _ := errorCode(t, post)
		assert.Equal(t, apperror.CodeValidation, code)
		read := p.call(t, http.MethodGet, orgKey, "/api/v1/agents/AGENTpub/threads/"+clientThread+"/messages", dataplane.UserHeader, named)
		assert.Equal(t, http.StatusBadRequest, read.StatusCode, "GET as %s", named)
		assert.NotContains(t, read.Body, named)
	}
	assert.Len(t, p.store.Messages, stored, "nothing was written as someone else")
}

type sameHint struct{ names }

// KeyPublicID is what an actor built from a key's hint would read: every key here shares one.
func (sameHint) KeyPublicID(context.Context, uuid.UUID) (string, error) { return "...ab12", nil }

func TestActor_DefaultIsTheKeyNotItsHint(t *testing.T) {
	p := newPlane(t, 4, func(d *dataplane.Deps) { d.Names = sameHint{} })
	p.settledTurn(t, projectKey)
	path := "/api/v1/agents/AGENTpub/threads/" + clientThread + "/messages"
	require.Equal(t, http.StatusOK, p.call(t, http.MethodGet, projectKey, path).StatusCode)
	assert.Equal(t, http.StatusNotFound, p.call(t, http.MethodGet, orgKey, path).StatusCode,
		"a revoked key's hint can come back on a new key, which must not inherit its conversations")
}

func TestReads_BadActorHeaderAnswersTheSameWhetherOrNotTheRowExists(t *testing.T) {
	p := newPlane(t, 4)
	r := p.settledTurn(t, orgKey, dataplane.UserHeader, "customer:7")
	for name, path := range map[string][2]string{
		"messages": {"/api/v1/agents/AGENTpub/threads/" + clientThread + "/messages", "/api/v1/agents/AGENTpub/threads/no-such-thread/messages"},
		"run":      {runPath(r.PublicID), runPath("no-such-run")},
	} {
		for _, bad := range []string{"person:Uabc", "has space"} {
			existing := p.call(t, http.MethodGet, orgKey, path[0], dataplane.UserHeader, bad)
			missing := p.call(t, http.MethodGet, orgKey, path[1], dataplane.UserHeader, bad)
			assert.Equal(t, http.StatusBadRequest, existing.StatusCode, "%s exists, header %q", name, bad)
			assert.Equal(t, http.StatusBadRequest, missing.StatusCode, "%s missing, header %q", name, bad)
		}
	}
	cancel := p.call(t, http.MethodPost, orgKey, runPath("no-such-run")+"/cancel", dataplane.UserHeader, "key:x")
	assert.Equal(t, http.StatusBadRequest, cancel.StatusCode)
}
