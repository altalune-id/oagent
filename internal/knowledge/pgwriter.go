package knowledge

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/nanoid"
	"altalune.id/oagent/schema/entity"
)

var errLockOutsideTx = errors.New("knowledge.postgres: a row lock needs a transaction")

// InsertSource records a new source; a name taken in the project is a *SourceAlreadyExistsError, and a full project a *LimitReachedError.
func (s *postgresStore) InsertSource(ctx context.Context, src *Source) error {
	if err := sameOrg(ctx, src.OrgID); err != nil {
		return err
	}
	err := db.NewTransactor(s.pool).RunInTx(ctx, func(ctx context.Context) error {
		if err := db.LockInTx(ctx, "knowledge-sources:"+src.ProjectID.String()); err != nil {
			return err
		}
		tx, _ := db.CurrentTx(ctx)
		t := s.sources
		var row struct {
			N int `alias:"n"`
		}
		count := postgres.SELECT(postgres.COUNT(postgres.STAR).AS("n")).FROM(t).
			WHERE(t.OrgID.EQ(postgres.UUID(src.OrgID)).AND(t.ProjectID.EQ(postgres.UUID(src.ProjectID))))
		if err := count.QueryContext(ctx, tx, &row); err != nil {
			return err
		}
		if row.N >= MaxSourcesPerProject {
			return &LimitReachedError{Reason: LimitSourcesPerProject, Max: MaxSourcesPerProject}
		}
		return s.insertSource(ctx, src)
	})
	if err != nil && !IsLimitReachedError(err) && !IsSourceAlreadyExistsError(err) {
		return fmt.Errorf("knowledge.postgres: InsertSource: %w", err)
	}
	return err
}

func (s *postgresStore) insertSource(ctx context.Context, src *Source) error {
	t := s.sources
	code, reason := failureColumns(src.Failure)
	stmt := t.INSERT(t.AllColumns).VALUES(
		src.ID, src.PublicID, src.OrgID, src.ProjectID, string(src.Kind), string(src.Backend), src.Name, nullableText(src.SeedURL),
		string(src.Status), code, reason, false, entity.NullTimestampz(), entity.NullText(), entity.NullTimestampz(),
		src.CreatedAt.UTC(), src.UpdatedAt.UTC(),
	)
	if err := db.Write(ctx, s.pool, func(q qrm.DB) error { _, err := stmt.ExecContext(ctx, q); return err }); err != nil {
		if db.IsUniqueViolation(err) {
			return &SourceAlreadyExistsError{Name: src.Name}
		}
		return err
	}
	return nil
}

// LockSource loads a source FOR UPDATE until the unit of work on ctx ends.
func (s *postgresStore) LockSource(ctx context.Context, id uuid.UUID) (*Source, error) {
	tc, tx, err := lockScope(ctx)
	if err != nil {
		return nil, err
	}
	t := s.sources
	stmt := postgres.SELECT(t.AllColumns).FROM(t).
		WHERE(t.ID.EQ(postgres.UUID(id)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID)))).
		FOR(postgres.UPDATE())
	var row sourceRow
	if err := stmt.QueryContext(ctx, tx, &row); err != nil {
		if db.IsNoRows(err) {
			return nil, &SourceNotFoundError{}
		}
		return nil, fmt.Errorf("knowledge.postgres: LockSource: %w", err)
	}
	return row.toSource(), nil
}

// CountLiveDocuments counts a source's documents that have not failed.
func (s *postgresStore) CountLiveDocuments(ctx context.Context, sourceID uuid.UUID) (int, error) {
	t := s.documents
	stmt := postgres.SELECT(postgres.COUNT(t.ID).AS("count.n")).FROM(t).
		WHERE(t.SourceID.EQ(postgres.UUID(sourceID)).AND(t.Status.NOT_EQ(postgres.String(string(DocumentFailed)))))
	var row struct {
		N int64 `alias:"count.n"`
	}
	if err := db.ReadPrimary(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		return 0, fmt.Errorf("knowledge.postgres: CountLiveDocuments: %w", err)
	}
	return int(row.N), nil
}

