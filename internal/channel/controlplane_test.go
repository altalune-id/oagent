package channel_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"altalune.id/oagent/apperror"
	channelv1 "altalune.id/oagent/gen/go/channel/v1"
	channelv1connect "altalune.id/oagent/gen/go/channel/v1/channelv1connect"
	"altalune.id/oagent/internal/channel"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/testutil/fakes"
	"altalune.id/oagent/sealer"
)

const (
	cpSecret = "whsec_primary_secret_value"
	cpKey    = "owk_device_key_value"
)

func testSealer(t *testing.T) sealer.Sealer {
	t.Helper()
	key, err := sealer.GenerateKey()
	require.NoError(t, err)
	sl, err := sealer.New(key)
	require.NoError(t, err)
	return sl
}

type cpWorld struct {
	store              *fakes.ChannelStore
	refs               *fakes.PublicIDs
	svc                *channel.Service
	cp                 *channel.ChannelService
	client             channelv1connect.ChannelServiceClient
	org, project       uuid.UUID
	agent              uuid.UUID
	projectPub, agtPub string
}

func cpOptions() channel.ControlPlaneOptions {
	return channel.ControlPlaneOptions{HooksURL: "https://oagent.example.com/hooks/openwa", DefaultBaseURL: "https://wa.example.com", MaxMessageAge: 20 * time.Minute}
}

func newCPWorld(t *testing.T) *cpWorld {
	t.Helper()
	w := &cpWorld{store: &fakes.ChannelStore{}, refs: fakes.NewPublicIDs(), org: uuid.New(), project: uuid.New(), agent: uuid.New()}
	w.refs.Org(w.org)
	w.projectPub = w.refs.Project(w.project, w.org)
	w.agtPub = w.refs.Resource(w.agent, w.org)
	w.svc = channel.NewService(w.store, fakes.Transactor{}, w.refs, apperror.NewReporter(nil, false).Unexpected).WithSealer(testSealer(t))
	w.cp = channel.NewControlPlane(w.svc, w.refs, cpOptions())
	_, handler := channelv1connect.NewChannelServiceHandler(w.cp, connect.WithInterceptors(fakes.OperatorInterceptor()))
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	w.client = channelv1connect.NewChannelServiceClient(http.DefaultClient, ts.URL)
	return w
}

func createConnReq(projectPub, agentPub string) *channelv1.CreateConnectionRequest {
	return &channelv1.CreateConnectionRequest{
		ProjectId: projectPub, AgentId: agentPub, BaseUrl: "https://wa.example.com", OpenwaOrg: "acme", OpenwaProject: "main",
		DeviceId: "dev_V1StGXR8Z5jdHi6B", DeviceKey: cpKey,
	}
}

func (w *cpWorld) create(t *testing.T) *channelv1.Connection {
	t.Helper()
	resp, err := w.client.CreateConnection(t.Context(), connect.NewRequest(createConnReq(w.projectPub, w.agtPub)))
	require.NoError(t, err)
	return resp.Msg.GetConnection()
}

func (w *cpWorld) setSecret(t *testing.T, id, secret string, drop bool) *channelv1.Connection {
	t.Helper()
	resp, err := w.client.SetConnectionSecret(t.Context(), connect.NewRequest(&channelv1.SetConnectionSecretRequest{Id: id, SigningSecret: secret, DropPrevious: drop}))
	require.NoError(t, err)
	return resp.Msg.GetConnection()
}

func TestCreateConnection_SecretNeverReturned(t *testing.T) {
	w := newCPWorld(t)
	c := w.create(t)
	assert.False(t, c.GetSecretSet(), "OpenWA mints the secret after it has the webhook URL")
	c = w.setSecret(t, c.GetId(), cpSecret, false)
	assert.True(t, c.GetSecretSet())
	raw, err := protojson.Marshal(c)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), cpSecret)
	assert.NotContains(t, string(raw), cpKey)
	assert.Len(t, c.GetId(), 21)
	assert.Equal(t, w.projectPub, c.GetProjectId())
	assert.Equal(t, w.agtPub, c.GetAgentId())
	assert.Equal(t, "openwa", c.GetProvider())
	assert.True(t, c.GetEnabled())
	assert.Equal(t, channelv1.ConnectionStatus_CONNECTION_STATUS_WAITING, c.GetStatus(), "waiting before the first verified delivery")
	assert.Equal(t, "https://oagent.example.com/hooks/openwa/"+c.GetId(), c.GetWebhookUrl())
	assert.False(t, c.GetSecondarySet())
	assert.Equal(t, int32(3000), c.GetQuietWindowMs())
	assert.Equal(t, channel.DefaultReplyVoice, c.GetReplyVoice())
	for _, uuidLike := range []string{w.org.String(), w.project.String(), w.agent.String()} {
		assert.NotContains(t, string(raw), uuidLike)
	}

	got, err := w.client.GetConnection(t.Context(), connect.NewRequest(&channelv1.GetConnectionRequest{Id: c.GetId()}))
	require.NoError(t, err)
	raw, _ = protojson.Marshal(got.Msg)
	assert.NotContains(t, string(raw), cpSecret)
	assert.NotContains(t, string(raw), cpKey)
}

