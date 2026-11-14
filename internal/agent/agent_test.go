package agent

import (
	"encoding/json"
	"errors"
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
)

func TestNew(t *testing.T) {
	orgID, projectID := uuid.New(), uuid.New()
	a, err := New(orgID, projectID, "acme", "Acme Bot", "Helpful bot", uuid.New())
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, a.ID)
	assert.Len(t, a.PublicID, 21)
	assert.Equal(t, orgID, a.OrgID)
	assert.Equal(t, projectID, a.ProjectID)
	assert.Equal(t, "acme", a.Slug)
	assert.Equal(t, "Acme Bot", a.Name)
	assert.Equal(t, "Helpful bot", a.Description)
	assert.True(t, a.CreatedBy.Valid)
}

func TestNew_Genesis(t *testing.T) {
	a, err := New(uuid.New(), uuid.New(), "acme", "Acme", "", uuid.Nil)
	require.NoError(t, err)
	assert.False(t, a.CreatedBy.Valid)
	assert.Empty(t, a.Description)
}

func TestNew_Invalid(t *testing.T) {
	o, p := uuid.New(), uuid.New()
	_, err := New(o, p, "ab", "Acme", "", uuid.New())
	assert.True(t, IsInvalidSlugError(err))

	_, err = New(o, p, "acme", "", "", uuid.New())
	assert.True(t, IsInvalidNameError(err))

	_, err = New(o, p, "acme", "Acme", strings.Repeat("a", 2001), uuid.New())
	assert.True(t, IsInvalidDescriptionError(err))
}

func TestAgent_Update(t *testing.T) {
	a, err := New(uuid.New(), uuid.New(), "acme", "Acme", "old", uuid.New())
	require.NoError(t, err)

	require.NoError(t, a.Update("  Acme Two  ", "new"))
	assert.Equal(t, "Acme Two", a.Name)
	assert.Equal(t, "new", a.Description)
	assert.True(t, IsInvalidNameError(a.Update("", "x")))
	assert.True(t, IsInvalidDescriptionError(a.Update("Acme", strings.Repeat("a", 2001))))
}

func TestValidateSlug(t *testing.T) {
	assert.NoError(t, ValidateSlug("acme"))
	assert.True(t, IsInvalidSlugError(ValidateSlug("ab")))
	assert.True(t, IsInvalidSlugError(ValidateSlug(strings.Repeat("a", 65))))
	assert.True(t, IsInvalidSlugError(ValidateSlug("Bad_Slug")))
}

func TestValidateName(t *testing.T) {
	assert.NoError(t, ValidateName("Acme"))
	assert.True(t, IsInvalidNameError(ValidateName("")))
	assert.True(t, IsInvalidNameError(ValidateName(strings.Repeat("a", 201))))
}

func validConfig() Config {
	return Config{
		Instructions: "Answer billing questions.", Model: "anthropic/claude-sonnet-5-5", Temperature: 0.7,
		MaxSteps: 10, MaxCostCents: 500, Tools: []Tool{{Name: "search"}, {Name: "refund", RequireApproval: true}},
	}
}

func TestConfig_KnowledgeSources(t *testing.T) {
	src := uuid.New()
	cfg := Config{KnowledgeSourceIDs: []uuid.UUID{src}}
	assert.NoError(t, cfg.Validate())
	assert.False(t, cfg.Equal(Config{}), "a bound source is a change")
	assert.True(t, cfg.Equal(Config{KnowledgeSourceIDs: []uuid.UUID{src}}))

	dup := Config{KnowledgeSourceIDs: []uuid.UUID{src, src}}
	ae, _ := apperror.AsAppError(dup.Validate())
	assert.Equal(t, apperror.CodeValidation, ae.Code())
	many := Config{}
	for range 17 {
		many.KnowledgeSourceIDs = append(many.KnowledgeSourceIDs, uuid.New())
	}
	ae, _ = apperror.AsAppError(many.Validate())
	assert.Equal(t, apperror.CodeValidation, ae.Code())
}

