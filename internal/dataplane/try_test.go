package dataplane_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/dataplane"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/testutil/fakes"
	"altalune.id/oagent/scope"
)

type drafts struct {
	mu     sync.Mutex
	err    error
	pinned []uuid.UUID
}

func (d *drafts) Pin(_ context.Context, agentID, _ uuid.UUID) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.err != nil {
		return d.err
	}
	d.pinned = append(d.pinned, agentID)
	return nil
}

type tryPlane struct {
	*plane
	console *httptest.Server
	drafts  *drafts
	members map[string]session.Principal
	draftID uuid.UUID
}

const (
	alice    = "alice"
	bob      = "bob"
	viewer   = "viewer"
	stranger = "stranger"
)

func newTryPlane(t *testing.T) *tryPlane {
	t.Helper()
	tp := &tryPlane{drafts: &drafts{}, draftID: uuid.New()}
	tp.plane = newPlane(t, 4, func(d *dataplane.Deps) { d.Drafts = tp.drafts })
	published := tp.agents.Versions[tp.version]
	draft := *published
	draft.ID, draft.Version, draft.Draft = tp.draftID, 0, true
	tp.agents.Drafts = map[uuid.UUID]*run.AgentVersion{tp.agent: &draft}
	tp.agents.Versions[draft.ID] = &draft
	person := func(org uuid.UUID, scopes ...string) session.Principal {
		return session.Principal{UserID: uuid.New(), Source: session.SourceLocal, Scopes: scopes, ActiveOrgID: org}
	}
	tp.members = map[string]session.Principal{
		alice:    person(tp.org, scope.RunCreate, scope.RunRead),
		bob:      person(tp.org, scope.RunCreate, scope.RunRead),
		viewer:   person(tp.org, scope.RunRead),
		stranger: person(uuid.New(), scope.RunCreate, scope.RunRead),
	}
	mux := http.NewServeMux()
	for pattern, route := range tp.handler.TryRoutes() {
		method, path, _ := strings.Cut(pattern, " ")
		mux.Handle(method+" /console"+path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if p, ok := tp.members[r.Header.Get("X-Test-Member")]; ok {
				r = r.WithContext(session.PrincipalInto(r.Context(), p))
			}
			route.ServeHTTP(w, r)
		}))
	}
	tp.console = httptest.NewServer(mux)
	t.Cleanup(tp.console.Close)
	return tp
}

func (tp *tryPlane) ctx() context.Context {
	return tenant.Into(context.Background(), tenant.Context{OrgID: tp.org, ProjectID: tp.project})
}

func (tp *tryPlane) reply(t *testing.T) {
	t.Helper()
	tp.rt.Replies = append(tp.rt.Replies, fakes.RuntimeReply{Stream: runtimeStream(t, "plain_reply")})
}

func (tp *tryPlane) do(t *testing.T, member, method, path, payload string) result {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, tp.console.URL+"/console"+path, strings.NewReader(payload))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if member != "" {
		req.Header.Set("X-Test-Member", member)
	}
	resp, err := tp.console.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return result{StatusCode: resp.StatusCode, Header: resp.Header, Body: string(raw)}
}

func TestTry_RunsTheDraftAsTheMember(t *testing.T) {
	tp := newTryPlane(t)
	tp.reply(t)
	resp := tp.do(t, alice, http.MethodPost, "/agents/AGENTpub/try/runs", body("try-1", "hello"))
	require.Equal(t, http.StatusOK, resp.StatusCode, resp.Body)
	assert.Contains(t, resp.Body, "RUN_FINISHED")
	r := tp.settled(t)
	assert.Equal(t, tp.draftID, r.AgentVersionID, "a try run is pinned to the draft")
	th, err := tp.store.Thread(tp.ctx(), r.ThreadID)
	require.NoError(t, err)
	assert.Equal(t, "try", th.Kind)
	assert.Equal(t, []uuid.UUID{tp.agent}, tp.drafts.pinned, "the draft is pinned, refused like a publish, before any write")
	alicePub := "person:U" + tp.members[alice].UserID.String()[:8]
	id, ok, err := tp.eu.FindAPIUser(context.Background(), tp.project, alicePub)
	require.NoError(t, err)
	require.True(t, ok, "the member speaks as person:<user public id>, the same actor as their personal key")
	assert.Equal(t, id, th.ActorEndUserID)
}

