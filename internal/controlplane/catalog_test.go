package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	apperrorv1 "altalune.id/oagent/gen/go/apperror/v1"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/apikey"
	"altalune.id/oagent/internal/auth"
	"altalune.id/oagent/internal/install"
	"altalune.id/oagent/internal/kernel/authn"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/user"
	"altalune.id/oagent/internal/webhook"
	"altalune.id/oagent/password"
	"altalune.id/oagent/scope"
)

func errOf[T any](_ T, err error) error { return err }

func samples() []error {
	nobody := uuid.New()
	hook := []events.Type{events.AgentCreated}
	return []error{
		&tenant.MissingError{},
		&tenant.UnscopedError{},
		&authn.UnauthorizedError{},
		apperror.Invalid("organization_id", apperror.FieldInvalid),
		apperror.Invalid("owner", apperror.FieldRequired),
		errOf(member.New(uuid.Nil, nobody, member.RoleOwner)),
		apperror.New(apperror.CodeNotFound, "Not found"),
		apperror.New(apperror.CodeAlreadyExists, "Already exists"),
		&authz.ForbiddenError{Permission: scope.AgentWrite},
		&tenant.ProjectOutsideOrgError{},
		apperror.NewReporter(nil, false).Unexpected(context.Background(), "boom", errors.New("secret detail")),

		&org.NotFoundError{Slug: "acme"},
		&org.AlreadyExistsError{Slug: "acme"},
		errOf(org.NewOrg("a", "Acme", nobody)),
		errOf(org.NewOrg("Not_A_Slug", "Acme", nobody)),
		errOf(org.NewOrg("new", "Acme", nobody)),
		errOf(org.NewOrg("acme", "", nobody)),
		&org.CreationDisabledError{},

		&project.NotFoundError{Slug: "web"},
		&project.AlreadyExistsError{Slug: "web"},
		errOf(project.NewProject(nobody, "a", "Web", nobody)),
		errOf(project.NewProject(nobody, "Not_A_Slug", "Web", nobody)),
		errOf(project.NewProject(nobody, "web", "", nobody)),

		&agent.NotFoundError{Slug: "bot"},
		&agent.AlreadyExistsError{Slug: "bot"},
		errOf(agent.New(nobody, nobody, "a", "Bot", "", nobody)),
		errOf(agent.New(nobody, nobody, "Not_A_Slug", "Bot", "", nobody)),
		errOf(agent.New(nobody, nobody, "bot", "", "", nobody)),
		errOf(agent.New(nobody, nobody, "bot", "Bot", strings.Repeat("d", 2001), nobody)),

		&apikey.NotFoundError{ID: "x"},
		apikey.ValidateName(""),
		apikey.ValidateName(strings.Repeat("n", 101)),
		&apikey.UnknownScopeError{Scope: "nope:read"},
		&apikey.ScopeLevelError{Scope: scope.OrgWrite},
		&apikey.ExpiryRequiredError{},
		&apikey.ExpiryInPastError{},
		&apikey.ExpiryTooLongError{},
		&apikey.GrantConflictError{},
		&apikey.EmptyGrantError{},
		&apikey.GrantOutsideOrgError{},
		&apikey.PersonOnlyScopeError{Scope: scope.MemberWrite},
		&apikey.ScopeExceedsRoleError{Scope: scope.OrgWrite},
		&apikey.ScopeExceedsRoleError{},
		&apikey.PersonalKeysDisabledError{},

		&user.NotFoundError{},
		&user.AlreadyExistsError{Email: "someone@example.com"},
		errOf(user.New("not-an-email", "Someone")),
		errOf(user.New("someone@example.com", strings.Repeat("n", 201))),

		&member.NotFoundError{},
		&member.AlreadyMemberError{OrgID: nobody, UserID: nobody},
		errOf(member.New(nobody, nobody, member.Role("root"))),

		&auth.InvalidCredentialsError{},
		password.CheckPolicy("short"),

		&install.AlreadyInstalledError{},
		apperror.New(apperror.CodeInstallRequired, "Setup required"),

		&webhook.NotFoundError{ID: "x"},
		errOf(webhook.New(nobody, nobody, "", "", hook)),
		errOf(webhook.New(nobody, nobody, "https://e.com/"+strings.Repeat("a", webhook.MaxURLLen), "", hook)),
		errOf(webhook.New(nobody, nobody, "https://e.com/\xff", "", hook)),
		errOf(webhook.New(nobody, nobody, "https://e.com/%zz", "", hook)),
		errOf(webhook.New(nobody, nobody, "ftp://e.com", "", hook)),
		errOf(webhook.New(nobody, nobody, "https://u:p@e.com", "", hook)),
		errOf(webhook.New(nobody, nobody, "https://", "", hook)),
		errOf(webhook.New(nobody, nobody, "https://e.com", "", nil)),
		errOf(webhook.New(nobody, nobody, "https://e.com", "", []events.Type{"x.y"})),
		errOf(webhook.New(nobody, nobody, "https://e.com", "", []events.Type{events.WebhookPing})),
		errOf(webhook.New(nobody, nobody, "https://e.com", "", []events.Type{events.AgentCreated, events.AgentCreated})),
		errOf(webhook.New(nobody, nobody, "https://e.com", "\xff", hook)),
		errOf(webhook.New(nobody, nobody, "https://e.com", strings.Repeat("d", webhook.MaxDescriptionRunes+1), hook)),
		&webhook.EndpointLimitError{Limit: 3},
		&webhook.EndpointInactiveError{ID: "x"},
		&webhook.SecretConflictError{ID: "x"},
		&webhook.DeliveryNotFoundError{ID: "x"},
		&webhook.DeliveryNotRetryableError{ID: "x"},
	}
}