func TestConnection_StatusComesFromVerifiedOutcomesOnly(t *testing.T) {
	w := newCPWorld(t)
	c := w.create(t)
	id := w.idOf(c)
	ctx := t.Context()
	for result, want := range map[string]channelv1.ConnectionStatus{
		channel.ResultConnected:      channelv1.ConnectionStatus_CONNECTION_STATUS_CONNECTED,
		channel.ResultDeviceMismatch: channelv1.ConnectionStatus_CONNECTION_STATUS_DEVICE_MISMATCH,
		channel.ResultKeyRefused:     channelv1.ConnectionStatus_CONNECTION_STATUS_KEY_REFUSED,
	} {
		require.NoError(t, w.store.RecordResult(ctx, id, result, time.Now()))
		got, err := w.client.GetConnection(ctx, connect.NewRequest(&channelv1.GetConnectionRequest{Id: c.GetId()}))
		require.NoError(t, err)
		assert.Equal(t, want, got.Msg.GetConnection().GetStatus(), result)
		assert.NotNil(t, got.Msg.GetConnection().GetLastResultAt())
	}
	require.NoError(t, w.store.RecordResult(ctx, id, channel.ResultKeyRefused, time.Now()))
	require.NoError(t, w.store.RecordBadSignature(ctx, id, time.Now()))
	got, err := w.client.GetConnection(ctx, connect.NewRequest(&channelv1.GetConnectionRequest{Id: c.GetId()}))
	require.NoError(t, err)
	assert.Equal(t, channelv1.ConnectionStatus_CONNECTION_STATUS_KEY_REFUSED, got.Msg.GetConnection().GetStatus(), "a bad signature never changes the status")
	assert.NotNil(t, got.Msg.GetConnection().GetLastBadSignatureAt())
}

func (w *cpWorld) operator() context.Context {
	return session.PrincipalInto(context.Background(), session.System())
}

func (w *cpWorld) idOf(c *channelv1.Connection) uuid.UUID {
	for id, stored := range w.store.Connections {
		if stored.PublicID == c.GetId() {
			return id
		}
	}
	return uuid.Nil
}

func updateReq(id string) *channelv1.UpdateConnectionRequest {
	return &channelv1.UpdateConnectionRequest{Id: id, OpenwaOrg: new("acme2")}
}

func TestUpdateConnection_EnabledIsTheAdminSwitch(t *testing.T) {
	w := newCPWorld(t)
	c := w.create(t)
	ctx := t.Context()
	kept, err := w.client.UpdateConnection(ctx, connect.NewRequest(updateReq(c.GetId())))
	require.NoError(t, err)
	assert.True(t, kept.Msg.GetConnection().GetEnabled(), "an update that leaves enabled unset never turns a connection off")
	off := updateReq(c.GetId())
	off.Enabled = new(false)
	got, err := w.client.UpdateConnection(ctx, connect.NewRequest(off))
	require.NoError(t, err)
	assert.False(t, got.Msg.GetConnection().GetEnabled())
	assert.Equal(t, channel.StatusDisabled, w.store.Connections[w.idOf(c)].Status)
	assert.Equal(t, channelv1.ConnectionStatus_CONNECTION_STATUS_WAITING, got.Msg.GetConnection().GetStatus(), "the switch is not the reported status")

	got, err = w.client.UpdateConnection(ctx, connect.NewRequest(updateReq(c.GetId())))
	require.NoError(t, err)
	assert.False(t, got.Msg.GetConnection().GetEnabled(), "unset keeps the switch")
	assert.Equal(t, "acme2", got.Msg.GetConnection().GetOpenwaOrg())

	on := updateReq(c.GetId())
	on.Enabled = new(true)
	got, err = w.client.UpdateConnection(ctx, connect.NewRequest(on))
	require.NoError(t, err)
	assert.True(t, got.Msg.GetConnection().GetEnabled())
}

