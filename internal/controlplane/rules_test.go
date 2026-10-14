package controlplane

import (
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/known/timestamppb"

	"altalune.id/oagent/apperror"
	agentv1 "altalune.id/oagent/gen/go/agent/v1"
	apikeyv1 "altalune.id/oagent/gen/go/apikey/v1"
	authv1 "altalune.id/oagent/gen/go/auth/v1"
	invitev1 "altalune.id/oagent/gen/go/invite/v1"
	memberv1 "altalune.id/oagent/gen/go/member/v1"
	orgv1 "altalune.id/oagent/gen/go/org/v1"
	projectv1 "altalune.id/oagent/gen/go/project/v1"
	webhookv1 "altalune.id/oagent/gen/go/webhook/v1"
	"altalune.id/oagent/internal/permission"
	"altalune.id/oagent/nanoid"
)

//nolint:gochecknoglobals // a fixed allowlist.
var unruled = map[string]string{
	"auth.v1.WhoamiRequest":            "empty request",
	"auth.v1.SignOutEverywhereRequest": "empty request",
	"org.v1.ListOrgsRequest":           "empty request",

	"apikey.v1.CreateAPIKeyRequest.all_projects":    "a bool; the domain checks it against project_ids (KEY005)",
	"webhook.v1.UpdateWebhookRequest.active":        "a bool; false deactivates",
	"webhook.v1.ListWebhookDeliveriesRequest.limit": "any int32 works today: clamped to 1..50",
}

// SECURITY: every RPC request is checked before its handler; a field with no rule must be allowlisted with a reason.
func TestEveryRequestFieldHasARule(t *testing.T) {
	var missing []string
	for procedure := range permission.Table() {
		service, method, ok := strings.Cut(strings.TrimPrefix(procedure, "/"), "/")
		require.True(t, ok, procedure)
		desc, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(service + "." + method))
		require.NoError(t, err, procedure)
		md, ok := desc.(protoreflect.MethodDescriptor)
		require.True(t, ok, procedure)

		input := md.Input()
		name := string(input.FullName())
		if _, ok := unruled[name]; ok {
			assert.Zero(t, input.Fields().Len(), "%s is allowlisted as empty but has fields", name)
			continue
		}
		for i := range input.Fields().Len() {
			field := input.Fields().Get(i)
			key := name + "." + string(field.Name())
			if _, ok := unruled[key]; ok {
				continue
			}
			if !hasRule(field) {
				missing = append(missing, key)
			}
		}
	}
	slices.Sort(missing)
	assert.Empty(t, missing, "request fields with no buf.validate rule; add one or allowlist it with a reason")
}

func TestUnruledListsOnlyRealFields(t *testing.T) {
	for key := range unruled {
		if desc, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(key)); err == nil {
			_, isMsg := desc.(protoreflect.MessageDescriptor)
			_, isField := desc.(protoreflect.FieldDescriptor)
			assert.True(t, isMsg || isField, key)
			continue
		}
		t.Errorf("%s is allowlisted but does not exist", key)
	}
}

func customRuleIDs(t *testing.T) []string {
	t.Helper()
	seen := map[string]bool{}
	for procedure := range permission.Table() {
		service, method, _ := strings.Cut(strings.TrimPrefix(procedure, "/"), "/")
		desc, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(service + "." + method))
		require.NoError(t, err, procedure)
		md, ok := desc.(protoreflect.MethodDescriptor)
		require.True(t, ok, procedure)
		for i := range md.Input().Fields().Len() {
			rules := fieldRules(md.Input().Fields().Get(i))
			for _, r := range [...]*validate.FieldRules{rules, rules.GetRepeated().GetItems()} {
				for _, cel := range r.GetCel() {
					seen[cel.GetId()] = true
				}
				for _, cel := range predefinedRules(r.GetString()) {
					seen[cel.GetId()] = true
				}
			}
		}
	}
	return slices.Sorted(maps.Keys(seen))
}

func predefinedRules(r *validate.StringRules) []*validate.Rule {
	var out []*validate.Rule
	if r == nil {
		return nil
	}
	r.ProtoReflect().Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		if !fd.IsExtension() || fd.Kind() != protoreflect.BoolKind || !v.Bool() {
			return true
		}
		if p, ok := proto.GetExtension(fd.Options(), validate.E_Predefined).(*validate.PredefinedRules); ok {
			out = append(out, p.GetCel()...)
		}
		return true
	})
	return out
}