// InsertDocument records a new document.
func (s *postgresStore) InsertDocument(ctx context.Context, d *Document) error {
	if err := sameOrg(ctx, d.OrgID); err != nil {
		return err
	}
	t := s.documents
	code, reason := failureColumns(d.Failure)
	fileID := entity.NullUUID()
	if d.FileID != nil {
		fileID = postgres.UUID(*d.FileID)
	}
	pages := pageCount(d.PageCount)
	stmt := t.INSERT(t.AllColumns).VALUES(
		d.ID, d.PublicID, d.OrgID, d.ProjectID, d.SourceID, d.Title, fileID, nullableText(d.URL), entity.NullJSONB(), entity.NullText(),
		string(d.Status), code, reason, pages, d.EmbedTokens, d.EmbedCostMicros, 0, entity.NullText(), entity.NullTimestampz(),
		d.CreatedAt.UTC(), d.UpdatedAt.UTC(),
	)
	if err := db.Write(ctx, s.pool, func(q qrm.DB) error { _, err := stmt.ExecContext(ctx, q); return err }); err != nil {
		return fmt.Errorf("knowledge.postgres: InsertDocument: %w", err)
	}
	return nil
}

// LockDocument loads a document FOR UPDATE until the unit of work on ctx ends.
func (s *postgresStore) LockDocument(ctx context.Context, id uuid.UUID) (*Document, error) {
	return s.lockDocument(ctx, s.documents.ID.EQ(postgres.UUID(id)))
}

// LockDocumentByFile loads the document of a file FOR UPDATE until the unit of work on ctx ends.
func (s *postgresStore) LockDocumentByFile(ctx context.Context, fileID uuid.UUID) (*Document, error) {
	return s.lockDocument(ctx, s.documents.FileID.EQ(postgres.UUID(fileID)))
}

func (s *postgresStore) lockDocument(ctx context.Context, where postgres.BoolExpression) (*Document, error) {
	tc, tx, err := lockScope(ctx)
	if err != nil {
		return nil, err
	}
	t := s.documents
	stmt := postgres.SELECT(t.AllColumns).FROM(t).
		WHERE(where.AND(t.OrgID.EQ(postgres.UUID(tc.OrgID)))).
		LIMIT(1).
		FOR(postgres.UPDATE())
	var row documentRow
	if err := stmt.QueryContext(ctx, tx, &row); err != nil {
		if db.IsNoRows(err) {
			return nil, &DocumentNotFoundError{}
		}
		return nil, fmt.Errorf("knowledge.postgres: lockDocument: %w", err)
	}
	return row.toDocument(), nil
}

// SaveDocument writes a document's status, failure, counts and update time.
func (s *postgresStore) SaveDocument(ctx context.Context, d *Document) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	t := s.documents
	code, reason := failureColumns(d.Failure)
	pages := pageCount(d.PageCount)
	stmt := t.UPDATE().SET(
		t.Status.SET(postgres.String(string(d.Status))),
		t.ErrorCode.SET(code),
		t.ErrorReason.SET(reason),
		t.PageCount.SET(pages),
		t.EmbedTokens.SET(postgres.Int(d.EmbedTokens)),
		t.EmbedCostMicros.SET(postgres.Int(d.EmbedCostMicros)),
		t.UpdatedAt.SET(postgres.TimestampzT(d.UpdatedAt.UTC())),
	).WHERE(t.ID.EQ(postgres.UUID(d.ID)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID))))
	var n int64
	err = db.Write(ctx, s.pool, func(q qrm.DB) error {
		res, err := stmt.ExecContext(ctx, q)
		if err != nil {
			return err
		}
		n, err = res.RowsAffected()
		return err
	})
	if err != nil {
		return fmt.Errorf("knowledge.postgres: SaveDocument: %w", err)
	}
	if n == 0 {
		return &DocumentNotFoundError{}
	}
	return nil
}

func lockScope(ctx context.Context) (tenant.Context, qrm.DB, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return tenant.Context{}, nil, err
	}
	tx, ok := db.CurrentTx(ctx)
	if !ok {
		return tenant.Context{}, nil, errLockOutsideTx
	}
	return tc, tx, nil
}

// SECURITY: without RLS in force, a fresh row naming another org would otherwise land in it.
func sameOrg(ctx context.Context, orgID uuid.UUID) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	if orgID != tc.OrgID {
		return &SourceNotFoundError{}
	}
	return nil
}

func failureColumns(f *Failure) (code, reason postgres.StringExpression) {
	if f == nil {
		return entity.NullText(), entity.NullText()
	}
	return postgres.String(string(f.Code)), nullableText(f.Reason)
}