func TestSetConnectionSecret_KeepsOrDropsThePrevious(t *testing.T) {
	w := newCPWorld(t)
	c := w.create(t)
	first := w.setSecret(t, c.GetId(), "whsec_first", true)
	assert.True(t, first.GetSecretSet())
	assert.False(t, first.GetSecondarySet(), "drop_previous is a no-op on the first set")
	kept := w.setSecret(t, c.GetId(), "whsec_next", false)
	assert.True(t, kept.GetSecondarySet(), "the zero value keeps the old secret")
	raw, _ := protojson.Marshal(kept)
	assert.NotContains(t, string(raw), "whsec_next")
	dropped := w.setSecret(t, c.GetId(), "whsec_next", true)
	assert.False(t, dropped.GetSecondarySet())
	assert.True(t, dropped.GetSecretSet())
}

func TestCreateConnection_NoPublicURLIsATypedError(t *testing.T) {
	w := newCPWorld(t)
	opts := cpOptions()
	opts.HooksURL = ""
	cp := channel.NewControlPlane(w.svc, w.refs, opts)
	_, err := cp.CreateConnection(w.operator(), connect.NewRequest(createConnReq(w.projectPub, w.agtPub)))
	assert.True(t, channel.IsWebhookURLUnavailableError(err), "%v", err)
	var typed *channel.WebhookURLUnavailableError
	require.ErrorAs(t, err, &typed)
	assert.Equal(t, apperror.CodeConnectionWebhookURLUnavailable, typed.ToAppError().Code())
	assert.Empty(t, w.store.Connections, "nothing is stored without a webhook URL to show")
}

func TestListConnections_ByProjectAndAgent(t *testing.T) {
	w := newCPWorld(t)
	first := w.create(t)
	other := uuid.New()
	otherPub := w.refs.Resource(other, w.org)
	req := createConnReq(w.projectPub, otherPub)
	req.DeviceId = "dev_ZZZZZZZZZZZZZZZZ"
	_, err := w.client.CreateConnection(t.Context(), connect.NewRequest(req))
	require.NoError(t, err)

	all, err := w.client.ListConnections(t.Context(), connect.NewRequest(&channelv1.ListConnectionsRequest{ProjectId: w.projectPub}))
	require.NoError(t, err)
	assert.Len(t, all.Msg.GetConnections(), 2)
	mine, err := w.client.ListConnections(t.Context(), connect.NewRequest(&channelv1.ListConnectionsRequest{ProjectId: w.projectPub, AgentId: w.agtPub}))
	require.NoError(t, err)
	require.Len(t, mine.Msg.GetConnections(), 1)
	assert.Equal(t, first.GetId(), mine.Msg.GetConnections()[0].GetId())
}

func TestDeleteConnection(t *testing.T) {
	w := newCPWorld(t)
	c := w.create(t)
	_, err := w.client.DeleteConnection(t.Context(), connect.NewRequest(&channelv1.DeleteConnectionRequest{Id: c.GetId()}))
	require.NoError(t, err)
	assert.Empty(t, w.store.Connections)
	_, err = w.cp.GetConnection(w.operator(), connect.NewRequest(&channelv1.GetConnectionRequest{Id: c.GetId()}))
	assert.True(t, channel.IsNotFoundError(err), "%v", err)
}

func TestGetChannelSettings(t *testing.T) {
	w := newCPWorld(t)
	got, err := w.client.GetChannelSettings(t.Context(), connect.NewRequest(&channelv1.GetChannelSettingsRequest{}))
	require.NoError(t, err)
	m := got.Msg
	assert.Equal(t, "https://wa.example.com", m.GetDefaultOpenwaBaseUrl())
	assert.Equal(t, channel.DefaultReplyFailure, m.GetReplyFailure())
	assert.Equal(t, channel.DefaultReplyVoice, m.GetReplyVoice())
	assert.Equal(t, channel.DefaultReplyPhotoUnreadable, m.GetReplyPhotoUnreadable())
	assert.Equal(t, int32(3000), m.GetQuietWindowMs())
	assert.Equal(t, int32(10000), m.GetQuietMaxMs())
	assert.Equal(t, int32(20), m.GetMaxMessageAgeMinutes())
	assert.Equal(t, channel.DefaultApprovalPrompt, m.GetApprovalPrompt())
	assert.Equal(t, channel.DefaultApprovalExpired, m.GetApprovalExpired())
	assert.Equal(t, int32(channel.MaxQuiet.Milliseconds()), m.GetMaxQuietMs(), "the console reads the cap instead of hard-coding it")
}

