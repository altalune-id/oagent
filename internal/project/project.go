// Package project models projects within an organization.
package project

import (
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	projectv1 "altalune.id/oagent/gen/go/project/v1"
	"altalune.id/oagent/nanoid"
)

const (
	slugMinLen = 3
	slugMaxLen = 64
	nameMinLen = 1
	nameMaxLen = 200
)

// DefaultSlug and DefaultName seed the project every newly created org gets.
const (
	DefaultSlug = "default"
	DefaultName = "Default"
)

//nolint:gochecknoglobals // immutable compiled pattern.
var slugRe = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

// Project is an aggregate scoped to an organization.
type Project struct {
	ID        uuid.UUID
	PublicID  string
	OrgID     uuid.UUID
	Slug      string
	Name      string
	CreatedBy uuid.NullUUID
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewProject constructs a Project after validating slug and name.
func NewProject(orgID uuid.UUID, slug, name string, createdBy uuid.UUID) (*Project, error) {
	s := strings.TrimSpace(slug)
	if err := ValidateSlug(s); err != nil {
		return nil, err
	}
	n := strings.TrimSpace(name)
	if err := ValidateName(n); err != nil {
		return nil, err
	}
	publicID, err := nanoid.New(21)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	p := &Project{ID: uuid.New(), PublicID: publicID, OrgID: orgID, Slug: s, Name: n, CreatedAt: now, UpdatedAt: now}
	if createdBy != uuid.Nil {
		p.CreatedBy = uuid.NullUUID{UUID: createdBy, Valid: true}
	}
	return p, nil
}

// Update applies a new display name and slug.
func (p *Project) Update(name, slug string) error {
	n := strings.TrimSpace(name)
	if err := ValidateName(n); err != nil {
		return err
	}
	s := strings.TrimSpace(slug)
	if err := ValidateSlug(s); err != nil {
		return err
	}
	p.Name = n
	p.Slug = s
	return nil
}

// ValidateSlug reports whether s is usable as a project slug.
func ValidateSlug(s string) error {
	if len(s) < slugMinLen || len(s) > slugMaxLen {
		return &InvalidSlugError{Slug: s, Reason: SlugLength}
	}
	if !slugRe.MatchString(s) {
		return &InvalidSlugError{Slug: s, Reason: SlugFormat}
	}
	return nil
}

// ValidateName reports whether n is usable as a project name.
func ValidateName(n string) error {
	if len(n) < nameMinLen || len(n) > nameMaxLen {
		return &InvalidNameError{Reason: NameLength}
	}
	return nil
}

type projectRow struct {
	ID        uuid.UUID     `alias:"projects.id"`
	PublicID  string        `alias:"projects.public_id"`
	OrgID     uuid.UUID     `alias:"projects.org_id"`
	Slug      string        `alias:"projects.slug"`
	Name      string        `alias:"projects.name"`
	CreatedBy uuid.NullUUID `alias:"projects.created_by"`
	CreatedAt time.Time     `alias:"projects.created_at"`
	UpdatedAt time.Time     `alias:"projects.updated_at"`
}

func (r *projectRow) toProject() *Project {
	return &Project{
		ID:        r.ID,
		PublicID:  r.PublicID,
		OrgID:     r.OrgID,
		Slug:      r.Slug,
		Name:      r.Name,
		CreatedBy: r.CreatedBy,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
}

func projectToProto(p *Project, orgPublicID string) *projectv1.Project {
	return &projectv1.Project{
		Id:        p.PublicID,
		OrgId:     orgPublicID,
		Slug:      p.Slug,
		Name:      p.Name,
		CreatedAt: timestamppb.New(p.CreatedAt),
		UpdatedAt: timestamppb.New(p.UpdatedAt),
	}
}
