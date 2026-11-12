package channel

import (
	"context"
	"fmt"
	"time"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
)

func bytesOrNull(b []byte) postgres.Expression {
	if b == nil {
		return postgres.NULL
	}
	return postgres.Bytea(b)
}

func textOrNull(s string) postgres.Expression {
	if s == "" {
		return postgres.NULL
	}
	return postgres.String(s)
}

func timeOrNull(t *time.Time) postgres.Expression {
	if t == nil {
		return postgres.NULL
	}
	return postgres.TimestampzT(t.UTC())
}

func ms(d time.Duration) int32 {
	return int32(min(max(d/time.Millisecond, 0), maxQuiet/time.Millisecond))
}

// Insert stores a new connection in the bound org; an agent outside the project is refused.
func (s *postgresStore) Insert(ctx context.Context, c *Connection) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	if c.OrgID != tc.OrgID {
		// SECURITY: without RLS in force, a row naming another org would otherwise land in it.
		return &NotFoundError{}
	}
	t := s.conns
	stmt := t.INSERT(t.AllColumns).VALUES(
		c.ID, c.PublicID, c.OrgID, c.ProjectID, c.AgentID, c.Provider, c.Status, c.BaseURL, c.OpenWAOrg, c.OpenWAProject, c.DeviceID,
		bytesOrNull(c.SigningPrimarySealed), bytesOrNull(c.SigningSecondarySealed), postgres.Bytea(c.DeviceKeySealed),
		ms(c.QuietWindow), ms(c.QuietMax), c.ReplyFailure, c.ReplyVoice, c.ReplyPhotoUnreadable,
		textOrNull(c.LastResult), timeOrNull(c.LastResultAt), timeOrNull(c.LastBadSignatureAt), c.CreatedAt.UTC(), c.UpdatedAt.UTC(),
		c.ApprovalPrompt, c.ApprovalExpired,
	)
	err = db.Write(ctx, s.pool, func(q qrm.DB) error {
		_, err := stmt.ExecContext(ctx, q)
		return err
	})
	if db.IsForeignKeyViolation(err) {
		return &InvalidError{Field: "agent_id"}
	}
	if err != nil {
		return fmt.Errorf("channel.postgres: Insert: %w", err)
	}
	return nil
}

// Update rewrites a connection's editable settings, secrets and status in the bound org.
func (s *postgresStore) Update(ctx context.Context, c *Connection) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	t := s.conns
	stmt := t.UPDATE(
		t.Status, t.BaseURL, t.OpenwaOrg, t.OpenwaProject, t.DeviceID,
		t.SigningSecretPrimarySealed, t.SigningSecretSecondarySealed, t.DeviceKeySealed,
		t.QuietWindowMS, t.QuietMaxMS, t.ReplyFailure, t.ReplyVoice, t.ReplyPhotoUnreadable, t.ApprovalPrompt, t.ApprovalExpired, t.UpdatedAt,
	).SET(
		c.Status, c.BaseURL, c.OpenWAOrg, c.OpenWAProject, c.DeviceID,
		bytesOrNull(c.SigningPrimarySealed), bytesOrNull(c.SigningSecondarySealed), postgres.Bytea(c.DeviceKeySealed),
		ms(c.QuietWindow), ms(c.QuietMax), c.ReplyFailure, c.ReplyVoice, c.ReplyPhotoUnreadable, c.ApprovalPrompt, c.ApprovalExpired,
		c.UpdatedAt.UTC(),
	).
		WHERE(t.ID.EQ(postgres.UUID(c.ID)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID))))
	return s.affectOne(ctx, "Update", stmt)
}

// Delete removes a connection of the bound org and its deliveries; its threads are detached.
func (s *postgresStore) Delete(ctx context.Context, id uuid.UUID) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	t := s.conns
	return s.affectOne(ctx, "Delete", t.DELETE().WHERE(t.ID.EQ(postgres.UUID(id)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID)))))
}

func (s *postgresStore) affectOne(ctx context.Context, op string, stmt postgres.Statement) error {
	var n int64
	if err := db.Write(ctx, s.pool, func(q qrm.DB) error {
		res, err := stmt.ExecContext(ctx, q)
		if err != nil {
			return err
		}
		n, err = res.RowsAffected()
		return err
	}); err != nil {
		return fmt.Errorf("channel.postgres: %s: %w", op, err)
	}
	if n == 0 {
		return &NotFoundError{}
	}
	return nil
}

// InsertDelivery records a delivery id once per connection; inserted is false for one already taken.
func (s *postgresStore) InsertDelivery(ctx context.Context, c *Connection, deliveryID string, at time.Time) (bool, error) {
	t := s.deliveries
	stmt := t.INSERT(t.OrgID, t.ProjectID, t.ConnectionID, t.DeliveryID, t.ReceivedAt).
		VALUES(c.OrgID, c.ProjectID, c.ID, deliveryID, at.UTC()).
		ON_CONFLICT(t.ConnectionID, t.DeliveryID).DO_NOTHING()
	var n int64
	if err := db.Write(ctx, s.pool, func(q qrm.DB) error {
		res, err := stmt.ExecContext(ctx, q)
		if err != nil {
			return err
		}
		n, err = res.RowsAffected()
		return err
	}); err != nil {
		return false, fmt.Errorf("channel.postgres: InsertDelivery: %w", err)
	}
	return n == 1, nil
}

// PruneDeliveries drops the bound org's delivery ids received before before.
func (s *postgresStore) PruneDeliveries(ctx context.Context, before time.Time) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	t := s.deliveries
	stmt := t.DELETE().WHERE(t.OrgID.EQ(postgres.UUID(tc.OrgID)).AND(t.ReceivedAt.LT(postgres.TimestampzT(before.UTC()))))
	return db.Write(ctx, s.pool, func(q qrm.DB) error {
		if _, err := stmt.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("channel.postgres: PruneDeliveries: %w", err)
		}
		return nil
	})
}

// RecordResult stores a verified outcome as the connection's last result; the same result within a minute changes nothing.
func (s *postgresStore) RecordResult(ctx context.Context, id uuid.UUID, result string, at time.Time) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	t := s.conns
	at = at.UTC()
	stmt := t.UPDATE(t.LastResult, t.LastResultAt).SET(result, at).
		WHERE(t.ID.EQ(postgres.UUID(id)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID))).AND(
			t.LastResult.IS_DISTINCT_FROM(postgres.String(result)).
				OR(t.LastResultAt.IS_NULL()).
				OR(t.LastResultAt.LT(postgres.TimestampzT(at.Add(-resultStampEvery))))))
	return db.Write(ctx, s.pool, func(q qrm.DB) error {
		_, err := stmt.ExecContext(ctx, q)
		return err
	})
}

// RecordBadSignature stamps a failed verify at most once a minute; a stamp within the minute changes nothing.
func (s *postgresStore) RecordBadSignature(ctx context.Context, id uuid.UUID, at time.Time) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	t := s.conns
	at = at.UTC()
	stmt := t.UPDATE(t.LastBadSignatureAt).SET(at).
		WHERE(t.ID.EQ(postgres.UUID(id)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID))).
			AND(t.LastBadSignatureAt.IS_NULL().OR(t.LastBadSignatureAt.LT(postgres.TimestampzT(at.Add(-badSignatureEvery))))))
	return db.Write(ctx, s.pool, func(q qrm.DB) error {
		_, err := stmt.ExecContext(ctx, q)
		return err
	})
}