func TestConfig_Validate(t *testing.T) {
	require.NoError(t, Config{}.Validate(), "the zero config runs with the recommended values")
	require.NoError(t, validConfig().Validate())

	manyTools := make([]Tool, maxTools+1)
	for i := range manyTools {
		manyTools[i] = Tool{Name: "t" + strings.Repeat("x", i%10) + string(rune('a'+i%26))}
	}
	for field, mutate := range map[string]func(*Config){
		"config.instructions":    func(c *Config) { c.Instructions = strings.Repeat("a", maxInstructions+1) },
		"config.model":           func(c *Config) { c.Model = "has space" },
		"config.temperature":     func(c *Config) { c.Temperature = 2.1 },
		"config.temperature.nan": func(c *Config) { c.Temperature = math.NaN() },
		"config.max_steps":       func(c *Config) { c.MaxSteps = -1 },
		"config.max_cost_cents":  func(c *Config) { c.MaxCostCents = MaxCostCents + 1 },
		"config.tools":           func(c *Config) { c.Tools = manyTools },
		"config.tools.name":      func(c *Config) { c.Tools = []Tool{{Name: "Bad Name"}} },
	} {
		c := validConfig()
		mutate(&c)
		err := c.Validate()
		ae, ok := apperror.AsAppError(err)
		require.True(t, ok, field)
		require.Len(t, ae.Violations(), 1, field)
		assert.Equal(t, strings.TrimSuffix(field, ".nan"), ae.Violations()[0].Field)
	}

	dup := validConfig()
	dup.Tools = []Tool{{Name: "search"}, {Name: "search", RequireApproval: true}}
	ae, ok := apperror.AsAppError(dup.Validate())
	require.True(t, ok)
	assert.Equal(t, "config.tools", ae.Violations()[0].Field, "tool names are unique")
}

func TestConfig_Equal(t *testing.T) {
	a, b := validConfig(), validConfig()
	assert.True(t, a.Equal(b))
	b.Tools[1].RequireApproval = false
	assert.False(t, a.Equal(b))
	assert.True(t, Config{}.Equal(Config{Tools: []Tool{}}), "no tools and an empty tool list are the same config")
}

func TestAgent_DraftAndPublish(t *testing.T) {
	a, err := New(uuid.New(), uuid.New(), "support", "Support", "", uuid.Nil)
	require.NoError(t, err)
	assert.False(t, a.Unpublished(), "a new agent has nothing to publish")

	t0 := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	require.NoError(t, a.SetDraft(validConfig(), t0))
	assert.True(t, a.Unpublished())
	assert.Error(t, a.SetDraft(Config{Temperature: 3}, t0), "an invalid draft is refused")
	assert.Equal(t, validConfig(), a.Draft, "and changes nothing")

	by := uuid.New()
	v1, created, err := a.Publish(nil, by, t0.Add(time.Minute))
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, int32(1), v1.Version)
	assert.Equal(t, a.ID, v1.AgentID)
	assert.Equal(t, uuid.NullUUID{UUID: by, Valid: true}, v1.CreatedBy)
	assert.Len(t, v1.PublicID, 21)
	assert.Equal(t, int32(1), a.PublishedVersion)
	assert.False(t, a.Unpublished())

	same, created, err := a.Publish(v1, by, t0.Add(2*time.Minute))
	require.NoError(t, err)
	assert.False(t, created, "an unchanged draft creates no version")
	assert.Equal(t, v1, same)

	edited := validConfig()
	edited.Temperature = 0.2
	require.NoError(t, a.SetDraft(edited, t0.Add(3*time.Minute)))
	require.NoError(t, a.SetDraft(validConfig(), t0.Add(4*time.Minute)))
	assert.True(t, a.Unpublished(), "an edit after the publish shows until the next publish")
	again, created, err := a.Publish(v1, uuid.Nil, t0.Add(5*time.Minute))
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, v1, again)
	assert.False(t, a.Unpublished(), "a no-op publish settles a reverted edit")

	require.NoError(t, a.SetDraft(edited, t0.Add(6*time.Minute)))
	v2, created, err := a.Publish(v1, uuid.Nil, t0.Add(7*time.Minute))
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, int32(2), v2.Version)
	assert.False(t, v2.CreatedBy.Valid, "a key publishes as nobody")
	assert.Equal(t, edited, v2.Config)
}

