package invite_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	invitev1 "altalune.id/oagent/gen/go/invite/v1"
	invitev1connect "altalune.id/oagent/gen/go/invite/v1/invitev1connect"
	"altalune.id/oagent/internal/org/invite"
	"altalune.id/oagent/internal/prompt"
)

const (
	cliOrg    = "acmePublicId000000001"
	cliInvite = "invitePublicId0000001"
	cliLink   = "https://oagent.example.com/invite/raw-token"
)

type fakeInviteClient struct {
	invitev1connect.InviteServiceClient
	err      error
	created  []*invitev1.CreateInviteRequest
	resent   []*invitev1.ResendInviteRequest
	revoked  []*invitev1.RevokeInviteRequest
	listReqs []*invitev1.ListInvitesRequest
}

func sample(status string) *invitev1.Invite {
	at := timestamppb.New(time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC))
	return &invitev1.Invite{
		Id: cliInvite, Email: "bob@x.io", Role: "admin", Status: status, InvitedByName: "Olga", ExpiresAt: at, CreatedAt: at,
	}
}

func (f *fakeInviteClient) CreateInvite(_ context.Context, req *connect.Request[invitev1.CreateInviteRequest]) (*connect.Response[invitev1.CreateInviteResponse], error) {
	f.created = append(f.created, req.Msg)
	if f.err != nil {
		return nil, f.err
	}
	return connect.NewResponse(&invitev1.CreateInviteResponse{Invite: sample("pending"), Link: cliLink}), nil
}

func (f *fakeInviteClient) ResendInvite(_ context.Context, req *connect.Request[invitev1.ResendInviteRequest]) (*connect.Response[invitev1.ResendInviteResponse], error) {
	f.resent = append(f.resent, req.Msg)
	if f.err != nil {
		return nil, f.err
	}
	return connect.NewResponse(&invitev1.ResendInviteResponse{Invite: sample("pending"), Link: cliLink + "-2"}), nil
}

func (f *fakeInviteClient) RevokeInvite(_ context.Context, req *connect.Request[invitev1.RevokeInviteRequest]) (*connect.Response[invitev1.RevokeInviteResponse], error) {
	f.revoked = append(f.revoked, req.Msg)
	if f.err != nil {
		return nil, f.err
	}
	return connect.NewResponse(&invitev1.RevokeInviteResponse{}), nil
}

func (f *fakeInviteClient) ListInvites(_ context.Context, req *connect.Request[invitev1.ListInvitesRequest]) (*connect.Response[invitev1.ListInvitesResponse], error) {
	f.listReqs = append(f.listReqs, req.Msg)
	if f.err != nil {
		return nil, f.err
	}
	return connect.NewResponse(&invitev1.ListInvitesResponse{Invites: []*invitev1.Invite{sample("pending")}, Drafts: []*invitev1.Invite{sample("draft")}}), nil
}

func runInvite(t *testing.T, client invitev1connect.InviteServiceClient, stdin io.Reader, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	buf := new(bytes.Buffer)
	root := &cobra.Command{Use: "x", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().String("output", "", "")
	root.AddGroup(&cobra.Group{ID: "tenancy", Title: "Tenancy:"})
	root.AddCommand(invite.NewCmd(func(*cobra.Command) (invitev1connect.InviteServiceClient, error) { return client, nil }))
	root.SetOut(buf)
	root.SetErr(buf)
	if stdin != nil {
		root.SetIn(stdin)
	}
	root.SetArgs(append([]string{"invite"}, args...))
	return buf, root.ExecuteContext(t.Context())
}

func TestInviteCreate_Text(t *testing.T) {
	c := &fakeInviteClient{}
	buf, err := runInvite(t, c, nil, "create", "--org-id", cliOrg, "--email", "bob@x.io", "--role", "admin", "--output", "text")
	require.NoError(t, err)
	assert.Equal(t, "Invite sent to bob@x.io. Share this link if the email doesn't arrive: "+cliLink+"\n", buf.String())
	require.Len(t, c.created, 1)
	assert.Equal(t, cliOrg, c.created[0].GetOrgId())
	assert.Equal(t, "admin", c.created[0].GetRole())
}

func TestInviteCreate_JSONHasTheLink(t *testing.T) {
	buf, err := runInvite(t, &fakeInviteClient{}, nil, "create", "--org-id", cliOrg, "--email", "bob@x.io", "--output", "json")
	require.NoError(t, err)
	assert.Contains(t, buf.String(), `"link": "`+cliLink+`"`)
	assert.Contains(t, buf.String(), `"id": "`+cliInvite+`"`)
}

func TestInviteCreate_DefaultsToMember(t *testing.T) {
	c := &fakeInviteClient{}
	_, err := runInvite(t, c, nil, "create", "--org-id", cliOrg, "--email", "bob@x.io")
	require.NoError(t, err)
	assert.Equal(t, "member", c.created[0].GetRole())
}

func TestInviteList(t *testing.T) {
	buf, err := runInvite(t, &fakeInviteClient{}, nil, "list", "--org-id", cliOrg, "--output", "text")
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "INVITED BY")
	assert.Contains(t, out, "pending")
	assert.Contains(t, out, "draft")
	assert.Contains(t, out, "2026-10-20T09:00:00Z")

	buf, err = runInvite(t, &fakeInviteClient{}, nil, "list", "--org-id", cliOrg, "--output", "json")
	require.NoError(t, err)
	assert.NotContains(t, buf.String(), `"link"`, "a listing never carries a link")
	assert.Equal(t, 2, strings.Count(buf.String(), `"id"`))
}

