// Package org models organizations.
package org

import (
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"altalune.id/auth-server/nanoid"
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

// Rename applies a new display name; slug is immutable.
func (o *Org) Rename(name string) error {
	n := strings.TrimSpace(name)
	if err := ValidateName(n); err != nil {
		return err
	}
	o.Name = n
	return nil
}

// ValidateSlug reports whether s is usable as an org slug.
func ValidateSlug(s string) error {
	if len(s) < slugMinLen || len(s) > slugMaxLen {
		return &InvalidSlugError{Slug: s, Reason: "length out of range"}
	}
	if !slugRe.MatchString(s) {
		return &InvalidSlugError{Slug: s, Reason: "must be lowercase alphanumeric with dashes"}
	}
	if s == "new" {
		return &InvalidSlugError{Slug: s, Reason: "reserved word"}
	}
	return nil
}

// ValidateName reports whether n is usable as an org name.
func ValidateName(n string) error {
	if len(n) < nameMinLen || len(n) > nameMaxLen {
		return &InvalidNameError{Reason: "length out of range"}
	}
	return nil
}
