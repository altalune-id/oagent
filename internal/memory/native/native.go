// Package native is the Postgres memory.Facts driver: facts in one table, recalled with Postgres full-text search.
package native

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/memory"
	"altalune.id/oagent/nanoid"
	"altalune.id/oagent/schema/entity"
)

// Facts keeps facts in the memory_facts table; it never extracts facts itself.
type Facts struct {
	pool  db.Pool
	facts *entity.MemoryFacts
	table string
}

var _ memory.Facts = (*Facts)(nil)

// New returns the Facts driver over pool's memory_facts table in schema with tablePrefix.
func New(pool db.Pool, schema, tablePrefix string) *Facts {
	if schema == "" {
		schema = "public"
	}
	return &Facts{pool: pool, facts: entity.NewMemoryFacts(schema, tablePrefix), table: schema + "." + tablePrefix + "memory_facts"}
}

// SECURITY: a scope must name the org on ctx, so a scope of another org is refused before RLS is ever asked.
func check(ctx context.Context, scope memory.Scope, needAgent bool) error {
	if err := scope.Check(needAgent); err != nil {
		return err
	}
	return sameOrg(ctx, scope)
}

func sameOrg(ctx context.Context, scope memory.Scope) error {
	if tc, err := tenant.From(ctx); err != nil || tc.OrgID != scope.OrgID {
		return &memory.TenantMismatchError{}
	}
	return nil
}

// Extracts implements memory.Facts.
func (s *Facts) Extracts() bool { return false }

type factRow struct {
	PublicID    string     `alias:"memory_facts.public_id"`
	Text        string     `alias:"memory_facts.text"`
	CreatedAt   time.Time  `alias:"memory_facts.created_at"`
	AgentID     uuid.UUID  `alias:"memory_facts.agent_id"`
	SourceRunID *uuid.UUID `alias:"memory_facts.source_run_id"`
}

func (r factRow) fact() memory.Fact {
	f := memory.Fact{ID: r.PublicID, Text: r.Text, CreatedAt: r.CreatedAt.UTC(), AgentID: r.AgentID}
	if r.SourceRunID != nil {
		f.SourceRunID = *r.SourceRunID
	}
	return f
}

func toFacts(rows []factRow) []memory.Fact {
	out := make([]memory.Fact, len(rows))
	for i, r := range rows {
		out[i] = r.fact()
	}
	return out
}

// SECURITY: every statement filters org, project, store and end user, and the agent when the scope names one; RLS checks the org again.
func (s *Facts) in(scope memory.Scope) postgres.BoolExpression {
	t := s.facts
	where := t.OrgID.EQ(postgres.UUID(scope.OrgID)).
		AND(t.ProjectID.EQ(postgres.UUID(scope.ProjectID))).
		AND(t.StoreID.EQ(postgres.UUID(scope.StoreID))).
		AND(t.EndUserID.EQ(postgres.UUID(scope.EndUserID)))
	if scope.AgentID != uuid.Nil {
		where = where.AND(t.AgentID.EQ(postgres.UUID(scope.AgentID)))
	}
	return where
}

func (s *Facts) newestFirst() []postgres.OrderByClause {
	return []postgres.OrderByClause{s.facts.CreatedAt.DESC(), s.facts.PublicID.DESC()}
}

func (s *Facts) columns() postgres.ProjectionList {
	t := s.facts
	return postgres.ProjectionList{t.PublicID, t.Text, t.CreatedAt, t.AgentID, t.SourceRunID}
}

