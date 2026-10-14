// Package agent models agents within a project.
package agent

import (
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	agentv1 "altalune.id/oagent/gen/go/agent/v1"
	"altalune.id/oagent/nanoid"
)

const (
	slugMinLen = 3
	slugMaxLen = 64
	nameMinLen = 1
	nameMaxLen = 200
	descMaxLen = 2000
)

//nolint:gochecknoglobals // immutable compiled pattern.
var slugRe = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

// Agent is an aggregate scoped to an organization and a project.
type Agent struct {
	ID          uuid.UUID
	PublicID    string
	OrgID       uuid.UUID
	ProjectID   uuid.UUID
	Slug        string
	Name        string
	Description string
	CreatedBy   uuid.NullUUID
	CreatedAt   time.Time
	UpdatedAt   time.Time
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
}

func (r *agentRow) toAgent() *Agent {
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
	}
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
	}
}
