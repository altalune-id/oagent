// Package user models the people who sign in to oagent.
package user

import (
	"database/sql"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/nanoid"
)

const (
	publicIDLen = 21
	emailMaxLen = 254
	nameMaxLen  = 200
)

// User is the identity aggregate for one person.
type User struct {
	ID              uuid.UUID
	PublicID        string
	Email           string
	Name            string
	AvatarURL       string
	PasswordHash    string
	IDPIssuer       string
	IDPSubject      string
	Locale          string
	TermsAcceptedAt *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// New builds a User with a normalized email and trimmed name.
func New(email, name string) (*User, error) {
	e, err := NormalizeEmail(email)
	if err != nil {
		return nil, err
	}
	n, err := cleanName(name)
	if err != nil {
		return nil, err
	}
	publicID, err := nanoid.New(publicIDLen)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	return &User{ID: uuid.New(), PublicID: publicID, Email: e, Name: n, CreatedAt: now, UpdatedAt: now}, nil
}

func (u *User) rename(name string) error {
	n, err := cleanName(name)
	if err != nil {
		return err
	}
	u.Name = n
	return nil
}

func cleanName(name string) (string, error) {
	n := strings.TrimSpace(name)
	if len(n) > nameMaxLen {
		return "", &InvalidNameError{}
	}
	return n, nil
}

// HasPassword reports whether u can sign in with a local password.
func (u *User) HasPassword() bool { return u.PasswordHash != "" }

// NormalizeEmail trims and lowercases email and rejects anything that is not a bare address.
func NormalizeEmail(email string) (string, error) {
	e := strings.ToLower(strings.TrimSpace(email))
	if e == "" || len(e) > emailMaxLen {
		return "", &InvalidEmailError{}
	}
	addr, err := mail.ParseAddress(e)
	if err != nil || addr.Address != e || addr.Name != "" {
		return "", &InvalidEmailError{}
	}
	return e, nil
}

type userRow struct {
	ID              uuid.UUID      `alias:"users.id"`
	PublicID        string         `alias:"users.public_id"`
	Email           string         `alias:"users.email"`
	Name            string         `alias:"users.name"`
	AvatarURL       string         `alias:"users.avatar_url"`
	PasswordHash    string         `alias:"users.password_hash"`
	IDPIssuer       sql.NullString `alias:"users.idp_issuer"`
	IDPSubject      sql.NullString `alias:"users.idp_subject"`
	Locale          string         `alias:"users.locale"`
	TermsAcceptedAt *time.Time     `alias:"users.terms_accepted_at"`
	CreatedAt       time.Time      `alias:"users.created_at"`
	UpdatedAt       time.Time      `alias:"users.updated_at"`
}

func (r *userRow) toUser() *User {
	return &User{
		ID: r.ID, PublicID: r.PublicID, Email: r.Email, Name: r.Name, AvatarURL: r.AvatarURL,
		PasswordHash: r.PasswordHash, IDPIssuer: r.IDPIssuer.String, IDPSubject: r.IDPSubject.String,
		Locale: r.Locale, TermsAcceptedAt: r.TermsAcceptedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}