// Recall implements memory.Facts; the query's words are ORed, so one shared word is enough.
func (s *Facts) Recall(ctx context.Context, scope memory.Scope, query string, k int) ([]memory.Fact, error) {
	if err := check(ctx, scope, true); err != nil {
		return nil, err
	}
	var out []memory.Fact
	err := db.ReadPrimary(ctx, s.pool, func(q qrm.DB) error {
		matched, err := s.matching(ctx, q, scope, query, k)
		if err != nil || len(matched) >= k {
			out = matched
			return err
		}
		where := s.in(scope)
		if len(matched) > 0 {
			ids := make([]postgres.Expression, len(matched))
			for i, f := range matched {
				ids[i] = postgres.String(f.ID)
			}
			where = where.AND(s.facts.PublicID.NOT_IN(ids...))
		}
		var rows []factRow
		stmt := postgres.SELECT(s.columns()).FROM(s.facts).WHERE(where).ORDER_BY(s.newestFirst()...).LIMIT(int64(k - len(matched)))
		if err := stmt.QueryContext(ctx, q, &rows); err != nil && !db.IsNoRows(err) {
			return err
		}
		out = matched
		out = append(out, toFacts(rows)...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("memory.native: recall: %w", err)
	}
	return out, nil
}

// NOTE: raw SQL because go-jet has no tsquery builder and the generated search column is not in the binding.
func (s *Facts) matching(ctx context.Context, q qrm.DB, scope memory.Scope, query string, k int) ([]memory.Fact, error) {
	sqlText := `WITH q AS (SELECT replace(plainto_tsquery('simple', $1)::text, ' & ', ' | ')::tsquery AS q)
		SELECT public_id, text, created_at, agent_id, source_run_id FROM ` + s.table + `, q
		WHERE org_id = $2 AND project_id = $3 AND store_id = $4 AND end_user_id = $5 AND agent_id = $6 AND search @@ q.q
		ORDER BY ts_rank_cd(search, q.q) DESC, created_at DESC, public_id DESC LIMIT $7`
	rows, err := q.QueryContext(ctx, sqlText, query, scope.OrgID, scope.ProjectID, scope.StoreID, scope.EndUserID, scope.AgentID, k)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []memory.Fact
	for rows.Next() {
		var r factRow
		if err := rows.Scan(&r.PublicID, &r.Text, &r.CreatedAt, &r.AgentID, &r.SourceRunID); err != nil {
			return nil, err
		}
		out = append(out, r.fact())
	}
	return out, rows.Err()
}

// Record implements memory.Facts; it keeps at most runtime.MaxExtractedFacts facts, each cut to runtime.MaxMemoryFactRunes.
func (s *Facts) Record(ctx context.Context, scope memory.Scope, turn memory.Turn) error {
	if err := check(ctx, scope, true); err != nil {
		return err
	}
	t := s.facts
	run := postgres.NULL
	if turn.RunID != uuid.Nil {
		run = postgres.UUID(turn.RunID)
	}
	now := time.Now().UTC()
	err := db.Write(ctx, s.pool, func(q qrm.DB) error {
		for i, text := range memory.CleanFacts(turn.Facts) {
			publicID, err := nanoid.New(memory.PublicIDLen)
			if err != nil {
				return err
			}
			id, err := uuid.NewV7()
			if err != nil {
				return err
			}
			at := now.Add(time.Duration(i) * time.Microsecond)
			stmt := t.INSERT(t.ID, t.PublicID, t.OrgID, t.ProjectID, t.StoreID, t.AgentID, t.EndUserID, t.Text, t.TextHash, t.SourceRunID,
				t.CreatedAt).
				VALUES(id, publicID, scope.OrgID, scope.ProjectID, scope.StoreID, scope.AgentID, scope.EndUserID, text,
					memory.Hash(text), run, at).
				ON_CONFLICT(t.StoreID, t.EndUserID, t.AgentID, t.TextHash).
				DO_UPDATE(postgres.SET(t.CreatedAt.SET(postgres.TimestampzT(at)), t.SourceRunID.SET(postgres.StringExp(run))))
			if _, err := stmt.ExecContext(ctx, q); err != nil {
				return err
			}
		}
		oldest := postgres.SELECT(t.ID).FROM(t).WHERE(s.in(scope)).ORDER_BY(s.newestFirst()...).OFFSET(memory.MaxPerPerson)
		_, err := t.DELETE().WHERE(t.ID.IN(oldest)).ExecContext(ctx, q)
		return err
	})
	if err != nil {
		return fmt.Errorf("memory.native: record: %w", err)
	}
	return nil
}

type cursor struct {
	at time.Time
	id string
}

func (c cursor) String() string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(c.at.UnixNano(), 10) + "." + c.id))
}

