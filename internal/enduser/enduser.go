package enduser

import (
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"altalune.id/oagent/nanoid"
)

// MaxValueLen is the longest identity value, in characters.
const MaxValueLen = 256

// Kind names where an identity comes from.
type Kind string

// The identity kinds.
const (
	KindAPIUser    Kind = "api_user"
	KindWAPhone    Kind = "wa_phone"
	KindWALid      Kind = "wa_lid"
	KindWidgetAnon Kind = "widget_anon"
	KindWidgetSub  Kind = "widget_sub"
)

// Valid reports whether k is a known kind.
func (k Kind) Valid() bool {
	switch k {
	case KindAPIUser, KindWAPhone, KindWALid, KindWidgetAnon, KindWidgetSub:
		return true
	}
	return false
}

// Identity is one external handle of an end user, unique per project and kind.
type Identity struct {
	Kind  Kind
	Value string
}

// Validate reports an *InvalidIdentityError when the kind is unknown or the value is not 1 to MaxValueLen UTF-8 characters.
func (i Identity) Validate() error {
	if !i.Kind.Valid() {
		return &InvalidIdentityError{Reason: IdentityKind}
	}
	if i.Value == "" || !utf8.ValidString(i.Value) || utf8.RuneCountInString(i.Value) > MaxValueLen {
		return &InvalidIdentityError{Reason: IdentityValue}
	}
	return nil
}

// EndUser is a person who talks to a project's agents, known by one or more identities.
type EndUser struct {
	ID        uuid.UUID
	PublicID  string
	OrgID     uuid.UUID
	ProjectID uuid.UUID
	CreatedAt time.Time
	UpdatedAt time.Time
}

// New returns a fresh end user of projectID.
func New(orgID, projectID uuid.UUID, now time.Time) *EndUser {
	return &EndUser{
		ID: uuid.Must(uuid.NewV7()), PublicID: nanoid.MustNew(21), OrgID: orgID, ProjectID: projectID,
		CreatedAt: now, UpdatedAt: now,
	}
}
