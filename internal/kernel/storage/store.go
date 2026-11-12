package storage

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Store is the driven port for the files table; every call is scoped to the org on ctx.
type Store interface {
	// Insert records a new file.
	Insert(ctx context.Context, f *File) error
	// ByID loads a file, or *UploadNotFoundError when the org has none with that id.
	ByID(ctx context.Context, id uuid.UUID) (*File, error)
	// MarkReady moves a pending file not yet expired at at to ready; a file already ready is left as is.
	MarkReady(ctx context.Context, id uuid.UUID, at time.Time) error
	// MarkDeleting hands a file to the sweep.
	MarkDeleting(ctx context.Context, id uuid.UUID) error
	// Sweepable returns up to limit files that are deleting, or pending past their expiry at now.
	Sweepable(ctx context.Context, now time.Time, limit int) ([]*File, error)
	// LockSweepable locks the file FOR UPDATE while it is deleting, or pending past its expiry at now; otherwise nil. Needs a unit of work.
	LockSweepable(ctx context.Context, id uuid.UUID, now time.Time) (*File, error)
	// DeleteDeleting removes the file row id once it is marked deleting.
	DeleteDeleting(ctx context.Context, id uuid.UUID) error
}

// Abandoned hears about a pending upload the sweep drops, so what waited on it can fail; it runs in the file's org.
type Abandoned interface {
	UploadAbandoned(ctx context.Context, fileID uuid.UUID) error
}