func parseCursor(raw string) (cursor, bool) {
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return cursor{}, false
	}
	nanos, id, ok := strings.Cut(string(b), ".")
	n, err := strconv.ParseInt(nanos, 10, 64)
	if !ok || err != nil || id == "" {
		return cursor{}, false
	}
	return cursor{at: time.Unix(0, n).UTC(), id: id}, true
}

// List implements memory.Facts; it reads the primary so a fact just deleted never shows; an unreadable cursor starts at the newest.
func (s *Facts) List(
	ctx context.Context, scope memory.Scope, after string, limit int,
) (facts []memory.Fact, next string, total int, err error) {
	if err := check(ctx, scope, false); err != nil {
		return nil, "", 0, err
	}
	t := s.facts
	where := s.in(scope)
	if c, ok := parseCursor(after); ok {
		where = where.AND(postgres.ROW(t.CreatedAt, t.PublicID).LT(postgres.ROW(postgres.TimestampzT(c.at), postgres.String(c.id))))
	}
	var rows []factRow
	var count struct{ Count int64 }
	err = db.ReadPrimary(ctx, s.pool, func(q qrm.DB) error {
		stmt := postgres.SELECT(s.columns()).FROM(t).WHERE(where).ORDER_BY(s.newestFirst()...).LIMIT(int64(limit + 1))
		if err := stmt.QueryContext(ctx, q, &rows); err != nil && !db.IsNoRows(err) {
			return err
		}
		return postgres.SELECT(postgres.COUNT(postgres.STAR).AS("count")).FROM(t).WHERE(s.in(scope)).QueryContext(ctx, q, &count)
	})
	if err != nil {
		return nil, "", 0, fmt.Errorf("memory.native: list: %w", err)
	}
	facts = toFacts(rows)
	if len(facts) > limit {
		facts = facts[:limit]
		last := facts[len(facts)-1]
		next = cursor{at: last.CreatedAt, id: last.ID}.String()
	}
	return facts, next, int(count.Count), nil
}

// Delete implements memory.Facts.
func (s *Facts) Delete(ctx context.Context, scope memory.Scope, id string) error {
	if err := check(ctx, scope, false); err != nil {
		return err
	}
	n, err := s.remove(ctx, "delete", s.in(scope).AND(s.facts.PublicID.EQ(postgres.String(id))))
	if err != nil {
		return err
	}
	if n == 0 {
		return &memory.NotFoundError{ID: id}
	}
	return nil
}

// Clear implements memory.Facts.
func (s *Facts) Clear(ctx context.Context, scope memory.Scope) (int, error) {
	if err := check(ctx, scope, false); err != nil {
		return 0, err
	}
	return s.remove(ctx, "clear", s.in(scope))
}

// ClearStore implements memory.Facts; the store's FK cascade also takes its facts when the store row goes.
func (s *Facts) ClearStore(ctx context.Context, scope memory.Scope) (int, error) {
	if err := scope.CheckStore(); err != nil {
		return 0, err
	}
	if err := sameOrg(ctx, scope); err != nil {
		return 0, err
	}
	t := s.facts
	return s.remove(ctx, "clear store", t.OrgID.EQ(postgres.UUID(scope.OrgID)).
		AND(t.ProjectID.EQ(postgres.UUID(scope.ProjectID))).
		AND(t.StoreID.EQ(postgres.UUID(scope.StoreID))))
}

func (s *Facts) remove(ctx context.Context, op string, where postgres.BoolExpression) (int, error) {
	var res sql.Result
	err := db.Write(ctx, s.pool, func(q qrm.DB) error {
		var err error
		res, err = s.facts.DELETE().WHERE(where).ExecContext(ctx, q)
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("memory.native: %s: %w", op, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("memory.native: %s: %w", op, err)
	}
	return int(n), nil
}
