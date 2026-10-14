// Package org models organizations.
package org

import (
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	orgv1 "altalune.id/oagent/gen/go/org/v1"
	"altalune.id/oagent/nanoid"
)

const (
	slugMinLen = 3
	slugMaxLen = 64
	nameMinLen = 1
	nameMaxLen = 200
)

//nolint:gochecknoglobals // immutable compiled pattern.
var slugRe = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

// Org is the tenant boundary aggregate.
type Org struct {
	ID        uuid.UUID
	PublicID  string
	Slug      string
	Name      string
	CreatedBy uuid.NullUUID
	CreatedAt time.Time
	UpdatedAt time.Time
	IsSystem  bool
}

// NewOrg constructs an Org after validating slug and name. A nil createdBy is the genesis org.
func NewOrg(slug, name string, createdBy uuid.UUID) (*Org, error) {
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
	o := &Org{ID: uuid.New(), PublicID: publicID, Slug: s, Name: n, CreatedAt: now, UpdatedAt: now}
	if createdBy != uuid.Nil {
		o.CreatedBy = uuid.NullUUID{UUID: createdBy, Valid: true}
	}
	return o, nil
}

// NewSystemOrg constructs the deployment's single system org; its owners and admins are the operators.
func NewSystemOrg(slug, name string, createdBy uuid.UUID) (*Org, error) {
	o, err := NewOrg(slug, name, createdBy)
	if err != nil {
		return nil, err
	}
	o.IsSystem = true
	return o, nil
}

// Update applies a new display name and slug.
func (o *Org) Update(name, slug string) error {
	n := strings.TrimSpace(name)
	if err := ValidateName(n); err != nil {
		return err
	}
	s := strings.TrimSpace(slug)
	if err := ValidateSlug(s); err != nil {
		return err
	}
	o.Name = n
	o.Slug = s
	return nil
}

// ValidateSlug reports whether s is usable as an org slug.
func ValidateSlug(s string) error {
	if len(s) < slugMinLen || len(s) > slugMaxLen {
		return &InvalidSlugError{Slug: s, Reason: SlugLength}
	}
	if !slugRe.MatchString(s) {
		return &InvalidSlugError{Slug: s, Reason: SlugFormat}
	}
	if s == "new" {
		return &InvalidSlugError{Slug: s, Reason: SlugReserved}
	}
	return nil
}

// ValidateName reports whether n is usable as an org name.
func ValidateName(n string) error {
	if len(n) < nameMinLen || len(n) > nameMaxLen {
		return &InvalidNameError{Reason: NameLength}
	}
	return nil
}

type orgRow struct {
	ID        uuid.UUID     `alias:"orgs.id"`
	PublicID  string        `alias:"orgs.public_id"`
	Slug      string        `alias:"orgs.slug"`
	Name      string        `alias:"orgs.name"`
	CreatedBy uuid.NullUUID `alias:"orgs.created_by"`
	CreatedAt time.Time     `alias:"orgs.created_at"`
	UpdatedAt time.Time     `alias:"orgs.updated_at"`
	IsSystem  bool          `alias:"orgs.is_system"`
}

func (r *orgRow) toOrg() *Org {
	return &Org{
		ID:        r.ID,
		PublicID:  r.PublicID,
		Slug:      r.Slug,
		Name:      r.Name,
		CreatedBy: r.CreatedBy,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
		IsSystem:  r.IsSystem,
	}
}

func orgToProto(o *Org) *orgv1.Org {
	return &orgv1.Org{
		Id:        o.PublicID,
		Slug:      o.Slug,
		Name:      o.Name,
		CreatedAt: timestamppb.New(o.CreatedAt),
		UpdatedAt: timestamppb.New(o.UpdatedAt),
	}
}
