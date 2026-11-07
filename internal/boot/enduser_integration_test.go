//go:build integration

package boot

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	enduserv1 "altalune.id/oagent/gen/go/enduser/v1"
	threadv1 "altalune.id/oagent/gen/go/thread/v1"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/apiclient"
	"altalune.id/oagent/internal/apikey"
	"altalune.id/oagent/internal/channel"
	"altalune.id/oagent/internal/enduser"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/scope"
)

const (
	waPhone = "6281234567890"
	waLID   = "123456789012345"
)

// SECURITY: a person's phone number or LID never appears in any surface's output, only a masked hint.
func TestEndUsers_NoPhoneOrLIDInAPIOrMCP(t *testing.T) {
	f := newAPIFixture(t)
	s := seedReach(t, f)
	ctx := t.Context()
	prj, err := f.svcs.Projects.Create(ctx, project.CreateRequest{OrgID: s.orgB, Slug: "whatsapp", Name: "WA"})
	require.NoError(t, err)
	in := tenant.Into(ctx, tenant.Context{OrgID: s.orgB, ProjectID: prj.ID})
	ag, err := f.svcs.Agents.Create(ctx, agent.CreateRequest{OrgID: s.orgB, ProjectID: prj.ID, Slug: "wa-bot", Name: "WA Bot"})
	require.NoError(t, err)
	conn, err := f.svcs.Channels.Create(in, prj.ID, ag.ID, channel.Input{
		BaseURL: "https://wa.example.com", OpenWAOrg: "acme", OpenWAProject: "main", DeviceID: "dev_V1StGXR8Z5jdHi6B",
	}, "owk_probe")
	require.NoError(t, err)
	who, err := f.svcs.EndUsers.Upsert(in, prj.ID, enduser.Identity{Kind: enduser.KindWAPhone, Value: waPhone}, enduser.Identity{Kind: enduser.KindWALid, Value: waLID})
	require.NoError(t, err)
	lidOnly, err := f.svcs.EndUsers.Upsert(in, prj.ID, enduser.Identity{Kind: enduser.KindWALid, Value: "998877665544332"})
	require.NoError(t, err)
	require.NoError(t, f.svcs.EndUsers.SetDisplayName(in, who.ID, channel.CleanDisplayName("Sari 0812-3456-7890")))
	require.NoError(t, f.svcs.EndUsers.SetDisplayName(in, lidOnly.ID, channel.CleanDisplayName("Budi +62 812 3456 7890")))
	at := time.Now()
	dm, err := f.svcs.RunStore.AppendChannelMessage(in, run.ChannelMessage{
		ProjectID: prj.ID, AgentID: ag.ID, ConnectionID: conn.ID, ChatKey: "cht_DmDmDmDmDmDmDmDm", Kind: "dm", ReplyVia: "wa_phone", Actor: who.ID,
		Message:   run.Message{Body: json.RawMessage(`{"id":"m1","role":"user","content":"halo"}`), SpeakerID: &who.ID, ExternalMessageID: "msg_DmMessage0000001", ChannelTime: &at},
		SenderCap: 100,
	})
	require.NoError(t, err)
	group, err := f.svcs.RunStore.AppendChannelMessage(in, run.ChannelMessage{
		ProjectID: prj.ID, AgentID: ag.ID, ConnectionID: conn.ID, ChatKey: "cht_GrGrGrGrGrGrGrGr", Kind: "group", ChannelAddress: "120363012345678901@g.us",
		Message:   run.Message{Body: json.RawMessage(`{"id":"m2","role":"user","content":"[member] stok?"}`), SpeakerID: &lidOnly.ID, ExternalMessageID: "msg_GrMessage0000001", ChannelTime: &at},
		SenderCap: 100,
	})
	require.NoError(t, err)

	cookie, _, _ := f.personCookieIn(t, s.orgB, "b-viewer@example.com", member.RoleOwner)
	c := apiclient.New(strings.TrimSuffix(f.url, "/api"), apiclient.Credential{Session: strings.TrimPrefix(cookie, session.CookieName+"=")})
	pubOf := func(a run.ChannelAppend) string {
		th, err := f.svcs.RunStore.Thread(in, a.ThreadID)
		require.NoError(t, err)
		return th.PublicID
	}
	var outputs []proto.Message
	list, err := c.Threads.ListThreads(ctx, connect.NewRequest(&threadv1.ListThreadsRequest{ProjectId: prj.PublicID}))
	require.NoError(t, err)
	require.Len(t, list.Msg.GetThreads(), 2)
	outputs = append(outputs, list.Msg)
	for _, th := range list.Msg.GetThreads() {
		got, err := c.Threads.GetThread(ctx, connect.NewRequest(&threadv1.GetThreadRequest{ThreadId: th.GetId()}))
		require.NoError(t, err)
		msgs, err := c.Threads.ListThreadMessages(ctx, connect.NewRequest(&threadv1.ListThreadMessagesRequest{ThreadId: th.GetId()}))
		require.NoError(t, err)
		require.NotEmpty(t, msgs.Msg.GetMessages())
		outputs = append(outputs, got.Msg, msgs.Msg)
	}
	people, err := c.EndUsers.ListEndUsers(ctx, connect.NewRequest(&enduserv1.ListEndUsersRequest{ProjectId: prj.PublicID}))
	require.NoError(t, err)
	require.Len(t, people.Msg.GetEndUsers(), 2)
	outputs = append(outputs, people.Msg)
	one, err := c.EndUsers.GetEndUser(ctx, connect.NewRequest(&enduserv1.GetEndUserRequest{Id: who.PublicID}))
	require.NoError(t, err)
	assert.Equal(t, enduserv1.IdentityKind_IDENTITY_KIND_WA_PHONE, one.Msg.GetEndUser().GetIdentityKind())
	assert.Equal(t, "90", one.Msg.GetEndUser().GetIdentityHint())
	require.Len(t, one.Msg.GetThreads(), 1, "the DM it owns")
	outputs = append(outputs, one.Msg)

	var bodies []string
	for _, o := range outputs {
		bodies = append(bodies, protojson.Format(o))
	}
	key := f.mintKey(t, apikey.CreateRequest{OrgID: s.orgB, Kind: apikey.KindOrg, Scopes: []string{scope.ThreadRead}, AllProjects: true})
	for _, call := range []string{
		`{"name":"thread_list","arguments":{"projectId":"` + prj.PublicID + `"}}`,
		`{"name":"thread_messages","arguments":{"threadId":"` + pubOf(dm) + `"}}`,
		`{"name":"thread_messages","arguments":{"threadId":"` + pubOf(group) + `"}}`,
		`{"name":"end_user_list","arguments":{"projectId":"` + prj.PublicID + `"}}`,
		`{"name":"end_user_get","arguments":{"id":"` + who.PublicID + `"}}`,
	} {
		status, _, body := f.mcpCall(t, key, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":`+call+`}`)
		require.Equal(t, http.StatusOK, status, string(body))
		require.NotContains(t, string(body), `"isError":true`, "%s: %s", call, body)
		bodies = append(bodies, string(body))
	}
	assert.Contains(t, strings.Join(bodies, "\n"), "Budi ***", "the masked display name is what shows")
	for _, b := range bodies {
		for _, secret := range []string{waPhone, waLID, "998877665544332", "@s.whatsapp.net", "@lid", "3456-7890", "3456 7890"} {
			assert.NotContains(t, b, secret)
		}
	}
}

// SECURITY: a search by a whole phone number finds the person, but the number reaches no log line and no span.
func TestListEndUsers_PhoneQueryReachesNoLogOrSpan(t *testing.T) {
	spans := tracetest.NewSpanRecorder()
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans)))
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
	f := newAPIFixture(t)
	s := seedReach(t, f)
	ctx := t.Context()
	prj, err := f.svcs.Projects.Create(ctx, project.CreateRequest{OrgID: s.orgB, Slug: "whatsapp", Name: "WA"})
	require.NoError(t, err)
	in := tenant.Into(ctx, tenant.Context{OrgID: s.orgB, ProjectID: prj.ID})
	_, err = f.svcs.EndUsers.Upsert(in, prj.ID, enduser.Identity{Kind: enduser.KindWAPhone, Value: waPhone})
	require.NoError(t, err)

	cookie, _, _ := f.personCookieIn(t, s.orgB, "b-viewer@example.com", member.RoleOwner)
	c := apiclient.New(strings.TrimSuffix(f.url, "/api"), apiclient.Credential{Session: strings.TrimPrefix(cookie, session.CookieName+"=")})
	found, err := c.EndUsers.ListEndUsers(ctx, connect.NewRequest(&enduserv1.ListEndUsersRequest{ProjectId: prj.PublicID, Query: waPhone}))
	require.NoError(t, err)
	require.Len(t, found.Msg.GetEndUsers(), 1, "a whole number finds the person")
	key := f.mintKey(t, apikey.CreateRequest{OrgID: s.orgB, Kind: apikey.KindOrg, Scopes: []string{scope.ThreadRead}, AllProjects: true})
	status, _, body := f.mcpCall(t, key, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"end_user_list","arguments":{"projectId":"`+prj.PublicID+`","query":"`+waPhone+`"}}}`)
	require.Equal(t, http.StatusOK, status, string(body))
	require.NotContains(t, string(body), `"isError":true`, string(body))

	assert.NotEmpty(t, f.logs.String(), "the log capture is live")
	assert.NotContains(t, f.logs.String(), waPhone, "logs")
	var rpc int
	for _, sp := range spans.Ended() {
		var b strings.Builder
		b.WriteString(sp.Name())
		for _, a := range sp.Attributes() {
			b.WriteString(" " + a.Value.String())
		}
		for _, e := range sp.Events() {
			b.WriteString(" " + e.Name)
			for _, a := range e.Attributes {
				b.WriteString(" " + a.Value.String())
			}
		}
		if strings.Contains(sp.Name(), "ListEndUsers") {
			rpc++
		}
		assert.NotContains(t, b.String(), waPhone, "span %s", sp.Name())
	}
	assert.Positive(t, rpc, "the RPC is traced")
}
