package org

import (
	"time"

	"github.com/google/uuid"
)

// orgRow is the jet scan target for the orgs table.
type orgRow struct {
	ID        uuid.UUID     `alias:"orgs.id"`
	PublicID  string        `alias:"orgs.public_id"`
	Slug      string        `alias:"orgs.slug"`
	Name      string        `alias:"orgs.name"`
	CreatedBy uuid.NullUUID `alias:"orgs.created_by"`
	CreatedAt time.Time     `alias:"orgs.created_at"`
	UpdatedAt time.Time     `alias:"orgs.updated_at"`
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
	}
}