func nullableText(v string) postgres.StringExpression {
	if v == "" {
		return entity.NullText()
	}
	return postgres.String(v)
}

func pageCount(n int) postgres.IntegerExpression {
	if n < 1 {
		return entity.NullInteger()
	}
	return postgres.Int(int64(n))
}

// LockSettings loads a project's embedding model FOR UPDATE, or *EmbeddingModelNotSetError.
func (s *postgresStore) LockSettings(ctx context.Context, projectID uuid.UUID) (*Settings, error) {
	tc, tx, err := lockScope(ctx)
	if err != nil {
		return nil, err
	}
	t := s.settings
	stmt := postgres.SELECT(t.AllColumns).FROM(t).
		WHERE(t.ProjectID.EQ(postgres.UUID(projectID)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID)))).
		FOR(postgres.UPDATE())
	var row settingsRow
	if err := stmt.QueryContext(ctx, tx, &row); err != nil {
		if db.IsNoRows(err) {
			return nil, &EmbeddingModelNotSetError{}
		}
		return nil, fmt.Errorf("knowledge.postgres: LockSettings: %w", err)
	}
	return row.toSettings(), nil
}

// SaveSettings inserts or replaces a project's embedding model.
func (s *postgresStore) SaveSettings(ctx context.Context, st *Settings) error {
	if err := sameOrg(ctx, st.OrgID); err != nil {
		return err
	}
	t := s.settings
	stmt := t.INSERT(t.AllColumns).
		VALUES(st.ProjectID, st.OrgID, st.ProviderID, st.Model, st.Dims, st.UpdatedAt.UTC()).
		ON_CONFLICT(t.ProjectID).
		// SECURITY: the conflict clause is guarded by org, or a row naming another tenant's project would rewrite its setting.
		DO_UPDATE(postgres.SET(
			t.ProviderID.SET(postgres.UUID(st.ProviderID)),
			t.Model.SET(postgres.String(st.Model)),
			t.Dims.SET(postgres.Int(int64(st.Dims))),
			t.UpdatedAt.SET(postgres.TimestampzT(st.UpdatedAt.UTC())),
		).WHERE(t.OrgID.EQ(postgres.UUID(st.OrgID))))
	if err := db.Write(ctx, s.pool, func(q qrm.DB) error { _, err := stmt.ExecContext(ctx, q); return err }); err != nil {
		return fmt.Errorf("knowledge.postgres: SaveSettings: %w", err)
	}
	return nil
}

// RequeueDocuments sends a project's ready, processing and embedding-failed documents back to pending, dropping their leases.
func (s *postgresStore) RequeueDocuments(ctx context.Context, projectID uuid.UUID) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	t := s.documents
	stmt := t.UPDATE().SET(
		t.Status.SET(postgres.String(string(DocumentPending))),
		t.ErrorCode.SET(entity.NullText()),
		t.ErrorReason.SET(entity.NullText()),
		t.Attempts.SET(postgres.Int(0)),
		t.LeaseOwner.SET(entity.NullText()),
		t.LeaseUntil.SET(entity.NullTimestampz()),
	).WHERE(t.OrgID.EQ(postgres.UUID(tc.OrgID)).AND(t.ProjectID.EQ(postgres.UUID(projectID))).AND(
		t.Status.IN(postgres.String(string(DocumentReady)), postgres.String(string(DocumentProcessing))).
			OR(t.Status.EQ(postgres.String(string(DocumentFailed))).
				AND(t.ErrorCode.EQ(postgres.String(string(apperror.CodeKnowledgeEmbeddingFailed))))),
	))
	if err := db.Write(ctx, s.pool, func(q qrm.DB) error { _, err := stmt.ExecContext(ctx, q); return err }); err != nil {
		return fmt.Errorf("knowledge.postgres: RequeueDocuments: %w", err)
	}
	return nil
}

var (
	errLeaseLost    = errors.New("knowledge.postgres: the lease was lost")
	errModelChanged = errors.New("knowledge.postgres: the project's embedding model changed")
)

const (
	// MaxIngestPerProject caps a project's documents in ingest at once.
	MaxIngestPerProject = 2
	// MaxChunksPerProject caps the chunks a project stores.
	MaxChunksPerProject = 20000
	chunkInsertBatch    = 500
)