func TestInviteResend(t *testing.T) {
	c := &fakeInviteClient{}
	buf, err := runInvite(t, c, nil, "resend", "--org-id", cliOrg, "--id", cliInvite, "--output", "text")
	require.NoError(t, err)
	assert.Contains(t, buf.String(), cliLink+"-2")
	assert.Equal(t, cliInvite, c.resent[0].GetInviteId())
}

func TestInviteRevoke_Asks(t *testing.T) {
	c := &fakeInviteClient{}
	buf, err := runInvite(t, c, strings.NewReader("y\n"), "revoke", "--org-id", cliOrg, "--id", cliInvite)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Cancel invite "+cliInvite+"? [y/N]")
	assert.Contains(t, buf.String(), "Invite "+cliInvite+" cancelled")
	require.Len(t, c.revoked, 1)

	c = &fakeInviteClient{}
	_, err = runInvite(t, c, strings.NewReader("n\n"), "revoke", "--org-id", cliOrg, "--id", cliInvite)
	assert.ErrorIs(t, err, prompt.ErrCancelled)
	assert.Empty(t, c.revoked)

	_, err = runInvite(t, c, nil, "revoke", "--org-id", cliOrg, "--id", cliInvite, "--yes")
	require.NoError(t, err)
	assert.Len(t, c.revoked, 1)
}

func TestInvite_HelpSaysPublicID(t *testing.T) {
	buf, err := runInvite(t, &fakeInviteClient{}, nil, "resend", "--help")
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Organization public id")
	assert.Contains(t, buf.String(), "Invite public id")
}

func TestInvite_Errors(t *testing.T) {
	boom := errors.New("boom")
	c := &fakeInviteClient{err: boom}
	for _, args := range [][]string{
		{"create", "--org-id", cliOrg, "--email", "bob@x.io"},
		{"list", "--org-id", cliOrg},
		{"resend", "--org-id", cliOrg, "--id", cliInvite},
		{"revoke", "--org-id", cliOrg, "--id", cliInvite, "--yes"},
	} {
		_, err := runInvite(t, c, nil, args...)
		assert.ErrorIs(t, err, boom, args)
	}
	failing := func(*cobra.Command) (invitev1connect.InviteServiceClient, error) { return nil, boom }
	for _, args := range [][]string{
		{"create", "--org-id", cliOrg, "--email", "bob@x.io"},
		{"list", "--org-id", cliOrg},
		{"resend", "--org-id", cliOrg, "--id", cliInvite},
		{"revoke", "--org-id", cliOrg, "--id", cliInvite, "--yes"},
	} {
		root := &cobra.Command{Use: "x", SilenceUsage: true, SilenceErrors: true}
		root.AddGroup(&cobra.Group{ID: "tenancy", Title: "Tenancy:"})
		root.AddCommand(invite.NewCmd(failing))
		root.SetArgs(append([]string{"invite"}, args...))
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		assert.ErrorIs(t, root.ExecuteContext(t.Context()), boom, args)
	}
}
