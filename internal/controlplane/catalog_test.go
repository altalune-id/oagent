package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	apperrorv1 "altalune.id/oagent/gen/go/apperror/v1"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/apikey"
	"altalune.id/oagent/internal/approval"
	"altalune.id/oagent/internal/auth"
	"altalune.id/oagent/internal/channel"
	"altalune.id/oagent/internal/enduser"
	"altalune.id/oagent/internal/install"
	"altalune.id/oagent/internal/kernel/authn"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/knowledge"
	"altalune.id/oagent/internal/mcpserver"
	"altalune.id/oagent/internal/memory"
	"altalune.id/oagent/internal/modelprovider"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/org/invite"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/run"
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
		&authn.UnavailableError{},
		apperror.Invalid("organization_id", apperror.FieldInvalid),
		apperror.Invalid("owner", apperror.FieldRequired),
		apperror.Validation([]apperror.Violation{{Field: "slug", Reason: apperror.FieldTooShort, Param: "3"}}),
		apperror.Validation([]apperror.Violation{{Field: "name", Reason: apperror.FieldTooLong, Param: "200"}}),
		apperror.Validation([]apperror.Violation{{Field: "id", Reason: apperror.FieldInvalidFormat}}),
		apperror.Validation([]apperror.Violation{{Field: "role", Reason: apperror.FieldNotAllowed}}),
		apperror.Validation([]apperror.Violation{{Field: "events", Reason: apperror.FieldTooMany, Param: "20"}}),
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
		&org.AlreadyOwnedError{},

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
		&agent.NeedsRepublishError{},
		&agent.InvalidGrantError{Reason: agent.GrantProvider},
		&agent.InvalidGrantError{Reason: agent.GrantModel, Name: "m"},
		&agent.InvalidGrantError{Reason: agent.GrantHelperModel, Name: "m"},
		&agent.InvalidGrantError{Reason: agent.GrantServer},
		&agent.InvalidGrantError{Reason: agent.GrantToolName, Name: "a.b"},
		&agent.InvalidGrantError{Reason: agent.GrantLegacyTools},
		&agent.InvalidGrantError{Reason: agent.GrantBudgetUnpriced, Name: "m"},

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
		&member.LastOwnerError{Reason: member.LastOwnerRemove},
		&member.LastOwnerError{Reason: member.LastOwnerDemote},
		&member.LastOwnerError{Reason: member.LastOwnerLeave},
		&member.RoleNotAllowedError{Role: string(member.RoleOwner)},
		&member.SelfChangeError{},

		&invite.NotUsableError{Reason: invite.ReasonExpired},
		&invite.NotUsableError{Reason: invite.ReasonRevoked},
		&invite.NotUsableError{Reason: invite.ReasonUsed},
		&invite.NotUsableError{Reason: invite.ReasonNotFound},
		&invite.EmailMismatchError{},
		&invite.DraftExpiredError{},
		&invite.NotFoundError{},
		&invite.PendingError{},

		&auth.InvalidCredentialsError{},
		&auth.RateLimitedError{RetryAfter: 90 * time.Second},
		&auth.ResetNotUsableError{Reason: auth.ResetExpired},
		&auth.ResetNotUsableError{Reason: auth.ResetUsed},
		&auth.ResetNotUsableError{Reason: auth.ResetNotFound},
		&auth.NoConsoleAccountError{Reason: auth.NoConsoleNotInvited},
		&auth.NoConsoleAccountError{Reason: auth.NoConsoleRemoved},
		&auth.NoConsoleAccountError{Reason: auth.NoConsoleUnverified},
		&auth.SignInExpiredError{},
		&auth.IdentityConflictError{},
		password.Check("short", password.Hints{}),
		password.Check(strings.Repeat("a", password.MaxLen+1), password.Hints{}),
		password.Check("iloveyou1234", password.Hints{}),
		password.Check("abababababab", password.Hints{}),
		password.Check("abcdefghijklm", password.Hints{}),
		password.Check("alice-is-great-2026", password.Hints{Email: "alice@example.com"}),
		password.Check("maria-loves-tea", password.Hints{Name: "Ana Maria"}),

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

		&modelprovider.NotFoundError{ID: "x"},
		&modelprovider.AlreadyExistsError{Name: "openrouter"},
		errOf(modelprovider.New(nobody, nobody, provider(""), time.Time{})),
		errOf(modelprovider.New(nobody, nobody, provider("https://e.com/"+strings.Repeat("a", modelprovider.MaxBaseURLLen)), time.Time{})),
		errOf(modelprovider.New(nobody, nobody, provider("https://e.com/\xff"), time.Time{})),
		errOf(modelprovider.New(nobody, nobody, provider("https://e.com/%zz"), time.Time{})),
		errOf(modelprovider.New(nobody, nobody, provider("http://e.com"), time.Time{})),
		errOf(modelprovider.New(nobody, nobody, provider("https://u:p@e.com"), time.Time{})),
		errOf(modelprovider.New(nobody, nobody, provider("https://"), time.Time{})),
		errOf(modelprovider.New(nobody, nobody, providerModels(nil), time.Time{})),
		errOf(modelprovider.New(nobody, nobody, providerModels(make([]modelprovider.Model, modelprovider.MaxModels+1)), time.Time{})),
		errOf(modelprovider.New(nobody, nobody, providerModels([]modelprovider.Model{{ID: "a"}, {ID: "a"}}), time.Time{})),
		errOf(modelprovider.New(nobody, nobody, providerModels([]modelprovider.Model{{ID: "a", Prices: &modelprovider.Prices{InputMicros: -1}}}), time.Time{})),
		errOf(modelprovider.New(nobody, nobody, providerModels([]modelprovider.Model{{ID: "a", Transcription: true}}), time.Time{})),
		errOf(modelprovider.New(nobody, nobody, providerModels([]modelprovider.Model{{ID: "a", Transcription: true, Embedding: true, Prices: &modelprovider.Prices{}}}), time.Time{})),
		&modelprovider.InUseError{Reason: modelprovider.InUseDelete},
		&modelprovider.InUseError{Reason: modelprovider.InUseModelRemoved, Model: "a"},
		&modelprovider.InUseError{Reason: modelprovider.InUsePriceRemoved, Model: "a"},
		&modelprovider.SecretRequiredError{},
		&modelprovider.UnknownModelError{Model: "gpt-9"},
		&modelprovider.SecretsUnavailableError{},

		&mcpserver.NotFoundError{ID: "x"},
		&mcpserver.AlreadyExistsError{Name: "yasaku"},
		errOf(mcpserver.New(nobody, nobody, mcpInput(""), time.Time{})),
		errOf(mcpserver.New(nobody, nobody, mcpInput("https://e.com/"+strings.Repeat("a", mcpserver.MaxURLLen)), time.Time{})),
		errOf(mcpserver.New(nobody, nobody, mcpInput("https://e.com/\xff"), time.Time{})),
		errOf(mcpserver.New(nobody, nobody, mcpInput("https://e.com/%zz"), time.Time{})),
		errOf(mcpserver.New(nobody, nobody, mcpInput("http://e.com"), time.Time{})),
		errOf(mcpserver.New(nobody, nobody, mcpInput("https://u:p@e.com"), time.Time{})),
		errOf(mcpserver.New(nobody, nobody, mcpInput("https://"), time.Time{})),
		&mcpserver.InUseError{},
		&mcpserver.UnreachableError{},
		&mcpserver.SecretRequiredError{Reason: mcpserver.SecretValueRequired},
		errOf(mcpserver.New(nobody, nobody, mcpserver.Input{Name: "s", URL: "https://e.com", AuthKind: mcpserver.AuthHeader, AuthHeader: "Host"}, time.Time{})),
		&mcpserver.SecretsUnavailableError{},
		&mcpserver.AuthRefusedError{Status: 401},
		&enduser.NotFoundError{ID: "eu"},
		&enduser.InvalidIdentityError{Reason: enduser.IdentityValue},
		&enduser.InvalidIdentityError{Reason: enduser.IdentityKind},
		&run.NotFoundError{ID: "r"},
		&run.RateLimitedError{},
		&run.RuntimeUnavailableError{},
		&run.TimedOutError{},
		&run.AttemptsExhaustedError{Attempts: 3},
		&run.RuntimeFailedError{Reason: "model_error"},
		&run.InProgressError{RunPublicID: "r"},
		&run.IdempotencyConflictError{},
		&channel.NotFoundError{},
		&channel.InvalidError{Field: "base_url"},
		&channel.SecretRequiredError{},
		&channel.SecretsUnavailableError{Cause: errors.New("sealer down")},
		&channel.ProviderUnavailableError{Cause: errors.New("dial tcp: refused")},
		&channel.WebhookURLUnavailableError{},
		&run.ThreadNotFoundError{ID: "t"},
		&run.ThreadActorMismatchError{ThreadID: "t"},
		&approval.PendingError{Approval: "Vb3Kq8Rt2Wy5Zp9Hm7Nc4"},
		&approval.NotFoundError{},
		&approval.AlreadyDecidedError{Status: "rejected"},
		&approval.ResumeIncompleteError{Missing: []string{"call_1"}},
		&approval.EditedArgsRefusedError{},
		&approval.AnsweredByAppError{},
		&agent.InvalidGrantError{Reason: agent.GrantKnowledgeSource},
		&agent.InvalidGrantError{Reason: agent.GrantUnapprovedTool, Name: "yasaku__send"},
		&agent.InvalidGrantError{Reason: agent.GrantMemoryStore},
		&agent.InvalidGrantError{Reason: agent.GrantMemoryStoreDisabled},
		&agent.InvalidGrantError{Reason: agent.GrantTranscriptionModel, Name: "whisper-1"},
		&memory.NotFoundError{ID: "f1"},
		&memory.StoreNotFoundError{},
		&memory.InvalidStoreError{Reason: memory.StoreNameRequired},
		&memory.InvalidStoreError{Reason: memory.StoreNameTooLong},
		&memory.InvalidStoreError{Reason: memory.StoreBackendDisabled},
		&memory.StoreInUseError{},
		&memory.StoreAlreadyExistsError{Name: "People"},
		&memory.StoreBackendOffError{Backend: "mem0"},
		&modelprovider.InUseError{Reason: modelprovider.InUseEmbeddingModel, Model: "a"},
		&modelprovider.InUseError{Reason: modelprovider.InUseTranscriptionModel, Model: "a"},
		&knowledge.SourceNotFoundError{},
		&knowledge.InvalidSourceError{Reason: knowledge.SourceURLRequired},
		&knowledge.InvalidSourceError{Reason: knowledge.SourceURLTooLong},
		&knowledge.InvalidSourceError{Reason: knowledge.SourceURLNotUTF8},
		&knowledge.InvalidSourceError{Reason: knowledge.SourceURLUnparsable},
		&knowledge.InvalidSourceError{Reason: knowledge.SourceURLHTTPSRequired},
		&knowledge.InvalidSourceError{Reason: knowledge.SourceURLHasCredentials},
		&knowledge.InvalidSourceError{Reason: knowledge.SourceURLNoHost},
		&knowledge.InvalidSourceError{Reason: knowledge.SourceBackendDisabled},
		&knowledge.InvalidSourceError{Reason: knowledge.SourceKindMismatch},
		&knowledge.DocumentNotFoundError{},
		&knowledge.UploadRejectedError{Reason: knowledge.RejectMissing},
		&knowledge.UploadRejectedError{Reason: knowledge.RejectSize},
		&knowledge.UploadRejectedError{Reason: knowledge.RejectType},
		&knowledge.UploadRejectedError{Reason: knowledge.RejectContent},
		&knowledge.UploadRejectedError{Reason: knowledge.RejectExpired},
		&knowledge.UploadRejectedError{Reason: knowledge.RejectTooLarge},
		&knowledge.UploadRejectedError{Reason: knowledge.RejectTooManyEntries},
		&knowledge.UploadRejectedError{Reason: knowledge.RejectTooManyPages},
		&knowledge.UploadRejectedError{Reason: knowledge.RejectMissingDocumentXML},
		&knowledge.UploadRejectedError{Reason: knowledge.RejectInvalidUTF8},
		&knowledge.UploadRejectedError{Reason: knowledge.RejectUnsupportedType},
		&knowledge.NoTextFoundError{},
		&knowledge.UnreadableError{Reason: knowledge.UnreadableCorrupt},
		&knowledge.UnreadableError{Reason: knowledge.UnreadableTimeout},
		&knowledge.UnreadableError{Reason: knowledge.UnreadableOutputOverflow},
		&knowledge.UnreadableError{Reason: knowledge.UnreadableCrashed},
		&knowledge.EmbeddingModelInvalidError{Reason: knowledge.EmbeddingSize, Dims: 384},
		&knowledge.EmbeddingModelInvalidError{Reason: knowledge.EmbeddingProviderKind},
		&knowledge.EmbeddingModelInvalidError{Reason: knowledge.EmbeddingNotEmbedding},
		&knowledge.EmbeddingModelInvalidError{Reason: knowledge.EmbeddingUnpriced},
		&knowledge.SourceInUseError{},
		&knowledge.StorageNotConfiguredError{},
		&knowledge.LimitReachedError{Reason: knowledge.LimitDocumentsPerSource, Max: 100},
		&knowledge.LimitReachedError{Reason: knowledge.LimitChunksPerProject, Max: 20000},
		&knowledge.LimitReachedError{Reason: knowledge.LimitSourcesPerProject, Max: 50},
		&knowledge.LimitReachedError{Reason: knowledge.LimitRefreshTooSoon, Max: 10},
		&knowledge.LimitReachedError{Reason: knowledge.LimitTextPerDocument, Max: 2 << 20},
		&knowledge.LimitReachedError{Reason: knowledge.LimitChunksPerDocument, Max: 2000},
		&knowledge.EmbeddingModelNotSetError{},
		&knowledge.SourceAlreadyExistsError{Name: "Handbook"},
		&knowledge.EmbeddingFailedError{Reason: knowledge.EmbeddingUnreachable},
		&knowledge.EmbeddingFailedError{Reason: knowledge.EmbeddingRejected},
		&knowledge.EmbeddingFailedError{Reason: knowledge.EmbeddingWrongDims},
		&knowledge.CrawlFailedError{Reason: knowledge.CrawlOffHostRedirect},
		&knowledge.CrawlFailedError{Reason: knowledge.CrawlRobotsDisallowed},
		&knowledge.CrawlFailedError{Reason: knowledge.CrawlUnreachable},
		&knowledge.CrawlFailedError{Reason: knowledge.CrawlNoPages},
	}
}

