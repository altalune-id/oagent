package storage

import (
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/schema/entity"
)

type postgresStore struct {
	pool  db.Pool
	files *entity.Files
}

var _ Store = (*postgresStore)(nil)

// NewStore builds the Postgres store over pool.
func NewStore(pool db.Pool, schema, tablePrefix string) *postgresStore {
	return &postgresStore{pool: pool, files: entity.NewFiles(schema, tablePrefix)}
}

type fileRow struct {
	ID          uuid.UUID  `alias:"files.id"`
	PublicID    string     `alias:"files.public_id"`
	OrgID       uuid.UUID  `alias:"files.org_id"`
	ProjectID   uuid.UUID  `alias:"files.project_id"`
	Purpose     string     `alias:"files.purpose"`
	Visibility  string     `alias:"files.visibility"`
	ObjectKey   string     `alias:"files.object_key"`
	ContentType string     `alias:"files.content_type"`
	SizeBytes   int64      `alias:"files.size_bytes"`
	Status      string     `alias:"files.status"`
	CreatedAt   time.Time  `alias:"files.created_at"`
	CompletedAt *time.Time `alias:"files.completed_at"`
	ExpiresAt   time.Time  `alias:"files.expires_at"`
}

func (r *fileRow) toFile() *File {
	return &File{
		ID: r.ID, PublicID: r.PublicID, OrgID: r.OrgID, ProjectID: r.ProjectID,
		Purpose: r.Purpose, Visibility: Visibility(r.Visibility), ObjectKey: r.ObjectKey, ContentType: r.ContentType,
		SizeBytes: r.SizeBytes, Status: Status(r.Status),
		CreatedAt: r.CreatedAt.UTC(), CompletedAt: utcPtr(r.CompletedAt), ExpiresAt: r.ExpiresAt.UTC(),
	}
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}