// ClaimDocuments leases up to limit claimable documents of the org on ctx, at most MaxIngestPerProject per project, oldest first.
func (s *postgresStore) ClaimDocuments(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]Claim, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	var claims []Claim
	err = db.NewTransactor(s.pool).RunInTx(ctx, func(ctx context.Context) error {
		if err := db.LockInTx(ctx, "knowledge-ingest:"+tc.OrgID.String()); err != nil {
			return err
		}
		tx, _ := db.CurrentTx(ctx)
		t := s.documents
		claimable := t.Status.EQ(postgres.String(string(DocumentPending))).
			OR(t.Status.EQ(postgres.String(string(DocumentProcessing))).AND(t.LeaseUntil.LT_EQ(postgres.TimestampzT(now.UTC()))))
		crashed := t.UPDATE().SET(
			t.Status.SET(postgres.String(string(DocumentFailed))),
			t.ErrorCode.SET(postgres.String(string(apperror.CodeKnowledgeCouldNotReadFile))),
			t.ErrorReason.SET(postgres.String(string(UnreadableCrashed))),
			t.LeaseOwner.SET(entity.NullText()),
			t.LeaseUntil.SET(entity.NullTimestampz()),
			t.UpdatedAt.SET(postgres.TimestampzT(now.UTC())),
		).WHERE(t.OrgID.EQ(postgres.UUID(tc.OrgID)).AND(claimable).AND(t.Attempts.GT_EQ(postgres.Int(MaxIngestAttempts))))
		if _, err := crashed.ExecContext(ctx, tx); err != nil {
			return err
		}
		ids, err := queryIDs(ctx, tx, s.sql.claimable, tc.OrgID, now.UTC(), MaxIngestPerProject, limit)
		if err != nil {
			return err
		}
		for _, id := range ids {
			token := uuid.NewString()
			upd := t.UPDATE().SET(
				t.Status.SET(postgres.String(string(DocumentProcessing))),
				t.LeaseOwner.SET(postgres.String(token)),
				t.LeaseUntil.SET(postgres.TimestampzT(now.Add(lease).UTC())),
				t.Attempts.SET(t.Attempts.ADD(postgres.Int(1))),
			).WHERE(t.ID.EQ(postgres.UUID(id)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID)))).RETURNING(t.AllColumns)
			var row documentRow
			if err := upd.QueryContext(ctx, tx, &row); err != nil {
				return err
			}
			claims = append(claims, Claim{Document: row.toDocument(), Token: token})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("knowledge.postgres: ClaimDocuments: %w", err)
	}
	return claims, nil
}

func queryIDs(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]uuid.UUID, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// RenewLease moves a claimed document's lease to until, or answers errLeaseLost when the claim no longer holds it.
func (s *postgresStore) RenewLease(ctx context.Context, c Claim, until time.Time) error {
	t := s.documents
	return s.renew(ctx, "RenewLease", t.UPDATE().SET(t.LeaseUntil.SET(postgres.TimestampzT(until.UTC()))).WHERE(s.held(c)))
}

// RenewSourceLease moves a crawl's lease to until, or answers errLeaseLost when the claim no longer holds it.
func (s *postgresStore) RenewSourceLease(ctx context.Context, c SourceClaim, until time.Time) error {
	t := s.sources
	return s.renew(ctx, "RenewSourceLease", t.UPDATE().SET(t.LeaseUntil.SET(postgres.TimestampzT(until.UTC()))).
		WHERE(t.ID.EQ(postgres.UUID(c.Source.ID)).AND(t.OrgID.EQ(postgres.UUID(c.Source.OrgID))).AND(t.LeaseOwner.EQ(postgres.String(c.Token)))))
}

func (s *postgresStore) renew(ctx context.Context, op string, stmt postgres.Statement) error {
	var n int64
	err := db.Write(ctx, s.pool, func(q qrm.DB) error {
		res, err := stmt.ExecContext(ctx, q)
		if err != nil {
			return err
		}
		n, err = res.RowsAffected()
		return err
	})
	if err != nil {
		return fmt.Errorf("knowledge.postgres: %s: %w", op, err)
	}
	if n == 0 {
		return errLeaseLost
	}
	return nil
}