func mcpInput(url string) mcpserver.Input {
	return mcpserver.Input{Name: "s", URL: url, AuthKind: mcpserver.AuthNone, TimeoutMS: 5000}
}

func provider(baseURL string) modelprovider.Input {
	return modelprovider.Input{Name: "p", Kind: modelprovider.KindOpenAICompatible, BaseURL: baseURL, Models: []modelprovider.Model{{ID: "m"}}}
}

func providerModels(models []modelprovider.Model) modelprovider.Input {
	in := provider("https://e.com")
	in.Models = models
	return in
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
		&agent.InvalidGrantError{Reason: agent.GrantToolName, Name: long},
		&modelprovider.AlreadyExistsError{Name: long},
		&mcpserver.AlreadyExistsError{Name: long},
		&modelprovider.UnknownModelError{Model: long},
		&modelprovider.InvalidModelsError{Reason: modelprovider.ModelsDuplicate, Model: long},
		&modelprovider.InUseError{Reason: modelprovider.InUseModelRemoved, Model: long},
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

func TestEveryInvalidFieldHasALabel(t *testing.T) {
	fields := map[string]string{}
	err := filepath.WalkDir("../..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && slices.Contains([]string{".git", "node_modules", "gen", "ui", "docs"}, d.Name()) {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Invalid" || fmt.Sprint(sel.X) != "apperror" {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !assert.True(t, ok && lit.Kind == token.STRING, "%s: apperror.Invalid needs a literal field name", path) {
				return true
			}
			name, _ := strconv.Unquote(lit.Value)
			fields[name] = path
			return true
		})
		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, fields)

	locales, err := filepath.Glob("../../ui/base/i18n/locales/*.json")
	require.NoError(t, err)
	require.NotEmpty(t, locales)
	for _, path := range locales {
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		var messages map[string]string
		require.NoError(t, json.Unmarshal(raw, &messages))
		for field, src := range fields {
			assert.Contains(t, messages, "field."+field, "%s: no label for field %q (used in %s)", filepath.Base(path), field, src)
		}
	}
}