func fieldRules(field protoreflect.FieldDescriptor) *validate.FieldRules {
	opts := field.Options()
	if opts == nil || !proto.HasExtension(opts, validate.E_Field) {
		return nil
	}
	rules, _ := proto.GetExtension(opts, validate.E_Field).(*validate.FieldRules)
	return rules
}

func hasRule(field protoreflect.FieldDescriptor) bool {
	opts := field.Options()
	if opts == nil || !proto.HasExtension(opts, validate.E_Field) {
		return false
	}
	rules, ok := proto.GetExtension(opts, validate.E_Field).(*validate.FieldRules)
	if !ok || rules == nil {
		return false
	}
	return rules.GetRequired() || len(rules.GetCel()) > 0 || rules.GetType() != nil
}

const goodID = "abcdefghjkmnpqrstuvwx"

type ruleCase struct {
	name   string
	msg    proto.Message
	field  string
	reason apperror.FieldReason
}

func TestRequestRules_Accept(t *testing.T) {
	v := newTestValidator(t)
	future := timestamppb.New(time.Now().Add(time.Hour))
	accepted := []struct {
		name string
		msg  proto.Message
	}{
		{"public id", &orgv1.GetOrgRequest{Id: goodID}},
		{"padded name the domain trims", &orgv1.UpdateOrgRequest{Id: goodID, Name: "  " + strings.Repeat("é", 200) + "  ", Slug: "acme"}},
		{"uppercase email", &invitev1.DraftInviteRequest{OrgId: goodID, Email: "  Dina@Example.COM "}},
		{"unicode email", &invitev1.DraftInviteRequest{OrgId: goodID, Email: "dïna@exämple.com"}},
		{"dotless email", &invitev1.DraftInviteRequest{OrgId: goodID, Email: "dina@localhost"}},
		{"domain-literal email", &invitev1.DraftInviteRequest{OrgId: goodID, Email: "dina@[1.2.3.4]"}},
		{"254-character email", &invitev1.DraftInviteRequest{OrgId: goodID, Email: strings.Repeat("a", 242) + "@example.com"}},
		{"each role", &invitev1.CreateInviteRequest{OrgId: goodID, Email: "a@b.c", Role: "admin"}},
		{"org key", &apikeyv1.CreateAPIKeyRequest{OrgId: goodID, Kind: "org", Name: " ci ", Scopes: []string{"agent:read", "agent:read"}, ProjectIds: []string{goodID}, ExpiresAt: future}},
		{"project key", &apikeyv1.CreateAPIKeyRequest{OrgId: goodID, ProjectId: goodID, Kind: "project", Name: strings.Repeat("k", 100), ResourceIds: []string{goodID}, ExpiresAt: future}},
		{"bare hint", &apikeyv1.RevokeAPIKeyRequest{OrgId: goodID, Hint: "a_9-"}},
		{"hint as shown", &apikeyv1.RevokeAPIKeyRequest{OrgId: goodID, Hint: " …...Zz09 "}},
		{"hint with dots", &apikeyv1.RevokeAPIKeyRequest{OrgId: goodID, Hint: "...Zz09"}},
		{"blank profile name clears it", &authv1.UpdateProfileRequest{Name: "   "}},
		{"256-character password", &authv1.ChangePasswordRequest{CurrentPassword: "x", NewPassword: strings.Repeat("ü", 256)}},
		{"12-character password counted in runes", &authv1.ChangePasswordRequest{CurrentPassword: "x", NewPassword: strings.Repeat("ü", 12)}},
		{"common password is the domain's", &authv1.ChangePasswordRequest{CurrentPassword: "x", NewPassword: "password1234"}},
		{"padded description", &agentv1.UpdateAgentRequest{Id: goodID, Name: "Bot", Description: " " + strings.Repeat("d", 2000) + " "}},
		{"webhook url url.Parse accepts", &webhookv1.CreateWebhookRequest{OrgId: goodID, ProjectId: goodID, Url: " HTTPS://example.com/a b/ü ", EventTypes: []string{"agent.created"}}},
		{"duplicate events are the domain's", &webhookv1.UpdateWebhookRequest{OrgId: goodID, ProjectId: goodID, Id: goodID, Url: "http://[::1]:8080/x", EventTypes: []string{"agent.created", "agent.created"}}},
		{"any delivery limit", &webhookv1.ListWebhookDeliveriesRequest{OrgId: goodID, ProjectId: goodID, Id: goodID, Limit: -5}},
		{"webhook description of 200", &webhookv1.CreateWebhookRequest{OrgId: goodID, ProjectId: goodID, Url: "https://e.com", Description: strings.Repeat("ß", 200), EventTypes: []string{"member.added"}}},
		{"empty slug on create mints one", &orgv1.CreateOrgRequest{Name: "Acme"}},
		{"shortest slug", &projectv1.CreateProjectRequest{OrgId: goodID, Name: "Web", Slug: "a-1"}},
		{"longest slug", &agentv1.CreateAgentRequest{OrgId: goodID, ProjectId: goodID, Name: "Bot", Slug: strings.Repeat("a", 64)}},
		{"reserved slug is the domain's", &orgv1.UpdateOrgRequest{Id: goodID, Name: "Acme", Slug: "new"}},
	}
	for _, tt := range accepted {
		t.Run(tt.name, func(t *testing.T) {
			assert.NoError(t, v.Validate(tt.msg))
		})
	}
}

