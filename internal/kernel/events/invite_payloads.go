package events

import "time"

// InviteCreatedV1 is the payload for InviteCreated; ids are public ids and the token or link is never included.
type InviteCreatedV1 struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	InvitedBy string    `json:"invited_by,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
}

// MemberAddedV1 is the payload for MemberAdded; ids are public ids.
type MemberAddedV1 struct {
	UserID   string `json:"user_id"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	InviteID string `json:"invite_id,omitempty"`
}
