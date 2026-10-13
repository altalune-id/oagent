// Package member models who belongs to an org and with which role.
package member

import (
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/nanoid"
)

const publicIDLen = 21

// Role is a member's built-in role inside one org.
type Role string

// The built-in roles.
const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

// IsValid reports whether r is a built-in role.
func (r Role) IsValid() bool { return r == RoleOwner || r == RoleAdmin || r == RoleMember }

// Label is r's display name in English.
func (r Role) Label() string {
	switch r {
	case RoleOwner:
		return "Owner"
	case RoleAdmin:
		return "Admin"
	case RoleMember:
		return "Member"
	}
	return string(r)
}

// Membership links one user to one org with a role.
type Membership struct {
	ID        uuid.UUID
	PublicID  string
	OrgID     uuid.UUID
	UserID    uuid.UUID
	Role      Role
	CreatedAt time.Time
}

// New builds a Membership after validating its ids and role.
func New(orgID, userID uuid.UUID, role Role) (*Membership, error) {
	if orgID == uuid.Nil {
		return nil, apperror.Invalid("organization_id", apperror.FieldRequired)
	}
	if userID == uuid.Nil {
		return nil, apperror.Invalid("user_id", apperror.FieldRequired)
	}
	if !role.IsValid() {
		return nil, &InvalidRoleError{Role: string(role)}
	}
	publicID, err := nanoid.New(publicIDLen)
	if err != nil {
		return nil, err
	}
	return &Membership{ID: uuid.New(), PublicID: publicID, OrgID: orgID, UserID: userID, Role: role, CreatedAt: time.Now().UTC()}, nil
}

type membershipRow struct {
	ID        uuid.UUID `alias:"memberships.id"`
	PublicID  string    `alias:"memberships.public_id"`
	OrgID     uuid.UUID `alias:"memberships.org_id"`
	UserID    uuid.UUID `alias:"memberships.user_id"`
	Role      string    `alias:"memberships.role"`
	CreatedAt time.Time `alias:"memberships.created_at"`
}

func (r *membershipRow) toMembership() *Membership {
	return &Membership{ID: r.ID, PublicID: r.PublicID, OrgID: r.OrgID, UserID: r.UserID, Role: Role(r.Role), CreatedAt: r.CreatedAt}
}

// Member is a membership with the person's email and name, as listed to the org.
type Member struct {
	Membership
	UserPublicID string
	Email        string
	Name         string
}

type memberRow struct {
	Membership   membershipRow
	UserPublicID string `alias:"users.public_id"`
	Email        string `alias:"users.email"`
	Name         string `alias:"users.name"`
}