// ReleaseDocument hands a claimed document back to pending, if the claim still holds its lease.
func (s *postgresStore) ReleaseDocument(ctx context.Context, c Claim, spent Spend, now time.Time) error {
	t := s.documents
	return s.endLease(ctx, c, "ReleaseDocument",
		t.Status.SET(postgres.String(string(DocumentPending))),
		t.EmbedTokens.SET(t.EmbedTokens.ADD(postgres.Int(spent.Tokens))),
		t.EmbedCostMicros.SET(t.EmbedCostMicros.ADD(postgres.Int(spent.CostMicros))),
		t.UpdatedAt.SET(postgres.TimestampzT(now.UTC())),
	)
}

// FailDocument fails a claimed document with f and adds what the attempt paid, if the claim still holds its lease.
func (s *postgresStore) FailDocument(ctx context.Context, c Claim, f Failure, spent Spend, now time.Time) error {
	t := s.documents
	code, reason := failureColumns(&f)
	return s.endLease(ctx, c, "FailDocument",
		t.Status.SET(postgres.String(string(DocumentFailed))),
		t.ErrorCode.SET(code),
		t.ErrorReason.SET(reason),
		t.EmbedTokens.SET(t.EmbedTokens.ADD(postgres.Int(spent.Tokens))),
		t.EmbedCostMicros.SET(t.EmbedCostMicros.ADD(postgres.Int(spent.CostMicros))),
		t.UpdatedAt.SET(postgres.TimestampzT(now.UTC())),
	)
}

func (s *postgresStore) endLease(ctx context.Context, c Claim, op string, set ...postgres.ColumnAssigment) error {
	t := s.documents
	set = append(set, t.LeaseOwner.SET(entity.NullText()), t.LeaseUntil.SET(entity.NullTimestampz()))
	stmt := t.UPDATE().SET(set[0], toAny(set[1:])...).
		WHERE(s.held(c))
	if err := db.Write(ctx, s.pool, func(q qrm.DB) error { _, err := stmt.ExecContext(ctx, q); return err }); err != nil {
		return fmt.Errorf("knowledge.postgres: %s: %w", op, err)
	}
	return nil
}

func (s *postgresStore) held(c Claim) postgres.BoolExpression {
	t := s.documents
	return t.ID.EQ(postgres.UUID(c.Document.ID)).AND(t.OrgID.EQ(postgres.UUID(c.Document.OrgID))).
		AND(t.LeaseOwner.EQ(postgres.String(c.Token)))
}

func toAny(in []postgres.ColumnAssigment) []any {
	out := make([]any, len(in))
	for i, v := range in {
		out[i] = v
	}
	return out
}

// ReplaceChunks swaps a claimed document's chunks in one unit of work, only while st is the project's model and the claim holds its lease.
func (s *postgresStore) ReplaceChunks(ctx context.Context, c Claim, st Settings, res IngestResult, now time.Time) error {
	doc := c.Document
	err := db.NewTransactor(s.pool).RunInTx(ctx, func(ctx context.Context) error {
		tx, _ := db.CurrentTx(ctx)
		cur, err := s.LockSettings(ctx, doc.ProjectID)
		if err != nil && !IsEmbeddingModelNotSetError(err) {
			return err
		}
		if cur == nil || cur.ModelKey() != st.ModelKey() {
			return errModelChanged
		}
		var held int
		if err := tx.QueryRowContext(ctx, s.sql.leaseHeld, doc.ID, doc.OrgID, c.Token).Scan(&held); err != nil {
			return err
		}
		if held == 0 {
			return errLeaseLost
		}
		var others int
		if err := tx.QueryRowContext(ctx, s.sql.otherChunks, doc.OrgID, doc.ProjectID, doc.ID).Scan(&others); err != nil {
			return err
		}
		if others+len(res.Chunks) > MaxChunksPerProject {
			return &LimitReachedError{Reason: LimitChunksPerProject, Max: MaxChunksPerProject}
		}
		if _, err := tx.ExecContext(ctx, s.sql.deleteChunks, doc.OrgID, doc.ID); err != nil {
			return err
		}
		for batch := range slices.Chunk(res.Chunks, chunkInsertBatch) {
			if err := s.insertChunks(ctx, tx, doc, st, batch, now); err != nil {
				return err
			}
		}
		t := s.documents
		ready := t.UPDATE().SET(
			t.Status.SET(postgres.String(string(DocumentReady))),
			t.ErrorCode.SET(entity.NullText()),
			t.ErrorReason.SET(entity.NullText()),
			t.PageCount.SET(pageCount(res.Pages)),
			t.EmbedTokens.SET(t.EmbedTokens.ADD(postgres.Int(res.Tokens))),
			t.EmbedCostMicros.SET(t.EmbedCostMicros.ADD(postgres.Int(res.CostMicros))),
			t.Content.SET(entity.NullJSONB()),
			t.Attempts.SET(postgres.Int(0)),
			t.LeaseOwner.SET(entity.NullText()),
			t.LeaseUntil.SET(entity.NullTimestampz()),
			t.UpdatedAt.SET(postgres.TimestampzT(now.UTC())),
		).WHERE(t.ID.EQ(postgres.UUID(doc.ID)).AND(t.OrgID.EQ(postgres.UUID(doc.OrgID))))
		_, err = ready.ExecContext(ctx, tx)
		return err
	})
	if err != nil && !errors.Is(err, errLeaseLost) && !errors.Is(err, errModelChanged) && !IsLimitReachedError(err) {
		return fmt.Errorf("knowledge.postgres: ReplaceChunks: %w", err)
	}
	return err
}