func TestRequestRules_Refuse(t *testing.T) {
	v := newTestValidator(t)
	future := timestamppb.New(time.Now().Add(time.Hour))
	refused := []ruleCase{
		{"missing id", &orgv1.GetOrgRequest{}, "id", apperror.FieldRequired},
		{"short id", &orgv1.GetOrgRequest{Id: "abc"}, "id", apperror.FieldInvalidFormat},
		{"uuid id", &orgv1.GetOrgRequest{Id: "4b0b0a0e-1d2c-4f7a-9a6b-1f0e2d3c4b5a"}, "id", apperror.FieldInvalidFormat},
		{"id outside the alphabet", &projectv1.GetProjectRequest{Id: "abcdefghjkmnpqrstuv-_"}, "id", apperror.FieldInvalidFormat},
		{"blank org id", &invitev1.ListInvitesRequest{OrgId: "   "}, "org_id", apperror.FieldInvalidFormat},
		{"missing org id", &invitev1.ListInvitesRequest{}, "org_id", apperror.FieldRequired},
		{"missing invite id", &invitev1.RevokeInviteRequest{OrgId: goodID}, "invite_id", apperror.FieldRequired},
		{"missing name", &projectv1.CreateProjectRequest{OrgId: goodID}, "name", apperror.FieldRequired},
		{"blank name", &agentv1.CreateAgentRequest{OrgId: goodID, ProjectId: goodID, Name: " \t "}, "name", apperror.FieldRequired},
		{"201-character name", &orgv1.UpdateOrgRequest{Id: goodID, Name: strings.Repeat("a", 201), Slug: "acme"}, "name", apperror.FieldTooLong},
		{"missing email", &invitev1.DraftInviteRequest{OrgId: goodID}, "email", apperror.FieldRequired},
		{"not an email", &invitev1.DraftInviteRequest{OrgId: goodID, Email: "dina"}, "email", apperror.FieldInvalidFormat},
		{"spaced email", &invitev1.DraftInviteRequest{OrgId: goodID, Email: "di na@x.io"}, "email", apperror.FieldInvalidFormat},
		{"255-character email", &invitev1.DraftInviteRequest{OrgId: goodID, Email: strings.Repeat("a", 243) + "@example.com"}, "email", apperror.FieldTooLong},
		{"unknown role", &memberv1.SetMemberRoleRequest{OrgId: goodID, UserId: goodID, Role: "Owner"}, "role", apperror.FieldNotAllowed},
		{"missing role", &invitev1.CreateInviteRequest{OrgId: goodID, Email: "a@b.c"}, "role", apperror.FieldRequired},
		{"unknown kind", &apikeyv1.CreateAPIKeyRequest{OrgId: goodID, Kind: "admin", Name: "ci", ExpiresAt: future}, "kind", apperror.FieldNotAllowed},
		{"blank key name", &apikeyv1.CreateAPIKeyRequest{OrgId: goodID, Kind: "org", Name: "  ", ExpiresAt: future}, "name", apperror.FieldRequired},
		{"101-character key name", &apikeyv1.CreateAPIKeyRequest{OrgId: goodID, Kind: "org", Name: strings.Repeat("k", 101), ExpiresAt: future}, "name", apperror.FieldTooLong},
		{"scope shape", &apikeyv1.CreateAPIKeyRequest{OrgId: goodID, Kind: "org", Name: "ci", Scopes: []string{"agent:*"}, ExpiresAt: future}, "scopes[0]", apperror.FieldInvalidFormat},
		{"project ids are public ids", &apikeyv1.CreateAPIKeyRequest{OrgId: goodID, Kind: "org", Name: "ci", ProjectIds: []string{goodID, "x"}, ExpiresAt: future}, "project_ids[1]", apperror.FieldInvalidFormat},
		{"missing expiry", &apikeyv1.CreateAPIKeyRequest{OrgId: goodID, Kind: "org", Name: "ci"}, "expires_at", apperror.FieldRequired},
		{"hint shape", &apikeyv1.RevokeAPIKeyRequest{OrgId: goodID, Hint: "abcde"}, "hint", apperror.FieldInvalidFormat},
		{"201-character profile name", &authv1.UpdateProfileRequest{Name: strings.Repeat("n", 201)}, "name", apperror.FieldTooLong},
		{"missing current password", &authv1.ChangePasswordRequest{NewPassword: "a long enough one"}, "current_password", apperror.FieldRequired},
		{"257-character password", &authv1.ChangePasswordRequest{CurrentPassword: "x", NewPassword: strings.Repeat("p", 257)}, "new_password", apperror.FieldTooLong},
		{"2001-character description", &agentv1.CreateAgentRequest{OrgId: goodID, ProjectId: goodID, Name: "Bot", Description: strings.Repeat("d", 2001)}, "description", apperror.FieldTooLong},
		{"missing url", &webhookv1.CreateWebhookRequest{OrgId: goodID, ProjectId: goodID, EventTypes: []string{"agent.created"}}, "url", apperror.FieldRequired},
		{"relative url", &webhookv1.CreateWebhookRequest{OrgId: goodID, ProjectId: goodID, Url: "/hook", EventTypes: []string{"agent.created"}}, "url", apperror.FieldInvalidFormat},
		{"url without host", &webhookv1.CreateWebhookRequest{OrgId: goodID, ProjectId: goodID, Url: "https:///x", EventTypes: []string{"agent.created"}}, "url", apperror.FieldInvalidFormat},
		{"2049-character url", &webhookv1.CreateWebhookRequest{OrgId: goodID, ProjectId: goodID, Url: "https://e.com/" + strings.Repeat("u", 2035), EventTypes: []string{"agent.created"}}, "url", apperror.FieldTooLong},
		{"201-character webhook description", &webhookv1.CreateWebhookRequest{OrgId: goodID, ProjectId: goodID, Url: "https://e.com", Description: strings.Repeat("d", 201), EventTypes: []string{"agent.created"}}, "description", apperror.FieldTooLong},
		{"no event types", &webhookv1.CreateWebhookRequest{OrgId: goodID, ProjectId: goodID, Url: "https://e.com"}, "event_types", apperror.FieldRequired},
		{"event type shape", &webhookv1.UpdateWebhookRequest{OrgId: goodID, ProjectId: goodID, Id: goodID, Url: "https://e.com", EventTypes: []string{"Agent Created"}}, "event_types[0]", apperror.FieldInvalidFormat},
		{"short new password", &authv1.ChangePasswordRequest{CurrentPassword: "x", NewPassword: "short"}, "new_password", apperror.FieldTooShort},
		{"11-character password counted in runes", &authv1.ChangePasswordRequest{CurrentPassword: "x", NewPassword: strings.Repeat("ü", 11)}, "new_password", apperror.FieldTooShort},
		{"slug outside the alphabet", &orgv1.UpdateOrgRequest{Id: goodID, Name: "Acme", Slug: "NOT A SLUG"}, "slug", apperror.FieldInvalidFormat},
		{"slug ending in a dash", &projectv1.UpdateProjectRequest{Id: goodID, Name: "Web", Slug: "web-"}, "slug", apperror.FieldInvalidFormat},
		{"padded slug", &orgv1.CreateOrgRequest{Name: "Acme", Slug: " acme "}, "slug", apperror.FieldInvalidFormat},
		{"2-character slug", &projectv1.CreateProjectRequest{OrgId: goodID, Name: "Web", Slug: "ab"}, "slug", apperror.FieldTooShort},
		{"65-character slug", &agentv1.CreateAgentRequest{OrgId: goodID, ProjectId: goodID, Name: "Bot", Slug: strings.Repeat("a", 65)}, "slug", apperror.FieldTooLong},
		{"missing slug on update", &orgv1.UpdateOrgRequest{Id: goodID, Name: "Acme"}, "slug", apperror.FieldRequired},
		{"delivery id", &webhookv1.RetryWebhookDeliveryRequest{OrgId: goodID, ProjectId: goodID, Id: goodID, DeliveryId: "x"}, "delivery_id", apperror.FieldInvalidFormat},
	}
	for _, tt := range refused {
		t.Run(tt.name, func(t *testing.T) {
			ae, ok := apperror.AsAppError(v.Validate(tt.msg))
			require.True(t, ok, "want a refusal")
			assert.Contains(t, ae.Violations(), apperror.Violation{Field: tt.field, Reason: tt.reason, Param: paramOf(ae.Violations(), tt.field)})
			assert.Len(t, ae.Violations(), 1, "%v", ae.Violations())
		})
	}
}

