package run_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	threadv1 "altalune.id/oagent/gen/go/thread/v1"
	"altalune.id/oagent/internal/enduser"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/testutil/fakes"
)

type cpWorld struct {
	store                      *fakes.RunStore
	refs                       *fakes.PublicIDs
	cp                         *run.ThreadService
	org, project, agent, thrd  uuid.UUID
	projectPub, agentPub, tPub string
	running                    *run.Run
	phone                      enduser.Identity
}

func newCPWorld(t *testing.T) *cpWorld {
	t.Helper()
	w := &cpWorld{
		store: &fakes.RunStore{PublicIDs: map[uuid.UUID]string{}, Actors: map[uuid.UUID]uuid.UUID{}, Runs: map[uuid.UUID]*run.Run{}},
		refs:  fakes.NewPublicIDs(), org: uuid.New(), project: uuid.New(), agent: uuid.New(), thrd: uuid.New(),
		phone: enduser.Identity{Kind: enduser.KindWAPhone, Value: "6281234567890"},
	}
	w.refs.Org(w.org)
	w.projectPub = w.refs.Project(w.project, w.org)
	w.agentPub = w.refs.Resource(w.agent, w.org)
	w.tPub = "tHREADpub" + uuid.NewString()[:12]
	w.store.AddAgentThread(w.thrd, w.org, w.project, w.agent)
	w.store.PublicIDs[w.thrd] = w.tPub

	ctx := tenant.Into(context.Background(), tenant.Context{OrgID: w.org, ProjectID: w.project})
	people := enduser.NewService(&fakes.EndUserStore{}, fakes.Transactor{}, nil)
	who, err := people.Upsert(ctx, w.project, w.phone)
	require.NoError(t, err)
	w.store.Actors[w.thrd] = who.ID
	_, err = w.store.AppendMessage(ctx, w.thrd, run.Message{
		Role: run.RoleUser, Body: json.RawMessage(`{"id":"m1","role":"user","content":"hi"}`), SpeakerID: &who.ID, Status: run.MessagePending,
	})
	require.NoError(t, err)
	version := uuid.New()
	w.running = run.New(w.org, w.project, w.thrd, version, time.Now())
	w.store.Runs[w.running.ID] = w.running
	svc := run.NewService(w.store, fakes.Transactor{}, nil).WithAgents(&fakes.RunAgents{Versions: map[uuid.UUID]*run.AgentVersion{
		version: {ID: version, AgentID: w.agent, Version: 3},
	}})
	w.cp = run.NewControlPlane(svc, w.refs, people)
	return w
}

func member(orgID uuid.UUID) context.Context {
	return session.PrincipalInto(context.Background(), session.Principal{
		Source: session.SourceLocal, UserID: uuid.New(), ActiveOrgID: orgID, Scopes: []string{"*"},
	})
}

func TestThreadService_ReadsByPublicID(t *testing.T) {
	w := newCPWorld(t)
	ctx := member(w.org)
	list, err := w.cp.ListThreads(ctx, connect.NewRequest(&threadv1.ListThreadsRequest{ProjectId: w.projectPub, AgentId: w.agentPub}))
	require.NoError(t, err)
	require.Len(t, list.Msg.GetThreads(), 1)
	th := list.Msg.GetThreads()[0]
	assert.Equal(t, w.tPub, th.GetId())
	assert.Equal(t, w.projectPub, th.GetProjectId())
	assert.Equal(t, w.agentPub, th.GetAgentId())
	assert.Equal(t, threadv1.RunStatus_RUN_STATUS_RUNNING, th.GetOpenRunStatus())
	assert.Empty(t, list.Msg.GetNextPageToken(), "one page")

	got, err := w.cp.GetThread(ctx, connect.NewRequest(&threadv1.GetThreadRequest{ThreadId: w.tPub}))
	require.NoError(t, err)
	assert.Equal(t, int64(1), got.Msg.GetThread().GetMessageCount())
	assert.Equal(t, int64(1), got.Msg.GetThread().GetRunCount())

	msgs, err := w.cp.ListThreadMessages(ctx, connect.NewRequest(&threadv1.ListThreadMessagesRequest{ThreadId: w.tPub}))
	require.NoError(t, err)
	require.Len(t, msgs.Msg.GetMessages(), 1)
	assert.Equal(t, "hi", msgs.Msg.GetMessages()[0].GetMessage().GetFields()["content"].GetStringValue())
	assert.Equal(t, int64(1), msgs.Msg.GetNextAfterSeq())

	runs, err := w.cp.ListRuns(ctx, connect.NewRequest(&threadv1.ListRunsRequest{ThreadId: w.tPub}))
	require.NoError(t, err)
	require.Len(t, runs.Msg.GetRuns(), 1)
	r := runs.Msg.GetRuns()[0]
	assert.Equal(t, w.running.PublicID, r.GetId())
	assert.Equal(t, w.tPub, r.GetThreadId())
	assert.Equal(t, int32(3), r.GetAgentVersion())
	assert.Nil(t, r.CostMicros, "unpriced")
	assert.Nil(t, r.LatencyMs, "still running")

	one, err := w.cp.GetRun(ctx, connect.NewRequest(&threadv1.GetRunRequest{RunId: w.running.PublicID}))
	require.NoError(t, err)
	assert.Equal(t, threadv1.RunStatus_RUN_STATUS_RUNNING, one.Msg.GetRun().GetStatus())
	for _, raw := range []any{list.Msg, got.Msg, msgs.Msg, runs.Msg, one.Msg} {
		b, err := protojson.Marshal(raw.(proto.Message))
		require.NoError(t, err)
		for _, internal := range []uuid.UUID{w.org, w.project, w.agent, w.thrd, w.running.ID} {
			assert.NotContains(t, string(b), internal.String(), "no UUID on the wire")
		}
	}
}