func TestUpdateConnection_ApprovalTexts(t *testing.T) {
	w := newCPWorld(t)
	c := w.create(t)
	assert.Equal(t, channel.DefaultApprovalPrompt, c.GetApprovalPrompt())
	assert.Equal(t, channel.DefaultApprovalExpired, c.GetApprovalExpired())

	got, err := w.client.UpdateConnection(t.Context(), connect.NewRequest(&channelv1.UpdateConnectionRequest{Id: c.GetId(), ApprovalPrompt: new("{preview}\n\nYA / TIDAK")}))
	require.NoError(t, err)
	out := got.Msg.GetConnection()
	assert.Equal(t, "{preview}\n\nYA / TIDAK", out.GetApprovalPrompt())
	assert.Equal(t, channel.DefaultApprovalExpired, out.GetApprovalExpired(), "unset keeps the stored text")

	got, err = w.client.UpdateConnection(t.Context(), connect.NewRequest(&channelv1.UpdateConnectionRequest{Id: c.GetId(), Enabled: new(true)}))
	require.NoError(t, err)
	assert.Equal(t, "{preview}\n\nYA / TIDAK", got.Msg.GetConnection().GetApprovalPrompt(), "an update that leaves it unset keeps it")

	_, err = w.client.UpdateConnection(t.Context(), connect.NewRequest(&channelv1.UpdateConnectionRequest{Id: c.GetId(), ApprovalExpired: new("")}))
	assert.ErrorContains(t, err, "invalid approval_expired", "an empty text is refused")
}

func TestChannelReach_IDOnlyRPCs(t *testing.T) {
	refs := fakes.NewPublicIDs()
	orgA, orgB, prjB, agentB := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	refs.Org(orgA)
	refs.Org(orgB)
	prjBPub := refs.Project(prjB, orgB)
	agentBPub := refs.Resource(agentB, orgB)
	foreign, err := channel.New(orgB, prjB, agentB, channel.Input{BaseURL: "https://wa.example.com", OpenWAOrg: "acme", OpenWAProject: "main", DeviceID: "dev_V1StGXR8Z5jdHi6B"}, time.Now())
	require.NoError(t, err)
	for name, p := range map[string]session.Principal{
		"member of A": {Source: session.SourceLocal, UserID: uuid.New(), ActiveOrgID: orgA, Scopes: []string{"*"}},
		"key of A":    {Source: session.SourceAPIKey, ActiveOrgID: orgA, AllProjects: true},
	} {
		t.Run(name, func(t *testing.T) {
			store := &fakes.ChannelStore{Connections: map[uuid.UUID]*channel.Connection{foreign.ID: foreign}}
			svc := channel.NewService(store, fakes.Transactor{}, refs, apperror.NewReporter(nil, false).Unexpected).WithSealer(testSealer(t))
			cp := channel.NewControlPlane(svc, refs, cpOptions())
			ctx := session.PrincipalInto(context.Background(), p)
			for _, id := range []string{foreign.PublicID, foreign.ID.String()} {
				_, err := cp.GetConnection(ctx, connect.NewRequest(&channelv1.GetConnectionRequest{Id: id}))
				assert.True(t, channel.IsNotFoundError(err), "Get %s: %v", id, err)
				_, err = cp.UpdateConnection(ctx, connect.NewRequest(updateReq(id)))
				assert.True(t, channel.IsNotFoundError(err), "Update %s: %v", id, err)
				_, err = cp.SetConnectionSecret(ctx, connect.NewRequest(&channelv1.SetConnectionSecretRequest{Id: id, SigningSecret: "x", DropPrevious: true}))
				assert.True(t, channel.IsNotFoundError(err), "SetSecret %s: %v", id, err)
				_, err = cp.DeleteConnection(ctx, connect.NewRequest(&channelv1.DeleteConnectionRequest{Id: id}))
				assert.True(t, channel.IsNotFoundError(err), "Delete %s: %v", id, err)
			}
			require.Len(t, store.Connections, 1)
			assert.Equal(t, "https://wa.example.com", store.Connections[foreign.ID].BaseURL, "no write")
			for _, req := range []*channelv1.CreateConnectionRequest{createConnReq(prjBPub, agentBPub), createConnReq(prjB.String(), agentB.String())} {
				_, err := cp.CreateConnection(ctx, connect.NewRequest(req))
				assert.True(t, channel.IsNotFoundError(err), "Create in %s: %v", req.GetProjectId(), err)
				_, err = cp.ListConnections(ctx, connect.NewRequest(&channelv1.ListConnectionsRequest{ProjectId: req.GetProjectId()}))
				assert.True(t, channel.IsNotFoundError(err), "List in %s: %v", req.GetProjectId(), err)
			}
			assert.Len(t, store.Connections, 1)
		})
	}
}

