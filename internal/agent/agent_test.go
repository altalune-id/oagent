package agent

import (
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

func TestConfig_Validate(t *testing.T) {
	require.NoError(t, Config{}.Validate(), "the zero config is the runtime default")
	require.NoError(t, validConfig().Validate())

	manyTools := make([]Tool, maxTools+1)
	for i := range manyTools {
		manyTools[i] = Tool{Name: "t" + strings.Repeat("x", i%10) + string(rune('a'+i%26))}
	}
	for field, mutate := range map[string]func(*Config){
		"config.instructions":   func(c *Config) { c.Instructions = strings.Repeat("a", maxInstructions+1) },
		"config.model":          func(c *Config) { c.Model = "has space" },
		"config.temperature":    func(c *Config) { c.Temperature = 2.1 },
		"config.max_steps":      func(c *Config) { c.MaxSteps = -1 },
		"config.max_cost_cents": func(c *Config) { c.MaxCostCents = maxCostCents + 1 },
		"config.tools":          func(c *Config) { c.Tools = manyTools },
		"config.tools.name":     func(c *Config) { c.Tools = []Tool{{Name: "Bad Name"}} },
	} {
		c := validConfig()
		mutate(&c)
		err := c.Validate()
		ae, ok := apperror.AsAppError(err)
		require.True(t, ok, field)
		require.Len(t, ae.Violations(), 1, field)
		assert.Equal(t, field, ae.Violations()[0].Field)
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