func TestAgent_SetDraftAfterAPublishIsAlwaysUnpublished(t *testing.T) {
	a, err := New(uuid.New(), uuid.New(), "support", "Support", "", uuid.Nil)
	require.NoError(t, err)
	t0 := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	require.NoError(t, a.SetDraft(validConfig(), t0))
	_, _, err = a.Publish(nil, uuid.Nil, t0.Add(time.Second))
	require.NoError(t, err)
	require.NoError(t, a.SetDraft(validConfig(), t0), "a clock behind the publish still marks the edit as later")
	assert.True(t, a.Unpublished())
	assert.Equal(t, a.PublishedAt.Add(time.Microsecond), *a.DraftUpdatedAt)
	require.NoError(t, a.SetDraft(validConfig(), t0.Add(2*time.Second+123)))
	assert.Equal(t, t0.Add(2*time.Second), *a.DraftUpdatedAt, "stamps keep the microseconds Postgres stores")
}

func TestAgent_PublishNumbersFromTheLatestVersion(t *testing.T) {
	a, err := New(uuid.New(), uuid.New(), "support", "Support", "", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, a.SetDraft(validConfig(), time.Now()))
	a.PublishedVersion = 1
	latest := &Version{Version: 4, Config: Config{Model: "older"}}
	v, created, err := a.Publish(latest, uuid.Nil, time.Now())
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, int32(5), v.Version, "a stale agent row never reuses a version number")
}

func TestConfig_LegacyVersionLoads(t *testing.T) {
	var cfg Config
	require.NoError(t, json.Unmarshal([]byte(`{"model":"gpt-4o","tools":[{"name":"x.y"}]}`), &cfg))
	assert.Equal(t, Config{Model: "gpt-4o", Tools: []Tool{{Name: "x.y"}}}, cfg)
	assert.True(t, cfg.Legacy())
	assert.NoError(t, cfg.Validate(), "an old version still validates its shape")
	out := configToProto(cfg)
	assert.Equal(t, "gpt-4o", out.GetModel())
	assert.Equal(t, "x.y", out.GetTools()[0].GetName())

	grant := Config{ProviderID: uuid.New(), Model: "gpt-4o", MCPServers: []MCPGrant{{ServerID: uuid.New(), Tools: []Tool{{Name: "x.y"}}}}}
	assert.False(t, grant.Legacy())
	e, ok := errors.AsType[*InvalidGrantError](grant.Validate())
	require.True(t, ok, "a dotted name is refused as a grant tool")
	assert.Equal(t, GrantToolName, e.Reason)
}

func TestConfig_LegacyRule(t *testing.T) {
	assert.True(t, Config{}.Legacy(), "no provider")
	assert.True(t, Config{ProviderID: uuid.New(), Tools: []Tool{{Name: "a"}}}.Legacy(), "top-level tools")
	assert.False(t, Config{ProviderID: uuid.New()}.Legacy())
}

