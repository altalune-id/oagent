package legal

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Source is where a person accepted the terms.
type Source string

// The places a person accepts the terms.
const (
	SourceInstall Source = "install"
	SourceJoin    Source = "join"
	SourceWelcome Source = "welcome"
)

// Snapshot is one stored version of a built-in document.
type Snapshot struct {
	ID          uuid.UUID
	Slug        string
	Version     string
	Title       string
	Body        string
	SHA256      string
	PublishedAt time.Time
}

// Acceptance records that a person accepted one terms and one privacy version.
type Acceptance struct {
	ID                uuid.UUID
	PublicID          string
	UserID            uuid.UUID
	TermsDocumentID   uuid.UUID
	PrivacyDocumentID uuid.UUID
	Source            Source
	AcceptedAt        time.Time
}

// Store keeps document snapshots and the acceptance history; both join the transaction on ctx.
type Store interface {
	// Publish stores s at the database time unless its slug and version exist, and returns the stored snapshot.
	Publish(ctx context.Context, s Snapshot) (Snapshot, error)
	RecordAcceptance(ctx context.Context, a Acceptance) error
}