func TestChannelReach_AgentOfAnotherOrgIsNotFound(t *testing.T) {
	w := newCPWorld(t)
	stranger := uuid.New()
	strangerPub := w.refs.Resource(stranger, uuid.New())
	_, err := w.cp.CreateConnection(w.operator(), connect.NewRequest(createConnReq(w.projectPub, strangerPub)))
	assert.True(t, channel.IsNotFoundError(err), "%v", err)
	_, err = w.cp.ListConnections(w.operator(), connect.NewRequest(&channelv1.ListConnectionsRequest{ProjectId: w.projectPub, AgentId: strangerPub}))
	assert.True(t, channel.IsNotFoundError(err), "%v", err)
	assert.Empty(t, w.store.Connections)
}

func TestCreateConnection_NoPublicURLStillAnswersNotFoundOutsideReach(t *testing.T) {
	w := newCPWorld(t)
	opts := cpOptions()
	opts.HooksURL = ""
	cp := channel.NewControlPlane(w.svc, w.refs, opts)
	stranger := session.PrincipalInto(context.Background(), session.Principal{Source: session.SourceAPIKey, ActiveOrgID: uuid.New(), AllProjects: true})
	_, err := cp.CreateConnection(stranger, connect.NewRequest(createConnReq(w.projectPub, w.agtPub)))
	assert.True(t, channel.IsNotFoundError(err), "reach comes first: %v", err)
}

func TestUpdateConnection_UnsetAddressKeepsTheStoredOne(t *testing.T) {
	w := newCPWorld(t)
	c := w.create(t)
	id := w.idOf(c)
	key := append([]byte(nil), w.store.Connections[id].DeviceKeySealed...)
	got, err := w.client.UpdateConnection(t.Context(), connect.NewRequest(&channelv1.UpdateConnectionRequest{Id: c.GetId(), Enabled: new(false)}))
	require.NoError(t, err, "an enabled-only update from a stale page")
	out := got.Msg.GetConnection()
	assert.Equal(t, c.GetBaseUrl(), out.GetBaseUrl())
	assert.Equal(t, c.GetOpenwaOrg(), out.GetOpenwaOrg())
	assert.Equal(t, c.GetOpenwaProject(), out.GetOpenwaProject())
	assert.Equal(t, c.GetDeviceId(), out.GetDeviceId())
	assert.Equal(t, key, w.store.Connections[id].DeviceKeySealed, "the key is untouched")
}

// SECURITY: a new address with the old key would hand the stored device key to whoever runs the new base URL.
func TestUpdateConnection_NewAddressNeedsANewKey(t *testing.T) {
	w := newCPWorld(t)
	c := w.create(t)
	ctx := w.operator()
	for name, req := range map[string]*channelv1.UpdateConnectionRequest{
		"base_url":  {Id: c.GetId(), BaseUrl: new("https://elsewhere.example.com")},
		"device_id": {Id: c.GetId(), DeviceId: new("dev_ZZZZZZZZZZZZZZZZ")},
	} {
		_, err := w.cp.UpdateConnection(ctx, connect.NewRequest(req))
		assert.True(t, channel.IsSecretRequiredError(err), "%s without a key: %v", name, err)
		req.DeviceKey = new("owk_new_key")
		got, err := w.cp.UpdateConnection(ctx, connect.NewRequest(req))
		require.NoError(t, err, "%s with a key", name)
		assert.NotEmpty(t, got.Msg.GetConnection().GetId())
	}
	_, err := w.cp.UpdateConnection(ctx, connect.NewRequest(&channelv1.UpdateConnectionRequest{Id: c.GetId(), BaseUrl: new("https://elsewhere.example.com")}))
	assert.NoError(t, err, "the same base URL again is no change and needs no key")
}