func (s *postgresStore) insertChunks(
	ctx context.Context, tx *sql.Tx, doc *Document, st Settings, batch []StoredChunk, now time.Time,
) error {
	n := len(batch)
	ids, ordinals, pages := make([]string, n), make([]int32, n), make([]int32, n)
	headings, urls, texts, vectors := make([]string, n), make([]string, n), make([]string, n), make([]string, n)
	for i, c := range batch {
		//nolint:gosec // G115: both are clamped to int32 first.
		ids[i], ordinals[i], pages[i] = uuid.NewString(), int32(min(c.Ordinal, math.MaxInt32)), int32(min(c.Page, math.MaxInt32))
		headings[i], urls[i], texts[i], vectors[i] = c.HeadingPath, c.URL, c.Text, VectorText(c.Embedding)
	}
	_, err := tx.ExecContext(ctx, s.sql.insertChunks,
		doc.OrgID, doc.ProjectID, doc.SourceID, doc.ID, st.Dims, st.ModelKey(), now.UTC(),
		ids, ordinals, headings, pages, urls, texts, vectors)
	return err
}

// MarkCrawling queues a URL source for its crawl and clears its failure; a crawl in progress runs again when it ends.
func (s *postgresStore) MarkCrawling(ctx context.Context, sourceID uuid.UUID, now time.Time) (*Source, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	t := s.sources
	stmt := t.UPDATE().SET(
		t.Status.SET(postgres.String(string(SourceCrawling))),
		t.ErrorCode.SET(entity.NullText()),
		t.ErrorReason.SET(entity.NullText()),
		t.RefreshRequested.SET(t.LeaseOwner.IS_NOT_NULL()),
		t.RefreshedAt.SET(postgres.TimestampzT(now.UTC())),
		t.UpdatedAt.SET(postgres.TimestampzT(now.UTC())),
	).WHERE(t.ID.EQ(postgres.UUID(sourceID)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID))).
		AND(t.RefreshedAt.IS_NULL().OR(t.RefreshedAt.LT_EQ(postgres.TimestampzT(now.Add(-RefreshInterval).UTC()))))).
		RETURNING(t.AllColumns)
	var row sourceRow
	if err := db.Write(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		if !db.IsNoRows(err) {
			return nil, fmt.Errorf("knowledge.postgres: MarkCrawling: %w", err)
		}
		if _, err := s.SourceByID(ctx, sourceID); err != nil {
			return nil, err
		}
		return nil, &LimitReachedError{Reason: LimitRefreshTooSoon, Max: int(RefreshInterval / time.Minute)}
	}
	return row.toSource(), nil
}

