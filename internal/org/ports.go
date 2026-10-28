package org

import (
	"context"

	"github.com/google/uuid"
)

// MemberRoles returns a person's role in each org they belong to, keyed by org id; boot adapts member.Service to it.
type MemberRoles interface {
	Roles(ctx context.Context, userID uuid.UUID) (map[uuid.UUID]string, error)
}