func TestConfig_EqualNewFields(t *testing.T) {
	base := Config{
		ProviderID: uuid.New(), Model: "m", HelperModel: "h",
		MCPServers: []MCPGrant{{ServerID: uuid.New(), Tools: []Tool{{Name: "a"}, {Name: "b", RequireApproval: true}}}},
	}
	clone := func() Config {
		c := base
		c.MCPServers = []MCPGrant{{ServerID: base.MCPServers[0].ServerID, Tools: slices.Clone(base.MCPServers[0].Tools)}}
		return c
	}
	assert.True(t, base.Equal(clone()))
	for name, edit := range map[string]func(*Config){
		"provider":       func(c *Config) { c.ProviderID = uuid.New() },
		"helper model":   func(c *Config) { c.HelperModel = "other" },
		"server":         func(c *Config) { c.MCPServers[0].ServerID = uuid.New() },
		"grant tool":     func(c *Config) { c.MCPServers[0].Tools[0].Name = "z" },
		"grant approval": func(c *Config) { c.MCPServers[0].Tools[1].RequireApproval = false },
		"memory store":   func(c *Config) { c.MemoryStoreID = uuid.New() },
		"transcription":  func(c *Config) { c.TranscriptionModel = TranscriptionModel{ProviderID: uuid.New(), Model: "whisper-1"} },
		"one grant more": func(c *Config) { c.MCPServers = append(c.MCPServers, MCPGrant{ServerID: uuid.New()}) },
	} {
		c := clone()
		edit(&c)
		assert.False(t, base.Equal(c), name)
	}
	assert.True(t, Config{}.Equal(Config{MCPServers: []MCPGrant{}}), "no grants and an empty grant list are the same")
}