// ClaimSource leases one crawling source of the org on ctx whose lease is free or ran out, or returns nil; one claimer per org at a time.
func (s *postgresStore) ClaimSource(ctx context.Context, now time.Time, lease time.Duration) (*SourceClaim, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	var claim *SourceClaim
	err = db.NewTransactor(s.pool).RunInTx(ctx, func(ctx context.Context) error {
		if err := db.LockInTx(ctx, "knowledge-crawl:"+tc.OrgID.String()); err != nil {
			return err
		}
		tx, _ := db.CurrentTx(ctx)
		t := s.sources
		cand := postgres.SELECT(t.ID).FROM(t).
			WHERE(t.OrgID.EQ(postgres.UUID(tc.OrgID)).AND(t.Status.EQ(postgres.String(string(SourceCrawling)))).
				AND(t.LeaseUntil.IS_NULL().OR(t.LeaseUntil.LT_EQ(postgres.TimestampzT(now.UTC()))))).
			ORDER_BY(t.UpdatedAt.ASC(), t.ID.ASC()).LIMIT(1).
			FOR(postgres.UPDATE())
		var row struct {
			ID uuid.UUID `alias:"knowledge_sources.id"`
		}
		if err := cand.QueryContext(ctx, tx, &row); err != nil {
			if db.IsNoRows(err) {
				return nil
			}
			return err
		}
		token := uuid.NewString()
		upd := t.UPDATE().SET(
			t.LeaseOwner.SET(postgres.String(token)),
			t.LeaseUntil.SET(postgres.TimestampzT(now.Add(lease).UTC())),
			t.RefreshRequested.SET(postgres.Bool(false)),
		).WHERE(t.ID.EQ(postgres.UUID(row.ID))).RETURNING(t.AllColumns)
		var src sourceRow
		if err := upd.QueryContext(ctx, tx, &src); err != nil {
			return err
		}
		claim = &SourceClaim{Source: src.toSource(), Token: token}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("knowledge.postgres: ClaimSource: %w", err)
	}
	return claim, nil
}

// UpsertPage records a crawled page as a pending document of the claimed source; an unchanged ready page stays as it is.
func (s *postgresStore) UpsertPage(ctx context.Context, c SourceClaim, p Page, now time.Time) error {
	publicID, err := nanoid.New(publicIDLen)
	if err != nil {
		return err
	}
	return s.whileCrawling(ctx, c, now, "UpsertPage", func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, s.sql.upsertPage, uuid.New(), publicID, p.Title, p.URL, string(p.Sections), now.UTC(),
			c.Source.ID, c.Source.OrgID, p.Hash)
		return err
	})
}

// DeletePages removes the claimed source's pages at urls, with their chunks.
func (s *postgresStore) DeletePages(ctx context.Context, c SourceClaim, urls []string, now time.Time) error {
	if len(urls) == 0 {
		return nil
	}
	t := s.documents
	in := make([]postgres.Expression, len(urls))
	for i, u := range urls {
		in[i] = postgres.String(u)
	}
	stmt := t.DELETE().WHERE(t.OrgID.EQ(postgres.UUID(c.Source.OrgID)).AND(t.SourceID.EQ(postgres.UUID(c.Source.ID))).AND(t.URL.IN(in...)))
	return s.whileCrawling(ctx, c, now, "DeletePages", func(ctx context.Context, tx *sql.Tx) error {
		_, err := stmt.ExecContext(ctx, tx)
		return err
	})
}

// SECURITY: a crawl writes only while its claim still holds a live lease; a lost lease stops it with errLeaseLost.
func (s *postgresStore) whileCrawling(
	ctx context.Context, c SourceClaim, now time.Time, op string, fn func(context.Context, *sql.Tx) error,
) error {
	err := db.NewTransactor(s.pool).RunInTx(ctx, func(ctx context.Context) error {
		tx, _ := db.CurrentTx(ctx)
		t := s.sources
		held := postgres.SELECT(t.ID).FROM(t).
			WHERE(t.ID.EQ(postgres.UUID(c.Source.ID)).AND(t.OrgID.EQ(postgres.UUID(c.Source.OrgID))).
				AND(t.LeaseOwner.EQ(postgres.String(c.Token))).AND(t.LeaseUntil.GT(postgres.TimestampzT(now.UTC())))).
			FOR(postgres.SHARE())
		var row struct {
			ID uuid.UUID `alias:"knowledge_sources.id"`
		}
		if err := held.QueryContext(ctx, tx, &row); err != nil {
			if db.IsNoRows(err) {
				return errLeaseLost
			}
			return err
		}
		return fn(ctx, tx)
	})
	if err != nil && !errors.Is(err, errLeaseLost) {
		return fmt.Errorf("knowledge.postgres: %s: %w", op, err)
	}
	return err
}

