package legal

import (
	"context"
	"fmt"

	"github.com/go-jet/jet/v2/postgres"

	"altalune.id/oagent/internal/kernel/db"
)

// Publish inserts s at the database time unless its slug and version exist, then reads back the stored row.
func (s *postgresStore) Publish(ctx context.Context, snap Snapshot) (Snapshot, error) {
	t := s.documents
	insert := t.INSERT(t.AllColumns).
		VALUES(snap.ID, snap.Slug, snap.Version, snap.Title, snap.Body, snap.SHA256, postgres.NOW()).
		ON_CONFLICT(t.Slug, t.Version).DO_NOTHING()
	if _, err := insert.ExecContext(ctx, db.Writer(ctx, s.pool)); err != nil {
		return Snapshot{}, fmt.Errorf("legal.postgres: Publish: %w", err)
	}
	var row snapshotRow
	stmt := postgres.SELECT(t.AllColumns).FROM(t).
		WHERE(t.Slug.EQ(postgres.String(snap.Slug)).AND(t.Version.EQ(postgres.String(snap.Version))))
	if err := stmt.QueryContext(ctx, db.Writer(ctx, s.pool), &row); err != nil {
		return Snapshot{}, fmt.Errorf("legal.postgres: Publish: read back: %w", err)
	}
	snap = Snapshot(row)
	snap.PublishedAt = snap.PublishedAt.UTC()
	return snap, nil
}

// RecordAcceptance appends a.
func (s *postgresStore) RecordAcceptance(ctx context.Context, a Acceptance) error {
	t := s.acceptances
	stmt := t.INSERT(t.AllColumns).
		VALUES(a.ID, a.PublicID, a.UserID, a.TermsDocumentID, a.PrivacyDocumentID, string(a.Source), a.AcceptedAt.UTC())
	if _, err := stmt.ExecContext(ctx, db.Writer(ctx, s.pool)); err != nil {
		return fmt.Errorf("legal.postgres: RecordAcceptance: %w", err)
	}
	return nil
}
