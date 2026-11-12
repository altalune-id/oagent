package approval

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/schema/entity"
)

// Insert stores a pending approval in the bound org; a second pending approval on the thread is refused by the index.
func (s *postgresStore) Insert(ctx context.Context, a *Approval) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	if a.OrgID != tc.OrgID {
		// SECURITY: without RLS in force, a row naming another org would otherwise land in it.
		return &NotFoundError{}
	}
	calls, err := json.Marshal(a.Calls)
	if err != nil {
		return fmt.Errorf("approval.postgres: Insert: %w", err)
	}
	t := s.approvals
	requester := postgres.Expression(entity.NullUUID())
	if a.Requester.Valid {
		requester = postgres.UUID(a.Requester.UUID)
	}
	stmt := t.INSERT(t.ID, t.PublicID, t.OrgID, t.ProjectID, t.AgentID, t.RunID, t.ThreadID, t.Calls, t.Preview, t.Channel,
		t.RequesterEndUserID, t.RaisedSnapshotHash, t.Status, t.ExpiresAt, t.CreatedAt).VALUES(
		a.ID, a.PublicID, a.OrgID, a.ProjectID, a.AgentID, a.RunID, a.ThreadID, jsonb(calls), a.Preview, string(a.Channel),
		requester, postgres.Bytea(a.RaisedSnapshotHash), string(a.Status), a.ExpiresAt.UTC(), a.CreatedAt.UTC(),
	)
	if err := db.Write(ctx, s.pool, func(q qrm.DB) error { _, err := stmt.ExecContext(ctx, q); return err }); err != nil {
		return fmt.Errorf("approval.postgres: Insert: %w", err)
	}
	return nil
}

// Decide writes d while the approval is pending and unexpired at d.At; ok is false when 0 rows matched.
func (s *postgresStore) Decide(ctx context.Context, id uuid.UUID, d Decided) (*Approval, bool, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, false, err
	}
	t := s.approvals
	calls := postgres.Expression(t.Calls)
	if d.Status == StatusApproved {
		calls = postgres.Raw(`(SELECT jsonb_agg(c || '{"state":"approved"}'::jsonb ORDER BY n) ` +
			`FROM jsonb_array_elements(calls) WITH ORDINALITY AS e(c, n))`)
	}
	stmt := t.UPDATE(t.Status, t.DecidedByKind, t.DecidedByID, t.DecidedVia, t.DecisionText, t.DecidedAt, t.Calls).
		SET(string(d.Status), textOrNull(string(d.By.Kind)), textOrNull(d.By.ID), textOrNull(string(d.Via)), textOrNull(d.Text),
			d.At.UTC(), calls).
		WHERE(t.ID.EQ(postgres.UUID(id)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID))).
			AND(t.Status.EQ(postgres.String(string(StatusPending)))).AND(t.ExpiresAt.GT(postgres.TimestampzT(d.At)))).
		RETURNING(t.AllColumns)
	var rows []approvalRow
	if err := db.Write(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &rows) }); err != nil && !db.IsNoRows(err) {
		return nil, false, fmt.Errorf("approval.postgres: Decide: %w", err)
	}
	if len(rows) == 0 {
		return nil, false, nil
	}
	a, err := rows[0].toApproval()
	return a, err == nil, err
}

// SetCallStates moves the named calls from one state to another in one guarded update; a call in any other state is an error.
func (s *postgresStore) SetCallStates(ctx context.Context, id uuid.UUID, toolCallIDs []string, from, to CallState) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	toArgs, fromArgs := postgres.RawArgs{"#to": string(to)}, postgres.RawArgs{"#from": string(from)}
	names := make([]string, 0, len(toolCallIDs))
	for i, call := range slices.Compact(slices.Sorted(slices.Values(toolCallIDs))) {
		key := fmt.Sprintf("#call%d", i)
		toArgs[key], fromArgs[key] = call, call
		names = append(names, key)
	}
	ids := "ARRAY[" + strings.Join(names, ", ") + "]::text[]"
	t := s.approvals
	moved := postgres.Raw(`(SELECT jsonb_agg(CASE WHEN c->>'tool_call_id' = ANY(`+ids+`) `+
		`THEN c || jsonb_build_object('state', #to::text) ELSE c END ORDER BY n) `+
		`FROM jsonb_array_elements(calls) WITH ORDINALITY AS e(c, n))`, toArgs)
	// SECURITY: the update matches only when every named call is in the from state, so a call never skips a state.
	allFrom := postgres.RawBool(`(SELECT count(*) FROM jsonb_array_elements(calls) AS x(c) `+
		`WHERE c->>'tool_call_id' = ANY(`+ids+`) AND coalesce(c->>'state', '') = #from) = cardinality(`+ids+`)`, fromArgs)
	stmt := t.UPDATE(t.Calls).SET(moved).
		WHERE(t.ID.EQ(postgres.UUID(id)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID))).AND(allFrom))
	var n int64
	if err := db.Write(ctx, s.pool, func(q qrm.DB) error {
		res, err := stmt.ExecContext(ctx, q)
		if err != nil {
			return err
		}
		n, err = res.RowsAffected()
		return err
	}); err != nil {
		return fmt.Errorf("approval.postgres: SetCallStates: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("approval.postgres: calls %v are not all %s", toolCallIDs, from)
	}
	return nil
}