func TestTry_RunsAnAgentThatWasNeverPublished(t *testing.T) {
	tp := newTryPlane(t)
	resp := tp.do(t, alice, http.MethodPost, "/agents/DRAFTpub/try/runs", body("try-1", "hello"))
	assert.NotEqual(t, http.StatusConflict, resp.StatusCode, "Try it never asks for a publish: %s", resp.Body)
}

func TestTry_Refusals(t *testing.T) {
	tp := newTryPlane(t)
	for name, c := range map[string]struct {
		member, agent string
		status        int
		code          apperror.Code
	}{
		"signed out":            {"", "AGENTpub", http.StatusUnauthorized, apperror.CodeUnauthenticated},
		"no run:create":         {viewer, "AGENTpub", http.StatusForbidden, apperror.CodeForbidden},
		"member of another org": {stranger, "AGENTpub", http.StatusNotFound, apperror.CodeAgentNotFound},
		"another org's agent":   {alice, "FOREIGNpub", http.StatusNotFound, apperror.CodeAgentNotFound},
		"unknown agent":         {alice, "NOPEpub", http.StatusNotFound, apperror.CodeAgentNotFound},
	} {
		t.Run(name, func(t *testing.T) {
			resp := tp.do(t, c.member, http.MethodPost, "/agents/"+c.agent+"/try/runs", body("try-1", "hello"))
			assert.Equal(t, c.status, resp.StatusCode, resp.Body)
			code, _ := errorCode(t, resp)
			assert.Equal(t, c.code, code)
		})
	}
	assert.Empty(t, tp.store.AllRuns(), "no refusal writes a run")
	assert.Empty(t, tp.drafts.pinned, "or pins a draft")
}

func TestTry_DraftBlockerRefusesBeforeAnyWrite(t *testing.T) {
	tp := newTryPlane(t)
	tp.drafts.err = apperror.New(apperror.CodeAgentInvalidGrant, "The model is not set").
		WithMeta(map[string]string{"field": "config.model", "reason": "model"})
	resp := tp.do(t, alice, http.MethodPost, "/agents/AGENTpub/try/runs", body("try-1", "hello"))
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, resp.Body)
	code, meta := errorCode(t, resp)
	assert.Equal(t, apperror.CodeAgentInvalidGrant, code, "the same AGT error publish gives")
	assert.Equal(t, "config.model", meta["field"])
	assert.Empty(t, tp.store.AllRuns())
	_, err := tp.store.TryThreadByExternalKey(tp.ctx(), tp.agent, "try-1")
	assert.True(t, run.IsThreadNotFoundError(err), "no thread was written")
}

func TestTry_CancelAndMessagesAreTheMembersOwn(t *testing.T) {
	tp := newTryPlane(t)
	tp.reply(t)
	require.Equal(t, http.StatusOK, tp.do(t, alice, http.MethodPost, "/agents/AGENTpub/try/runs", body("try-1", "hello")).StatusCode)
	r := tp.settled(t)

	page := tp.do(t, alice, http.MethodGet, "/agents/AGENTpub/try/threads/try-1/messages", "")
	require.Equal(t, http.StatusOK, page.StatusCode, page.Body)
	var got struct {
		Messages []json.RawMessage `json:"messages"`
	}
	require.NoError(t, json.Unmarshal([]byte(page.Body), &got))
	assert.NotEmpty(t, got.Messages)
	assert.Equal(t, http.StatusNotFound, tp.do(t, bob, http.MethodGet, "/agents/AGENTpub/try/threads/try-1/messages", "").StatusCode,
		"another member's try thread reads as not found")
	assert.Equal(t, http.StatusForbidden, tp.do(t, viewer, http.MethodPost, "/agents/AGENTpub/try/runs/"+r.PublicID+"/cancel", "").StatusCode,
		"cancel needs run:create")
	assert.Equal(t, http.StatusNotFound, tp.do(t, bob, http.MethodPost, "/agents/AGENTpub/try/runs/"+r.PublicID+"/cancel", "").StatusCode)
	assert.Equal(t, http.StatusAccepted, tp.do(t, alice, http.MethodPost, "/agents/AGENTpub/try/runs/"+r.PublicID+"/cancel", "").StatusCode)
}