func TestListThreads_NoPhoneInLabel(t *testing.T) {
	w := newCPWorld(t)
	ctx := member(w.org)
	list, err := w.cp.ListThreads(ctx, connect.NewRequest(&threadv1.ListThreadsRequest{ProjectId: w.projectPub}))
	require.NoError(t, err)
	label := list.Msg.GetThreads()[0].GetEndUserLabel()
	assert.Equal(t, "wa_phone …90", label)
	msgs, err := w.cp.ListThreadMessages(ctx, connect.NewRequest(&threadv1.ListThreadMessagesRequest{ThreadId: w.tPub}))
	require.NoError(t, err)
	assert.Equal(t, "wa_phone …90", msgs.Msg.GetMessages()[0].GetSpeakerLabel())
	for _, b := range []string{protojson.Format(list.Msg), protojson.Format(msgs.Msg)} {
		assert.NotContains(t, b, w.phone.Value)
		assert.NotContains(t, b, "1234567890")
	}
}

func TestCancelRun_SamePathAsDataPlane(t *testing.T) {
	w := newCPWorld(t)
	ctx := member(w.org)
	token := uuid.NewString()
	claim, err := w.store.Claim(tenant.Into(context.Background(), tenant.Context{OrgID: w.org}), w.thrd, token, time.Hour)
	require.NoError(t, err)
	require.Equal(t, run.Claimed, claim, "a live turn holds the thread")
	resp, err := w.cp.CancelRun(ctx, connect.NewRequest(&threadv1.CancelRunRequest{RunId: w.running.PublicID}))
	require.NoError(t, err)
	assert.Equal(t, threadv1.RunStatus_RUN_STATUS_RUNNING, resp.Msg.GetRun().GetStatus(), "the turn stops at its next step")
	th, err := w.store.Thread(tenant.Into(context.Background(), tenant.Context{OrgID: w.org}), w.thrd)
	require.NoError(t, err)
	assert.True(t, th.CancelRequested, "the held thread is asked to cancel, as the data plane's cancel does")

	w.running.Status = run.StatusCompleted
	again, err := w.cp.CancelRun(ctx, connect.NewRequest(&threadv1.CancelRunRequest{RunId: w.running.PublicID}))
	require.NoError(t, err)
	assert.Equal(t, threadv1.RunStatus_RUN_STATUS_COMPLETED, again.Msg.GetRun().GetStatus(), "a run that ended is returned as it is")
}