func jsonb(raw []byte) postgres.Expression {
	return postgres.CAST(postgres.String(string(raw))).AS("jsonb")
}

func textOrNull(s string) postgres.Expression {
	if s == "" {
		return entity.NullText()
	}
	return postgres.String(s)
}

// SetPromptSent records the sent prompt's provider id, never an empty one, and its watermark; values already stored stay.
func (s *postgresStore) SetPromptSent(ctx context.Context, id uuid.UUID, externalID string, afterSeq int64) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	t := s.approvals
	sent := postgres.NULL
	if externalID != "" {
		sent = postgres.String(externalID)
	}
	stmt := t.UPDATE(t.PromptExternalID, t.PromptAfterSeq).
		SET(postgres.COALESCE(t.PromptExternalID, sent), postgres.COALESCE(t.PromptAfterSeq, postgres.Int(afterSeq))).
		WHERE(t.ID.EQ(postgres.UUID(id)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID))))
	if err := db.Write(ctx, s.pool, func(q qrm.DB) error {
		_, err := stmt.ExecContext(ctx, q)
		return err
	}); err != nil {
		return fmt.Errorf("approval.postgres: SetPromptSent: %w", err)
	}
	return nil
}

// Expire marks id expired by the system while it is pending and past its expiry on the database clock.
func (s *postgresStore) Expire(ctx context.Context, id uuid.UUID) (*Approval, bool, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, false, err
	}
	t := s.approvals
	stmt := t.UPDATE(t.Status, t.DecidedByKind, t.DecidedAt).
		SET(string(StatusExpired), string(DeciderSystem), postgres.NOW()).
		WHERE(t.ID.EQ(postgres.UUID(id)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID))).
			AND(t.Status.EQ(postgres.String(string(StatusPending)))).AND(t.ExpiresAt.LT_EQ(postgres.NOW()))).
		RETURNING(t.AllColumns)
	var rows []approvalRow
	if err := db.Write(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &rows) }); err != nil && !db.IsNoRows(err) {
		return nil, false, fmt.Errorf("approval.postgres: Expire: %w", err)
	}
	if len(rows) == 0 {
		return nil, false, nil
	}
	a, err := rows[0].toApproval()
	return a, err == nil, err
}

// ExpireUnresumable marks id expired while it is pending or approved; a decider and decision time already set stay.
func (s *postgresStore) ExpireUnresumable(ctx context.Context, id uuid.UUID) (*Approval, bool, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, false, err
	}
	t := s.approvals
	stmt := t.UPDATE(t.Status, t.DecidedByKind, t.DecidedAt).
		SET(string(StatusExpired), postgres.COALESCE(t.DecidedByKind, postgres.String(string(DeciderSystem))),
			postgres.COALESCE(t.DecidedAt, postgres.NOW())).
		WHERE(t.ID.EQ(postgres.UUID(id)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID))).
			AND(t.Status.IN(postgres.String(string(StatusPending)), postgres.String(string(StatusApproved))))).
		RETURNING(t.AllColumns)
	var rows []approvalRow
	if err := db.Write(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &rows) }); err != nil && !db.IsNoRows(err) {
		return nil, false, fmt.Errorf("approval.postgres: ExpireUnresumable: %w", err)
	}
	if len(rows) == 0 {
		return nil, false, nil
	}
	a, err := rows[0].toApproval()
	return a, err == nil, err
}