var diagnostic = []string{"trace_id", "request_id", "underlying_error"}

type catalogEntry struct {
	Code    string   `json:"code"`
	Params  []string `json:"params"`
	Reasons []string `json:"reasons"`
}

func catalog(t *testing.T) []catalogEntry {
	t.Helper()
	raw, err := os.ReadFile("../../apperror/codes.json")
	require.NoError(t, err)
	var entries []catalogEntry
	require.NoError(t, json.Unmarshal(raw, &entries))
	require.NotEmpty(t, entries)
	return entries
}

func wireDetail(t *testing.T, err error) (*apperrorv1.ErrorDetail, error) {
	t.Helper()
	unexpected := func(context.Context, string, error, ...any) *apperror.AppError {
		t.Errorf("%T reached the wire as an unexpected error", err)
		return apperror.New(apperror.CodeUnexpected, "reported")
	}
	out := Translate(context.Background(), err, unexpected)
	var cerr *connect.Error
	require.ErrorAs(t, out, &cerr)
	require.Len(t, cerr.Details(), 1, "%T: no ErrorDetail", err)
	value, verr := cerr.Details()[0].Value()
	require.NoError(t, verr)
	det, ok := value.(*apperrorv1.ErrorDetail)
	require.True(t, ok)
	ae, _ := apperror.AsAppError(err)
	return det, ae.Unwrap()
}

func TestEveryCodeSendsItsCatalogParams(t *testing.T) {
	// A code's samples together send every param and reason it declares; a sample without meta.reason gets the code's default message.
	entries := catalog(t)
	byCode := map[string]catalogEntry{}
	for _, e := range entries {
		byCode[e.Code] = e
	}

	sent := map[string]map[string]bool{}
	for _, err := range samples() {
		require.Error(t, err)
		det, cause := wireDetail(t, err)
		code := det.GetCode()
		entry, ok := byCode[code]
		require.True(t, ok, "%s (from %T) is not in codes.json", code, err)
		if sent[code] == nil {
			sent[code] = map[string]bool{}
		}

		meta := det.GetMeta()
		if reason, ok := meta["reason"]; ok {
			assert.Contains(t, entry.Reasons, reason, "%s from %T: meta.reason", code, err)
			sent[code]["reason="+reason] = true
		}
		for k, v := range meta {
			switch {
			case k == "reason":
			case code == string(apperror.CodeUnexpected) && slices.Contains(diagnostic, k):
				// Operator diagnostics: the console never shows them, so the catalog doesn't declare them.
			default:
				assert.Contains(t, entry.Params, k, "%s from %T sends %q, which codes.json does not declare", code, err, k)
			}
			sent[code][k] = true
			if cause != nil && code != string(apperror.CodeUnexpected) {
				assert.NotEqual(t, cause.Error(), v, "%s: meta %q repeats the cause", code, k)
			}
		}
	}

	for _, e := range entries {
		got, ok := sent[e.Code]
		if !assert.True(t, ok, "%s has no sample error", e.Code) {
			continue
		}
		for _, p := range e.Params {
			assert.True(t, got[p], "%s: no sample sends param %q", e.Code, p)
		}
		for _, r := range e.Reasons {
			assert.True(t, got["reason="+r], "%s: no sample sends reason %q", e.Code, r)
		}
	}
}

func TestMetaNeverEchoesAnEmail(t *testing.T) {
	err := &user.AlreadyExistsError{Email: "someone@example.com"}
	det, _ := wireDetail(t, err)
	for _, v := range det.GetMeta() {
		assert.NotContains(t, v, "@")
	}
	assert.NotContains(t, Translate(context.Background(), err, nil).Error(), "@")
}

func TestEchoedInputIsCapped(t *testing.T) {
	long := strings.Repeat("x", 10*apperror.EchoLimit)
	for _, err := range []error{
		&webhook.InvalidEventTypesError{Event: events.Type(long), Reason: webhook.EventTypeUnknown},
		&webhook.InvalidEventTypesError{Event: events.Type(long), Reason: webhook.EventTypeNotSubscribable},
		&webhook.InvalidEventTypesError{Event: events.Type(long), Reason: webhook.EventTypeDuplicate},
		&apikey.UnknownScopeError{Scope: long},
		&apikey.ScopeLevelError{Scope: long},
		&apikey.PersonOnlyScopeError{Scope: long},
		&apikey.ScopeExceedsRoleError{Scope: long},
		&member.InvalidRoleError{Role: long},
		&org.InvalidSlugError{Slug: long, Reason: org.SlugReserved},
		&org.InvalidSlugError{Slug: long, Reason: "other"},
		&project.InvalidSlugError{Slug: long, Reason: "other"},
		&agent.InvalidSlugError{Slug: long, Reason: "other"},
	} {
		det, _ := wireDetail(t, err)
		for k, v := range det.GetMeta() {
			assert.LessOrEqual(t, len(v), apperror.EchoLimit, "%T: meta %q", err, k)
		}
		var cerr *connect.Error
		require.ErrorAs(t, Translate(context.Background(), err, nil), &cerr)
		assert.Less(t, len(cerr.Message()), 2*apperror.EchoLimit, "%T: message", err)
		assert.Less(t, len(err.Error()), 2*apperror.EchoLimit, "%T: Error()", err)
	}
}