// FinishCrawl ends a crawl's lease: idle, or failed with f; a refresh asked for during the crawl queues it again.
func (s *postgresStore) FinishCrawl(ctx context.Context, c SourceClaim, f *Failure, now time.Time) error {
	t := s.sources
	status := SourceIdle
	if f != nil {
		status = SourceFailed
	}
	code, reason := failureColumns(f)
	again := t.RefreshRequested.IS_TRUE()
	stmt := t.UPDATE().SET(
		t.Status.SET(postgres.StringExp(postgres.CASE().WHEN(again).
			THEN(postgres.String(string(SourceCrawling))).ELSE(postgres.String(string(status))))),
		t.ErrorCode.SET(postgres.StringExp(postgres.CASE().WHEN(again).THEN(entity.NullText()).ELSE(code))),
		t.ErrorReason.SET(postgres.StringExp(postgres.CASE().WHEN(again).THEN(entity.NullText()).ELSE(reason))),
		t.RefreshRequested.SET(postgres.Bool(false)),
		t.LeaseOwner.SET(entity.NullText()),
		t.LeaseUntil.SET(entity.NullTimestampz()),
		t.UpdatedAt.SET(postgres.TimestampzT(now.UTC())),
	).WHERE(t.ID.EQ(postgres.UUID(c.Source.ID)).AND(t.OrgID.EQ(postgres.UUID(c.Source.OrgID))).AND(t.LeaseOwner.EQ(postgres.String(c.Token))))
	if err := db.Write(ctx, s.pool, func(q qrm.DB) error { _, err := stmt.ExecContext(ctx, q); return err }); err != nil {
		return fmt.Errorf("knowledge.postgres: FinishCrawl: %w", err)
	}
	return nil
}

// LockSourcesShared locks the sources FOR SHARE until the unit of work on ctx ends; a missing one is a *SourceNotFoundError.
func (s *postgresStore) LockSourcesShared(ctx context.Context, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	tc, tx, err := lockScope(ctx)
	if err != nil {
		return err
	}
	t := s.sources
	in := make([]postgres.Expression, len(ids))
	for i, id := range ids {
		in[i] = postgres.UUID(id)
	}
	stmt := postgres.SELECT(t.ID).FROM(t).
		WHERE(t.ID.IN(in...).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID)))).
		ORDER_BY(t.ID.ASC()).
		FOR(postgres.SHARE())
	var found []struct {
		ID uuid.UUID `alias:"knowledge_sources.id"`
	}
	if err := stmt.QueryContext(ctx, tx, &found); err != nil && !db.IsNoRows(err) {
		return fmt.Errorf("knowledge.postgres: LockSourcesShared: %w", err)
	}
	if len(found) != len(ids) {
		return &SourceNotFoundError{}
	}
	return nil
}

// DeleteSource removes a source of the org on ctx with its documents and chunks.
func (s *postgresStore) DeleteSource(ctx context.Context, id uuid.UUID) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	t := s.sources
	stmt := t.DELETE().WHERE(t.ID.EQ(postgres.UUID(id)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID))))
	if err := db.Write(ctx, s.pool, func(q qrm.DB) error { _, err := stmt.ExecContext(ctx, q); return err }); err != nil {
		return fmt.Errorf("knowledge.postgres: DeleteSource: %w", err)
	}
	return nil
}

// DeleteDocument removes a document of the org on ctx with its chunks.
func (s *postgresStore) DeleteDocument(ctx context.Context, id uuid.UUID) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	t := s.documents
	stmt := t.DELETE().WHERE(t.ID.EQ(postgres.UUID(id)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID))))
	if err := db.Write(ctx, s.pool, func(q qrm.DB) error { _, err := stmt.ExecContext(ctx, q); return err }); err != nil {
		return fmt.Errorf("knowledge.postgres: DeleteDocument: %w", err)
	}
	return nil
}

// SourceFileIDs lists the files of a source's uploaded documents.
func (s *postgresStore) SourceFileIDs(ctx context.Context, sourceID uuid.UUID) ([]uuid.UUID, error) {
	t := s.documents
	stmt := postgres.SELECT(t.FileID).FROM(t).WHERE(t.SourceID.EQ(postgres.UUID(sourceID)).AND(t.FileID.IS_NOT_NULL()))
	var rows []struct {
		FileID uuid.UUID `alias:"knowledge_documents.file_id"`
	}
	if err := db.ReadPrimary(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &rows) }); err != nil && !db.IsNoRows(err) {
		return nil, fmt.Errorf("knowledge.postgres: SourceFileIDs: %w", err)
	}
	out := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		out[i] = r.FileID
	}
	return out, nil
}
