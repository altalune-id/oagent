package enduser

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	enduserv1 "altalune.id/oagent/gen/go/enduser/v1"
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
	ID          uuid.UUID
	PublicID    string
	OrgID       uuid.UUID
	ProjectID   uuid.UUID
	DisplayName string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Ref is what an end user public id resolves to before any tenant is bound.
type Ref struct{ ID, OrgID, ProjectID uuid.UUID }

// Cursor marks where a newest-first page ended; the zero value starts at the newest.
type Cursor struct {
	At       time.Time
	PublicID string
}

// New returns a fresh end user of projectID.
func New(orgID, projectID uuid.UUID, now time.Time) *EndUser {
	return &EndUser{
		ID: uuid.Must(uuid.NewV7()), PublicID: nanoid.MustNew(21), OrgID: orgID, ProjectID: projectID,
		CreatedAt: now, UpdatedAt: now,
	}
}

// KindAPIKey names an end user that is an API key acting for itself in a Name; it is never stored as an identity.
const KindAPIKey Kind = "api_key"

// Name shows an end user: a display name and the first identity's kind with a masked hint. SECURITY: never a whole value.
type Name struct {
	DisplayName string
	Kind        Kind
	Hint        string
}

// Label is the display name, else the masked hint ("…90"), else empty; it carries no raw kind.
func (n Name) Label() string {
	switch {
	case n.DisplayName != "":
		return n.DisplayName
	case n.Hint != "":
		return "…" + n.Hint
	}
	return ""
}

// NameOf names an end user by its display name and first identity; no identity gives the zero Name.
func NameOf(displayName string, ids []Identity) Name {
	if len(ids) == 0 {
		return Name{DisplayName: displayName}
	}
	return Name{DisplayName: displayName, Kind: ids[0].Kind, Hint: hint(ids[0])}
}

// SECURITY: two digits of a phone or LID, four of anything else, and nothing for a value too short to mask.
func hint(id Identity) string {
	tail := 4
	if id.Kind == KindWAPhone || id.Kind == KindWALid {
		tail = 2
	}
	r := []rune(id.Value)
	if len(r) <= 2*tail {
		return ""
	}
	return string(r[len(r)-tail:])
}

// KindProto is k on the wire.
func KindProto(k Kind) enduserv1.IdentityKind {
	switch k {
	case KindAPIUser:
		return enduserv1.IdentityKind_IDENTITY_KIND_API_USER
	case KindAPIKey:
		return enduserv1.IdentityKind_IDENTITY_KIND_API_KEY
	case KindWAPhone:
		return enduserv1.IdentityKind_IDENTITY_KIND_WA_PHONE
	case KindWALid:
		return enduserv1.IdentityKind_IDENTITY_KIND_WA_LID
	case KindWidgetAnon:
		return enduserv1.IdentityKind_IDENTITY_KIND_WIDGET_ANON
	case KindWidgetSub:
		return enduserv1.IdentityKind_IDENTITY_KIND_WIDGET_SUB
	}
	return enduserv1.IdentityKind_IDENTITY_KIND_UNSPECIFIED
}

// MemoryClass is how far memory may follow an end user.
type MemoryClass int

// The memory classes.
const (
	// MemoryNever is an API key acting for itself or an anonymous widget visitor.
	MemoryNever MemoryClass = iota
	// MemoryPerson is a WhatsApp sender, a signed widget user or a member acting through a personal key.
	MemoryPerson
	// MemoryNamedAPIUser is an Oagent-User an integration names; any key of the project can name it, so a store must opt in.
	MemoryNamedAPIUser
)

// ClassOf names how far memory may follow the end user ids belong to.
func ClassOf(ids []Identity) MemoryClass {
	if len(ids) == 0 {
		return MemoryNever
	}
	// SECURITY: the lowest trust among the identities wins, so a person's identity never lifts a named or anonymous one.
	out := MemoryPerson
	for _, id := range ids {
		switch id.Kind {
		case KindWAPhone, KindWALid, KindWidgetSub:
		case KindAPIUser:
			switch {
			case strings.HasPrefix(id.Value, keyActorPrefix):
				return MemoryNever
			case !strings.HasPrefix(id.Value, personActorPrefix):
				out = MemoryNamedAPIUser
			}
		default:
			return MemoryNever
		}
	}
	return out
}