func paramOf(vs []apperror.Violation, field string) string {
	for _, v := range vs {
		if v.Field == field {
			return v.Param
		}
	}
	return ""
}

func TestRequestRules_ParamsCarryTheLimit(t *testing.T) {
	v := newTestValidator(t)
	limits := map[string]proto.Message{
		"200":  &orgv1.UpdateOrgRequest{Id: goodID, Name: strings.Repeat("a", 201), Slug: "acme"},
		"254":  &invitev1.DraftInviteRequest{OrgId: goodID, Email: strings.Repeat("a", 243) + "@example.com"},
		"100":  &apikeyv1.CreateAPIKeyRequest{OrgId: goodID, Kind: "org", Name: strings.Repeat("k", 101), ExpiresAt: timestamppb.Now()},
		"2000": &agentv1.CreateAgentRequest{OrgId: goodID, ProjectId: goodID, Name: "Bot", Description: strings.Repeat("d", 2001)},
		"256":  &authv1.ChangePasswordRequest{CurrentPassword: "x", NewPassword: strings.Repeat("p", 257)},
		"12":   &authv1.ChangePasswordRequest{CurrentPassword: "x", NewPassword: "short"},
		"3":    &projectv1.CreateProjectRequest{OrgId: goodID, Name: "Web", Slug: "ab"},
		"64":   &orgv1.UpdateOrgRequest{Id: goodID, Name: "Acme", Slug: strings.Repeat("a", 65)},
	}
	for want, msg := range limits {
		ae, ok := apperror.AsAppError(v.Validate(msg))
		require.True(t, ok, want)
		assert.Equal(t, want, ae.Violations()[0].Param)
	}
}

// SECURITY: oagent.public_id must accept every id nanoid mints, or a real record could never be named.
func TestPublicIDRule_AcceptsEveryMintedID(t *testing.T) {
	v := newTestValidator(t)
	for range 2000 {
		id, err := nanoid.New(21)
		require.NoError(t, err)
		require.NoError(t, v.Validate(&orgv1.GetOrgRequest{Id: id}), id)
	}
	for _, outside := range []string{"0", "1", "i", "l", "o", "I", "L", "O", "-", "_"} {
		ae, ok := apperror.AsAppError(v.Validate(&orgv1.GetOrgRequest{Id: goodID[:20] + outside}))
		require.True(t, ok, outside)
		assert.Equal(t, apperror.FieldInvalidFormat, ae.Violations()[0].Reason, outside)
	}
}
