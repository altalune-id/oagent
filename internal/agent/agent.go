// Package agent models agents within a project.
package agent

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"altalune.id/oagent/apperror"
	agentv1 "altalune.id/oagent/gen/go/agent/v1"
	"altalune.id/oagent/nanoid"
)

const (
	slugMinLen = 3
	slugMaxLen = 64
	nameMinLen = 1
	nameMaxLen = 200
	descMaxLen = 2000

	maxInstructions = 32000
	maxModelLen     = 128
	maxTemperature  = 2
	maxSteps        = 100
	maxCostCents    = 1000000
	maxTools        = 64
	maxGrants       = 16
	maxGrantTools   = 128
	maxFreeWhen     = 8
	maxFreeValues   = 16
	maxFreeValueLen = 256
)

//nolint:gochecknoglobals // immutable compiled patterns.
var (
	slugRe      = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)
	modelRe     = regexp.MustCompile(`^[A-Za-z0-9._:/-]*$`)
	toolNameRe  = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,64}$`)
	grantNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)
	argNameRe   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)
)

// Agent is an aggregate scoped to an organization and a project.
type Agent struct {
	ID          uuid.UUID
	PublicID    string
	OrgID       uuid.UUID
	ProjectID   uuid.UUID
	Slug        string
	Name        string
	Description string
	Draft       Config
	// DraftUpdatedAt is nil until the draft is first edited.
	DraftUpdatedAt *time.Time
	// PublishedVersion is the latest version's number, 0 before the first publish.
	PublishedVersion int32
	PublishedAt      *time.Time
	CreatedBy        uuid.NullUUID
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Config is what an agent runs with; zero MaxSteps is the runtime default and zero MaxCostCents no limit.
type Config struct {
	Instructions string  `json:"instructions,omitempty"`
	Model        string  `json:"model,omitempty"`
	Temperature  float64 `json:"temperature,omitempty"`
	MaxSteps     int32   `json:"max_steps,omitempty"`
	MaxCostCents int32   `json:"max_cost_cents,omitempty"`
	// Tools is the legacy tool list from before grants; a new draft never sets it.
	Tools       []Tool     `json:"tools,omitempty"`
	ProviderID  uuid.UUID  `json:"provider_id,omitzero"`
	HelperModel string     `json:"helper_model,omitempty"`
	MCPServers  []MCPGrant `json:"mcp_servers,omitempty"`
	// KnowledgeSourceIDs are the knowledge sources searched before each turn.
	KnowledgeSourceIDs []uuid.UUID `json:"knowledge_source_ids,omitempty"`
}

// MCPGrant lets the agent call the listed tools of one MCP server.
type MCPGrant struct {
	ServerID uuid.UUID `json:"server_id,omitzero"`
	Tools    []Tool    `json:"tools,omitempty"`
}

// Legacy reports whether c predates provider grants: no provider, or a top-level tool list; it must be republished before it runs.
func (c Config) Legacy() bool {
	return c.ProviderID == uuid.Nil || len(c.Tools) > 0
}

// Models returns the model ids c uses, the main model first.
func (c Config) Models() []string {
	out := []string{}
	if c.Model != "" {
		out = append(out, c.Model)
	}
	if c.HelperModel != "" && c.HelperModel != c.Model {
		out = append(out, c.HelperModel)
	}
	return out
}

// ServerIDs returns the MCP servers c grants tools of.
func (c Config) ServerIDs() []uuid.UUID {
	out := make([]uuid.UUID, 0, len(c.MCPServers))
	for _, g := range c.MCPServers {
		out = append(out, g.ServerID)
	}
	return out
}

// Tool is a tool the agent may call; RequireApproval makes a person approve each call.
type Tool struct {
	Name            string `json:"name"`
	RequireApproval bool   `json:"require_approval,omitempty"`
	// FreeWhen lets a call of a tool that requires approval run without asking when every condition holds.
	FreeWhen []FreeCondition `json:"free_when,omitempty"`
}

// FreeCondition holds when argument Arg equals one of Values (canonical JSON), or is absent and Absent is set.
type FreeCondition struct {
	Arg    string            `json:"arg"`
	Values []json.RawMessage `json:"values,omitempty"`
	Absent bool              `json:"absent,omitempty"`
}

// Equal reports whether t and o are the same tool with the same approval rule.
func (t Tool) Equal(o Tool) bool {
	return t.Name == o.Name && t.RequireApproval == o.RequireApproval && slices.EqualFunc(t.FreeWhen, o.FreeWhen, FreeCondition.Equal)
}

// Equal reports whether c and o are the same condition; values compare byte for byte, as they are stored canonical.
func (c FreeCondition) Equal(o FreeCondition) bool {
	return c.Arg == o.Arg && c.Absent == o.Absent &&
		slices.EqualFunc(c.Values, o.Values, func(a, b json.RawMessage) bool { return bytes.Equal(a, b) })
}

// Version is an immutable published Config of an agent; Version counts from 1 per agent.
type Version struct {
	ID        uuid.UUID
	PublicID  string
	OrgID     uuid.UUID
	ProjectID uuid.UUID
	AgentID   uuid.UUID
	// Version is 0 for a draft.
	Version   int32
	Config    Config
	CreatedBy uuid.NullUUID
	CreatedAt time.Time
	Kind      VersionKind
	// ConfigHash identifies a draft's config; a published version has none.
	ConfigHash []byte
}

// VersionKind tells a published version from a draft snapshot a Try it run is pinned to.
type VersionKind string

// The version kinds.
const (
	VersionPublished VersionKind = "published"
	VersionDraft     VersionKind = "draft"
)

// Validate reports the first rule c breaks as a validation error on its config.* field.
func (c Config) Validate() error {
	invalid := func(field string, reason apperror.FieldReason) error {
		return apperror.Validation([]apperror.Violation{{Field: "config." + field, Reason: reason}})
	}
	switch {
	case utf8.RuneCountInString(c.Instructions) > maxInstructions:
		return invalid("instructions", apperror.FieldTooLong)
	case len(c.Model) > maxModelLen:
		return invalid("model", apperror.FieldTooLong)
	case !modelRe.MatchString(c.Model):
		return invalid("model", apperror.FieldInvalidFormat)
	case len(c.HelperModel) > maxModelLen:
		return invalid("helper_model", apperror.FieldTooLong)
	case !modelRe.MatchString(c.HelperModel):
		return invalid("helper_model", apperror.FieldInvalidFormat)
	case !(c.Temperature >= 0 && c.Temperature <= maxTemperature):
		return invalid("temperature", apperror.FieldInvalid)
	case c.MaxSteps < 0 || c.MaxSteps > maxSteps:
		return invalid("max_steps", apperror.FieldInvalid)
	case c.MaxCostCents < 0 || c.MaxCostCents > maxCostCents:
		return invalid("max_cost_cents", apperror.FieldInvalid)
	case len(c.Tools) > maxTools:
		return invalid("tools", apperror.FieldTooMany)
	}
	seen := make(map[string]bool, len(c.Tools))
	for _, t := range c.Tools {
		if !toolNameRe.MatchString(t.Name) {
			return invalid("tools.name", apperror.FieldInvalidFormat)
		}
		if seen[t.Name] {
			return invalid("tools", apperror.FieldInvalid)
		}
		seen[t.Name] = true
		if reason, bad := validFreeWhen(t.FreeWhen); bad {
			return invalid("tools.free_when", reason)
		}
	}
	return c.validateGrants(invalid)
}

// NOTE: the same limits as the proto rules, for callers that do not pass through them (the CLI, a stored draft).
func validFreeWhen(conds []FreeCondition) (apperror.FieldReason, bool) {
	if len(conds) > maxFreeWhen {
		return apperror.FieldTooMany, true
	}
	for _, c := range conds {
		switch {
		case !argNameRe.MatchString(c.Arg):
			return apperror.FieldInvalidFormat, true
		case len(c.Values) > maxFreeValues:
			return apperror.FieldTooMany, true
		}
		for _, v := range c.Values {
			switch {
			case len(v) > maxFreeValueLen:
				return apperror.FieldTooLong, true
			case !json.Valid(v):
				return apperror.FieldInvalidFormat, true
			}
		}
	}
	return "", false
}

func (c Config) validateGrants(invalid func(string, apperror.FieldReason) error) error {
	if len(c.KnowledgeSourceIDs) > maxGrants {
		return invalid("knowledge_source_ids", apperror.FieldTooMany)
	}
	sources := make(map[uuid.UUID]bool, len(c.KnowledgeSourceIDs))
	for _, id := range c.KnowledgeSourceIDs {
		if sources[id] {
			return invalid("knowledge_source_ids", apperror.FieldInvalid)
		}
		sources[id] = true
	}
	if len(c.MCPServers) > maxGrants {
		return invalid("mcp_servers", apperror.FieldTooMany)
	}
	total := 0
	servers := make(map[uuid.UUID]bool, len(c.MCPServers))
	for _, g := range c.MCPServers {
		if servers[g.ServerID] {
			return invalid("mcp_servers", apperror.FieldInvalid)
		}
		servers[g.ServerID] = true
		if len(g.Tools) > maxTools {
			return invalid("mcp_servers.tools", apperror.FieldTooMany)
		}
		names := make(map[string]bool, len(g.Tools))
		for _, t := range g.Tools {
			if !grantNameRe.MatchString(t.Name) {
				return &InvalidGrantError{Reason: GrantToolName, Name: t.Name}
			}
			if names[t.Name] {
				return invalid("mcp_servers.tools", apperror.FieldInvalid)
			}
			names[t.Name] = true
			if reason, bad := validFreeWhen(t.FreeWhen); bad {
				return invalid("mcp_servers.tools.free_when", reason)
			}
		}
		total += len(g.Tools)
	}
	if total > maxGrantTools {
		return invalid("mcp_servers", apperror.FieldTooMany)
	}
	return nil
}

// Equal reports whether c and o run the same; a nil and an empty tool list are equal.
func (c Config) Equal(o Config) bool {
	return c.Instructions == o.Instructions && c.Model == o.Model && c.Temperature == o.Temperature &&
		c.MaxSteps == o.MaxSteps && c.MaxCostCents == o.MaxCostCents && slices.EqualFunc(c.Tools, o.Tools, Tool.Equal) &&
		c.ProviderID == o.ProviderID && c.HelperModel == o.HelperModel &&
		slices.EqualFunc(c.MCPServers, o.MCPServers, func(a, b MCPGrant) bool {
			return a.ServerID == b.ServerID && slices.EqualFunc(a.Tools, b.Tools, Tool.Equal)
		}) && slices.Equal(c.KnowledgeSourceIDs, o.KnowledgeSourceIDs)
}

// SetDraft replaces the draft with a valid cfg edited at now; the published version is unchanged.
func (a *Agent) SetDraft(cfg Config, now time.Time) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	now = now.Truncate(time.Microsecond)
	if a.PublishedAt != nil && !now.After(*a.PublishedAt) {
		now = a.PublishedAt.Add(time.Microsecond)
	}
	a.Draft, a.DraftUpdatedAt = cfg, &now
	return nil
}

// Unpublished reports whether the draft was edited after the last publish.
func (a *Agent) Unpublished() bool {
	return a.DraftUpdatedAt != nil && (a.PublishedAt == nil || a.DraftUpdatedAt.After(*a.PublishedAt))
}

// Publish makes the draft the version after latest at now; a draft equal to latest returns latest and created false.
func (a *Agent) Publish(latest *Version, by uuid.UUID, now time.Time) (v *Version, created bool, err error) {
	if latest != nil && latest.Config.Equal(a.Draft) {
		a.PublishedAt = &now
		return latest, false, nil
	}
	publicID, err := nanoid.New(21)
	if err != nil {
		return nil, false, err
	}
	next := int32(1)
	if latest != nil {
		next = latest.Version + 1
	}
	v = &Version{
		ID: uuid.New(), PublicID: publicID, OrgID: a.OrgID, ProjectID: a.ProjectID, AgentID: a.ID,
		Version: next, Config: a.Draft, CreatedAt: now, Kind: VersionPublished,
	}
	if by != uuid.Nil {
		v.CreatedBy = uuid.NullUUID{UUID: by, Valid: true}
	}
	a.PublishedVersion, a.PublishedAt = v.Version, &now
	return v, true, nil
}

// DraftVersion snapshots the draft config as an unnumbered draft version by person by at now; equal configs share a hash.
func (a *Agent) DraftVersion(by uuid.UUID, now time.Time) (*Version, error) {
	raw, err := json.Marshal(a.Draft)
	if err != nil {
		return nil, fmt.Errorf("agent: draft hash: %w", err)
	}
	sum := sha256.Sum256(raw)
	publicID, err := nanoid.New(21)
	if err != nil {
		return nil, err
	}
	v := &Version{
		ID: uuid.New(), PublicID: publicID, OrgID: a.OrgID, ProjectID: a.ProjectID, AgentID: a.ID,
		Config: a.Draft, CreatedAt: now, Kind: VersionDraft, ConfigHash: sum[:],
	}
	if by != uuid.Nil {
		v.CreatedBy = uuid.NullUUID{UUID: by, Valid: true}
	}
	return v, nil
}

// New constructs an Agent after validating slug, name and description.
func New(orgID, projectID uuid.UUID, slug, name, description string, createdBy uuid.UUID) (*Agent, error) {
	s := strings.TrimSpace(slug)
	if err := ValidateSlug(s); err != nil {
		return nil, err
	}
	n := strings.TrimSpace(name)
	if err := ValidateName(n); err != nil {
		return nil, err
	}
	d, err := normalizeDescription(description)
	if err != nil {
		return nil, err
	}
	publicID, err := nanoid.New(21)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	a := &Agent{
		ID:          uuid.New(),
		PublicID:    publicID,
		OrgID:       orgID,
		ProjectID:   projectID,
		Slug:        s,
		Name:        n,
		Description: d,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if createdBy != uuid.Nil {
		a.CreatedBy = uuid.NullUUID{UUID: createdBy, Valid: true}
	}
	return a, nil
}

// Update applies a new name and description; slug is immutable.
func (a *Agent) Update(name, description string) error {
	n := strings.TrimSpace(name)
	if err := ValidateName(n); err != nil {
		return err
	}
	d, err := normalizeDescription(description)
	if err != nil {
		return err
	}
	a.Name = n
	a.Description = d
	return nil
}

// ValidateSlug reports whether s is usable as an agent slug.
func ValidateSlug(s string) error {
	if len(s) < slugMinLen || len(s) > slugMaxLen {
		return &InvalidSlugError{Slug: s, Reason: SlugLength}
	}
	if !slugRe.MatchString(s) {
		return &InvalidSlugError{Slug: s, Reason: SlugFormat}
	}
	return nil
}

// ValidateName reports whether n is usable as an agent name.
func ValidateName(n string) error {
	if len(n) < nameMinLen || len(n) > nameMaxLen {
		return &InvalidNameError{Reason: NameLength}
	}
	return nil
}

func normalizeDescription(d string) (string, error) {
	d = strings.TrimSpace(d)
	if len(d) > descMaxLen {
		return "", &InvalidDescriptionError{Reason: DescriptionLength}
	}
	return d, nil
}

type agentRow struct {
	ID          uuid.UUID     `alias:"agents.id"`
	PublicID    string        `alias:"agents.public_id"`
	OrgID       uuid.UUID     `alias:"agents.org_id"`
	ProjectID   uuid.UUID     `alias:"agents.project_id"`
	Slug        string        `alias:"agents.slug"`
	Name        string        `alias:"agents.name"`
	Description string        `alias:"agents.description"`
	CreatedBy   uuid.NullUUID `alias:"agents.created_by"`
	CreatedAt   time.Time     `alias:"agents.created_at"`
	UpdatedAt   time.Time     `alias:"agents.updated_at"`

	DraftConfig      string     `alias:"agents.draft_config"`
	DraftUpdatedAt   *time.Time `alias:"agents.draft_updated_at"`
	PublishedVersion int32      `alias:"agents.published_version"`
	PublishedAt      *time.Time `alias:"agents.published_at"`
}

func (r *agentRow) toAgent() (*Agent, error) {
	var draft Config
	if err := json.Unmarshal([]byte(r.DraftConfig), &draft); err != nil {
		return nil, fmt.Errorf("agent: draft config: %w", err)
	}
	return &Agent{
		ID:          r.ID,
		PublicID:    r.PublicID,
		OrgID:       r.OrgID,
		ProjectID:   r.ProjectID,
		Slug:        r.Slug,
		Name:        r.Name,
		Description: r.Description,
		CreatedBy:   r.CreatedBy,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,

		Draft:            draft,
		DraftUpdatedAt:   utcPtr(r.DraftUpdatedAt),
		PublishedVersion: r.PublishedVersion,
		PublishedAt:      utcPtr(r.PublishedAt),
	}, nil
}

type versionRow struct {
	ID         uuid.UUID     `alias:"agent_versions.id"`
	PublicID   string        `alias:"agent_versions.public_id"`
	OrgID      uuid.UUID     `alias:"agent_versions.org_id"`
	ProjectID  uuid.UUID     `alias:"agent_versions.project_id"`
	AgentID    uuid.UUID     `alias:"agent_versions.agent_id"`
	Version    *int32        `alias:"agent_versions.version"`
	Config     string        `alias:"agent_versions.config"`
	CreatedBy  uuid.NullUUID `alias:"agent_versions.created_by"`
	CreatedAt  time.Time     `alias:"agent_versions.created_at"`
	Kind       string        `alias:"agent_versions.kind"`
	ConfigHash []byte        `alias:"agent_versions.config_hash"`
}

func (r *versionRow) toVersion() (*Version, error) {
	var cfg Config
	if err := json.Unmarshal([]byte(r.Config), &cfg); err != nil {
		return nil, fmt.Errorf("agent: version config: %w", err)
	}
	return &Version{
		ID: r.ID, PublicID: r.PublicID, OrgID: r.OrgID, ProjectID: r.ProjectID, AgentID: r.AgentID,
		Version: deref(r.Version), Config: cfg, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt.UTC(),
		Kind: VersionKind(r.Kind), ConfigHash: r.ConfigHash,
	}, nil
}

func deref(n *int32) int32 {
	if n == nil {
		return 0
	}
	return *n
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func agentToProto(a *Agent, orgPublicID, projectPublicID string) *agentv1.Agent {
	return &agentv1.Agent{
		Id:          a.PublicID,
		OrgId:       orgPublicID,
		ProjectId:   projectPublicID,
		Slug:        a.Slug,
		Name:        a.Name,
		Description: a.Description,
		CreatedAt:   timestamppb.New(a.CreatedAt),
		UpdatedAt:   timestamppb.New(a.UpdatedAt),

		Draft:              configToProto(a.Draft),
		PublishedVersion:   a.PublishedVersion,
		DraftUpdatedAt:     timestampOrNil(a.DraftUpdatedAt),
		UnpublishedChanges: a.Unpublished(),
	}
}

func versionToProto(v *Version, agentPublicID, createdBy string) *agentv1.AgentVersion {
	return &agentv1.AgentVersion{
		Id: v.PublicID, AgentId: agentPublicID, Version: v.Version, Config: configToProto(v.Config),
		CreatedAt: timestamppb.New(v.CreatedAt), CreatedBy: createdBy,
	}
}

func configToProto(c Config) *agentv1.AgentConfig {
	out := &agentv1.AgentConfig{
		Instructions: c.Instructions, Model: c.Model, Temperature: c.Temperature, MaxSteps: c.MaxSteps, MaxCostCents: c.MaxCostCents,
		HelperModel: c.HelperModel, Tools: toolsToProto(c.Tools),
	}
	for _, g := range c.MCPServers {
		out.McpServers = append(out.McpServers, &agentv1.McpGrant{Tools: toolsToProto(g.Tools)})
	}
	return out
}

func toolsToProto(tools []Tool) []*agentv1.AgentTool {
	out := make([]*agentv1.AgentTool, 0, len(tools))
	for _, t := range tools {
		out = append(out, &agentv1.AgentTool{Name: t.Name, RequireApproval: t.RequireApproval, FreeWhen: freeWhenToProto(t.FreeWhen)})
	}
	return out
}

func toolsFromProto(tools []*agentv1.AgentTool) []Tool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]Tool, 0, len(tools))
	for _, t := range tools {
		out = append(out, Tool{Name: t.GetName(), RequireApproval: t.GetRequireApproval(), FreeWhen: freeWhenFromProto(t.GetFreeWhen())})
	}
	return out
}

func configFromProto(c *agentv1.AgentConfig) Config {
	out := Config{
		Instructions: c.GetInstructions(), Model: c.GetModel(), Temperature: c.GetTemperature(),
		MaxSteps: c.GetMaxSteps(), MaxCostCents: c.GetMaxCostCents(), HelperModel: c.GetHelperModel(),
		Tools: toolsFromProto(c.GetTools()),
	}
	for _, g := range c.GetMcpServers() {
		out.MCPServers = append(out.MCPServers, MCPGrant{Tools: toolsFromProto(g.GetTools())})
	}
	return out
}

func timestampOrNil(t *time.Time) *timestamppb.Timestamp {
	if t == nil {
		return nil
	}
	return timestamppb.New(*t)
}

// Ref names an agent and the tenant it belongs to, before any row is read.
type Ref struct {
	ID        uuid.UUID
	OrgID     uuid.UUID
	ProjectID uuid.UUID
}
