// Package agent models agents within a project.
package agent

import (
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
)

//nolint:gochecknoglobals // immutable compiled patterns.
var (
	slugRe     = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)
	modelRe    = regexp.MustCompile(`^[A-Za-z0-9._:/-]*$`)
	toolNameRe = regexp.MustCompile(`^[a-z0-9_.-]{1,64}$`)
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
	Tools        []Tool  `json:"tools,omitempty"`
}

// Tool is a tool the agent may call; RequireApproval makes a person approve each call.
type Tool struct {
	Name            string `json:"name"`
	RequireApproval bool   `json:"require_approval,omitempty"`
}

// Version is an immutable published Config of an agent; Version counts from 1 per agent.
type Version struct {
	ID        uuid.UUID
	PublicID  string
	OrgID     uuid.UUID
	ProjectID uuid.UUID
	AgentID   uuid.UUID
	Version   int32
	Config    Config
	CreatedBy uuid.NullUUID
	CreatedAt time.Time
}

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
	}
	return nil
}

// Equal reports whether c and o run the same; a nil and an empty tool list are equal.
func (c Config) Equal(o Config) bool {
	return c.Instructions == o.Instructions && c.Model == o.Model && c.Temperature == o.Temperature &&
		c.MaxSteps == o.MaxSteps && c.MaxCostCents == o.MaxCostCents && slices.Equal(c.Tools, o.Tools)
}

// SetDraft replaces the draft with a valid cfg edited at now; the published version is unchanged.
func (a *Agent) SetDraft(cfg Config, now time.Time) error {
	if err := cfg.Validate(); err != nil {
		return err
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
		Version: next, Config: a.Draft, CreatedAt: now,
	}
	if by != uuid.Nil {
		v.CreatedBy = uuid.NullUUID{UUID: by, Valid: true}
	}
	a.PublishedVersion, a.PublishedAt = v.Version, &now
	return v, true, nil
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
	ID        uuid.UUID     `alias:"agent_versions.id"`
	PublicID  string        `alias:"agent_versions.public_id"`
	OrgID     uuid.UUID     `alias:"agent_versions.org_id"`
	ProjectID uuid.UUID     `alias:"agent_versions.project_id"`
	AgentID   uuid.UUID     `alias:"agent_versions.agent_id"`
	Version   int32         `alias:"agent_versions.version"`
	Config    string        `alias:"agent_versions.config"`
	CreatedBy uuid.NullUUID `alias:"agent_versions.created_by"`
	CreatedAt time.Time     `alias:"agent_versions.created_at"`
}

func (r *versionRow) toVersion() (*Version, error) {
	var cfg Config
	if err := json.Unmarshal([]byte(r.Config), &cfg); err != nil {
		return nil, fmt.Errorf("agent: version config: %w", err)
	}
	return &Version{
		ID: r.ID, PublicID: r.PublicID, OrgID: r.OrgID, ProjectID: r.ProjectID, AgentID: r.AgentID,
		Version: r.Version, Config: cfg, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt.UTC(),
	}, nil
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
	}
	for _, t := range c.Tools {
		out.Tools = append(out.Tools, &agentv1.AgentTool{Name: t.Name, RequireApproval: t.RequireApproval})
	}
	return out
}

func configFromProto(c *agentv1.AgentConfig) Config {
	out := Config{
		Instructions: c.GetInstructions(), Model: c.GetModel(), Temperature: c.GetTemperature(),
		MaxSteps: c.GetMaxSteps(), MaxCostCents: c.GetMaxCostCents(),
	}
	for _, t := range c.GetTools() {
		out.Tools = append(out.Tools, Tool{Name: t.GetName(), RequireApproval: t.GetRequireApproval()})
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