func TestThreadReach_IDOnlyRPCs(t *testing.T) {
	w := newCPWorld(t)
	other := uuid.New()
	for name, p := range map[string]session.Principal{
		"member of another org":          {Source: session.SourceLocal, UserID: uuid.New(), ActiveOrgID: other, Scopes: []string{"*"}},
		"key of another org":             {Source: session.SourceAPIKey, ActiveOrgID: other, AllProjects: true},
		"key limited to another project": {Source: session.SourceAPIKey, ActiveOrgID: w.org, ProjectIDs: []uuid.UUID{uuid.New()}},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := session.PrincipalInto(context.Background(), p)
			for _, id := range []string{w.tPub, w.thrd.String(), "unknown"} {
				_, err := w.cp.GetThread(ctx, connect.NewRequest(&threadv1.GetThreadRequest{ThreadId: id}))
				assert.True(t, run.IsThreadNotFoundError(err), "GetThread %s: %v", id, err)
				_, err = w.cp.ListThreadMessages(ctx, connect.NewRequest(&threadv1.ListThreadMessagesRequest{ThreadId: id}))
				assert.True(t, run.IsThreadNotFoundError(err), "ListThreadMessages %s: %v", id, err)
				_, err = w.cp.ListRuns(ctx, connect.NewRequest(&threadv1.ListRunsRequest{ThreadId: id}))
				assert.True(t, run.IsThreadNotFoundError(err), "ListRuns %s: %v", id, err)
			}
			for _, id := range []string{w.running.PublicID, w.running.ID.String(), "unknown"} {
				_, err := w.cp.GetRun(ctx, connect.NewRequest(&threadv1.GetRunRequest{RunId: id}))
				assert.True(t, run.IsNotFoundError(err), "GetRun %s: %v", id, err)
				_, err = w.cp.CancelRun(ctx, connect.NewRequest(&threadv1.CancelRunRequest{RunId: id}))
				assert.True(t, run.IsNotFoundError(err), "CancelRun %s: %v", id, err)
			}
			_, err := w.cp.ListThreads(ctx, connect.NewRequest(&threadv1.ListThreadsRequest{ProjectId: w.projectPub}))
			assert.True(t, run.IsThreadNotFoundError(err), "ListThreads: %v", err)
			th, err := w.store.Thread(tenant.Into(context.Background(), tenant.Context{OrgID: w.org}), w.thrd)
			require.NoError(t, err)
			assert.False(t, th.CancelRequested, "no write")
			assert.Equal(t, run.StatusRunning, w.running.Status)
		})
	}
}

func TestListThreads_PagesByToken(t *testing.T) {
	w := newCPWorld(t)
	second := uuid.New()
	w.store.AddAgentThread(second, w.org, w.project, w.agent)
	w.store.PublicIDs[second] = "tSECONDpub" + uuid.NewString()[:11]
	w.store.Activity = map[uuid.UUID]time.Time{w.thrd: time.Now(), second: time.Now().Add(-time.Minute)}
	ctx := member(w.org)
	first, err := w.cp.ListThreads(ctx, connect.NewRequest(&threadv1.ListThreadsRequest{ProjectId: w.projectPub, PageSize: 1}))
	require.NoError(t, err)
	require.Len(t, first.Msg.GetThreads(), 1)
	assert.Equal(t, w.tPub, first.Msg.GetThreads()[0].GetId())
	require.NotEmpty(t, first.Msg.GetNextPageToken())
	next, err := w.cp.ListThreads(ctx, connect.NewRequest(&threadv1.ListThreadsRequest{
		ProjectId: w.projectPub, PageSize: 1, PageToken: first.Msg.GetNextPageToken(),
	}))
	require.NoError(t, err)
	require.Len(t, next.Msg.GetThreads(), 1)
	assert.Equal(t, w.store.PublicIDs[second], next.Msg.GetThreads()[0].GetId())
	_, err = w.cp.ListThreads(ctx, connect.NewRequest(&threadv1.ListThreadsRequest{ProjectId: w.projectPub, PageToken: "not-a-token"}))
	assert.True(t, run.IsInvalidPageTokenError(err), "%v", err)
}

func TestAGUIBody_KeepsOnlyAGUIFieldsAndNeverLeaksWhatItCannotRead(t *testing.T) {
	m := run.Message{Body: json.RawMessage(`{"id":"m1","role":"tool","content":"ok","toolCallId":"c1","raw":{"secret":"x"}}`)}
	assert.JSONEq(t, `{"id":"m1","role":"tool","content":"ok","toolCallId":"c1"}`, string(m.AGUIBody()))
	for _, bad := range []string{`not json`, `["raw","secret"]`, `"secret"`, ``} {
		assert.Equal(t, `{}`, string(run.Message{Body: json.RawMessage(bad)}.AGUIBody()), "%q", bad)
	}
}