func TestConfig_JSONOmitsZeroIDs(t *testing.T) {
	raw, err := json.Marshal(Config{Model: "m", MCPServers: []MCPGrant{{Tools: []Tool{{Name: "a"}}}}})
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "provider_id")
	assert.NotContains(t, string(raw), "server_id")
	assert.NotContains(t, string(raw), "helper_model")

	id, server := uuid.New(), uuid.New()
	raw, err = json.Marshal(Config{ProviderID: id, MCPServers: []MCPGrant{{ServerID: server}}})
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"provider_id":"`+id.String()+`"`)
	assert.Contains(t, string(raw), `"server_id":"`+server.String()+`"`)
}

func TestConfig_ValidateGrants(t *testing.T) {
	grant := func(tools ...Tool) MCPGrant { return MCPGrant{ServerID: uuid.New(), Tools: tools} }
	ok := Config{ProviderID: uuid.New(), Model: "m", HelperModel: "h-1", MCPServers: []MCPGrant{grant(Tool{Name: "Expense_create-1"})}}
	require.NoError(t, ok.Validate())

	tooMany := ok
	tooMany.MCPServers = nil
	for range 3 {
		tools := make([]Tool, 50)
		for i := range tools {
			tools[i] = Tool{Name: "t" + strconv.Itoa(i)}
		}
		tooMany.MCPServers = append(tooMany.MCPServers, grant(tools...))
	}
	for field, c := range map[string]Config{
		"config.helper_model":              {HelperModel: "has space"},
		"config.transcription_model":       {TranscriptionModel: TranscriptionModel{ProviderID: uuid.New()}},
		"config.transcription_model.model": {TranscriptionModel: TranscriptionModel{ProviderID: uuid.New(), Model: "has space"}},
		"config.mcp_servers":               tooMany,
	} {
		ae, isApp := apperror.AsAppError(c.Validate())
		require.True(t, isApp, field)
		assert.Equal(t, field, ae.Violations()[0].Field)
	}
}

func TestNewUnattendedTool(t *testing.T) {
	s1, s2, src := uuid.New(), uuid.New(), uuid.New()
	free := []FreeCondition{{Arg: "amount", Values: []json.RawMessage{json.RawMessage("1")}}}
	cfg := func(knowledge bool, grants ...MCPGrant) *Config {
		c := &Config{MCPServers: grants}
		if knowledge {
			c.KnowledgeSourceIDs = []uuid.UUID{src}
		}
		return c
	}
	grant := func(server uuid.UUID, tools ...Tool) MCPGrant { return MCPGrant{ServerID: server, Tools: tools} }
	open := func(name string) Tool { return Tool{Name: name} }
	for _, c := range []struct {
		name   string
		draft  *Config
		latest *Config
		want   toolKey
		found  bool
	}{
		{"knowledge and an unapproved tool", cfg(true, grant(s1, open("send"))), nil, toolKey{s1, "send"}, true},
		{"knowledge and an approved tool", cfg(true, grant(s1, Tool{Name: "send", RequireApproval: true})), nil, toolKey{}, false},
		{"knowledge and a conditional tool", cfg(true, grant(s1, Tool{Name: "send", RequireApproval: true, FreeWhen: free})), nil, toolKey{}, false},
		{"free_when without require_approval runs unattended", cfg(true, grant(s1, Tool{Name: "send", FreeWhen: free})), nil, toolKey{s1, "send"}, true},
		{"no knowledge", cfg(false, grant(s1, open("send"))), nil, toolKey{}, false},
		{"grandfathered by a latest that read knowledge", cfg(true, grant(s1, open("send"))), cfg(true, grant(s1, open("send"))), toolKey{}, false},
		{"a new unapproved tool next to a grandfathered one", cfg(true, grant(s1, open("send"), open("delete"))), cfg(true, grant(s1, open("send"))), toolKey{s1, "delete"}, true},
		{"knowledge turned on over an existing unapproved tool", cfg(true, grant(s1, open("send"))), cfg(false, grant(s1, open("send"))), toolKey{s1, "send"}, true},
		{"the same tool name on a new server id", cfg(true, grant(s2, open("send"))), cfg(true, grant(s1, open("send"))), toolKey{s2, "send"}, true},
		{"a memory store over an unapproved tool", &Config{MemoryStoreID: uuid.New(), MCPServers: []MCPGrant{grant(s1, open("send"))}}, nil, toolKey{s1, "send"}, true},
		{"first in grant order", cfg(true, grant(s1, Tool{Name: "a", RequireApproval: true}, open("b")), grant(s2, open("c"))), nil, toolKey{s1, "b"}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, found := newUnattendedTool(*c.draft, c.latest)
			assert.Equal(t, c.found, found)
			assert.Equal(t, c.want, got)
		})
	}
}

func TestConfig_MemoryStoreStoresItsID(t *testing.T) {
	var old Config
	require.NoError(t, json.Unmarshal([]byte(`{"model":"m"}`), &old))
	assert.Equal(t, uuid.Nil, old.MemoryStoreID, "a stored config from before memory has none")
	raw, err := json.Marshal(Config{Model: "m"})
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "memory", "an agent without memory stores the same JSON as before")
	id := uuid.New()
	raw, err = json.Marshal(Config{Model: "m", MemoryStoreID: id})
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"memory_store_id":"`+id.String()+`"`)
}

func TestConfig_TranscriptionModelBothOrNeither(t *testing.T) {
	require.NoError(t, Config{}.Validate())
	require.NoError(t, Config{TranscriptionModel: TranscriptionModel{ProviderID: uuid.New(), Model: "whisper-1"}}.Validate())
	for name, tm := range map[string]TranscriptionModel{
		"provider only": {ProviderID: uuid.New()},
		"model only":    {Model: "whisper-1"},
	} {
		ae, isApp := apperror.AsAppError(Config{TranscriptionModel: tm}.Validate())
		require.True(t, isApp, name)
		assert.Equal(t, "config.transcription_model", ae.Violations()[0].Field, name)
	}
}

func TestConfig_TranscriptionModelJSON(t *testing.T) {
	raw, err := json.Marshal(Config{Model: "m"})
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "transcription_model", "zero means off and is left out")
	id := uuid.New()
	raw, err = json.Marshal(Config{TranscriptionModel: TranscriptionModel{ProviderID: id, Model: "whisper-1"}})
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"transcription_model":{"provider_id":"`+id.String()+`","model":"whisper-1"}`)
}
