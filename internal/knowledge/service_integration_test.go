//go:build integration

package knowledge_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/storage"
	"altalune.id/oagent/internal/kernel/storage/memory"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/knowledge"
	"altalune.id/oagent/internal/testutil/pgtest"
)

type projectNames map[uuid.UUID]string

func (p projectNames) ProjectPublicID(_ context.Context, id uuid.UUID) (string, error) {
	return p[id], nil
}

type orgOf map[uuid.UUID]uuid.UUID

func (o orgOf) OrgOf(_ context.Context, projectID uuid.UUID) (uuid.UUID, error) {
	if org, ok := o[projectID]; ok {
		return org, nil
	}
	return uuid.Nil, apperror.New(apperror.CodeProjectNotFound, "no project")
}

type failingReady struct {
	knowledge.Files
	err error
}

func (f failingReady) MarkReady(ctx context.Context, id uuid.UUID) error {
	if err := f.Files.MarkReady(ctx, id); err != nil {
		return err
	}
	return f.err
}

type env struct {
	admin    *sql.DB
	pool     db.Pool
	driver   *memory.Driver
	files    *storage.Service
	store    knowledge.Store
	svc      *knowledge.Service
	ctx      context.Context
	org, prj uuid.UUID
	other    context.Context
	otherOrg uuid.UUID
	otherPrj uuid.UUID
	names    projectNames
}

func newEnv(t *testing.T) *env {
	t.Helper()
	pool, admin, schemaName := pgtest.MigratedRLS(t)
	e := &env{admin: admin, pool: pool, driver: memory.New(), names: projectNames{}}
	e.org, e.prj = e.seed(t)
	otherOrg, otherPrj := e.seed(t)
	e.otherOrg, e.otherPrj = otherOrg, otherPrj
	e.ctx = tenant.Into(t.Context(), tenant.Context{OrgID: e.org, ProjectID: e.prj})
	e.other = tenant.Into(t.Context(), tenant.Context{OrgID: otherOrg, ProjectID: otherPrj})
	unexpected := apperror.NewReporter(nil, false).Unexpected
	e.files = storage.NewService(e.driver, storage.NewStore(pool, schemaName, ""), db.NewTransactor(pool),
		storage.Config{App: "oagent", UploadURLTTL: 15 * time.Minute, PendingTTL: time.Hour}, unexpected)
	e.store = knowledge.NewStore(pool, schemaName, "")
	e.svc = knowledge.NewService(e.store, db.NewTransactor(pool), e.files, e.names, pgvectorOn, unexpected).
		WithProjects(orgOf{e.prj: e.org, otherPrj: otherOrg})
	return e
}

func (e *env) seed(t *testing.T) (org, prj uuid.UUID) {
	t.Helper()
	org, prj = uuid.New(), uuid.New()
	now := time.Now().UTC()
	_, err := e.admin.ExecContext(t.Context(), `INSERT INTO orgs (id, public_id, slug, name, created_at, updated_at) VALUES ($1, $2, $2, 'Org', $3, $3)`,
		org, "o"+org.String()[:12], now)
	require.NoError(t, err)
	_, err = e.admin.ExecContext(t.Context(), `INSERT INTO projects (id, public_id, org_id, slug, name, created_at, updated_at) VALUES ($1, $2, $3, 'p', 'P', $4, $4)`,
		prj, "p"+prj.String()[:12], org, now)
	require.NoError(t, err)
	e.names[prj] = "p" + prj.String()[:12]
	return org, prj
}

func (e *env) uploadSource(t *testing.T, name string) *knowledge.Source {
	t.Helper()
	src, err := e.svc.CreateSource(e.ctx, e.prj, knowledge.SourceInput{Kind: knowledge.KindUpload, Backend: knowledge.BackendPgvector, Name: name})
	require.NoError(t, err)
	return src
}

func (e *env) fileStatus(t *testing.T, id uuid.UUID) string {
	t.Helper()
	var status string
	require.NoError(t, e.admin.QueryRowContext(t.Context(), `SELECT status FROM files WHERE id = $1`, id).Scan(&status))
	return status
}

func (e *env) fileKey(t *testing.T, id uuid.UUID) string {
	t.Helper()
	var key string
	require.NoError(t, e.admin.QueryRowContext(t.Context(), `SELECT object_key FROM files WHERE id = $1`, id).Scan(&key))
	return key
}

var pdf = append([]byte("%PDF-1.7\n"), bytes.Repeat([]byte{0xab}, 91)...)

func (e *env) begin(t *testing.T, src *knowledge.Source) (*knowledge.Document, storage.Upload) {
	t.Helper()
	doc, up, err := e.svc.BeginUpload(e.ctx, src.ID, knowledge.UploadInput{Filename: "handbook.pdf", ContentType: knowledge.TypePDF, Size: int64(len(pdf))})
	require.NoError(t, err)
	return doc, up
}

func TestUpload_PresignShape(t *testing.T) {
	e := newEnv(t)
	src := e.uploadSource(t, "Policies")
	before := time.Now()
	doc, up, err := e.svc.BeginUpload(e.ctx, src.ID, knowledge.UploadInput{Filename: "handbook.pdf", ContentType: knowledge.TypePDF, Size: 1234})
	require.NoError(t, err)

	assert.Equal(t, "PUT", up.Method)
	assert.Equal(t, map[string]string{"Content-Type": knowledge.TypePDF}, up.Headers, "only headers a browser may set; the signature binds the length")
	assert.WithinDuration(t, before.Add(15*time.Minute), up.ExpiresAt, time.Minute)
	key := e.fileKey(t, *doc.FileID)
	assert.Equal(t, "private/oagent/projects/"+e.names[e.prj]+"/knowledge/", key[:len("private/oagent/projects/"+e.names[e.prj]+"/knowledge/")])
	assert.Contains(t, up.URL, url.PathEscape(key))

	assert.Equal(t, knowledge.DocumentUploading, doc.Status)
	assert.Equal(t, "pending", e.fileStatus(t, *doc.FileID))
	got, err := e.svc.GetDocument(e.ctx, doc.ID)
	require.NoError(t, err)
	assert.Equal(t, doc.PublicID, got.PublicID)
	assert.Equal(t, "handbook.pdf", got.Title)
}

func TestUpload_RefusesWhatTheSourceCannotTake(t *testing.T) {
	e := newEnv(t)
	src := e.uploadSource(t, "Policies")
	for name, in := range map[string]knowledge.UploadInput{
		"type":  {Filename: "a.png", ContentType: "image/png", Size: 10},
		"size":  {Filename: "a.pdf", ContentType: knowledge.TypePDF, Size: knowledge.MaxUploadBytes + 1},
		"empty": {Filename: "a.pdf", ContentType: knowledge.TypePDF, Size: 0},
		"title": {Filename: " ", ContentType: knowledge.TypePDF, Size: 10},
	} {
		_, _, err := e.svc.BeginUpload(e.ctx, src.ID, in)
		assert.Equal(t, apperror.CodeValidation, codeOf(err), "%s: %v", name, err)
	}
	var n int
	require.NoError(t, e.admin.QueryRowContext(t.Context(), `SELECT count(*) FROM files`).Scan(&n))
	assert.Zero(t, n, "a refused upload leaves no file row")

	site, err := e.svc.CreateSource(e.ctx, e.prj, knowledge.SourceInput{Kind: knowledge.KindURL, Backend: knowledge.BackendPgvector, Name: "Site", SeedURL: "https://e.com"})
	require.NoError(t, err)
	_, _, err = e.svc.BeginUpload(e.ctx, site.ID, knowledge.UploadInput{Filename: "a.pdf", ContentType: knowledge.TypePDF, Size: 10})
	assert.Equal(t, apperror.CodeKnowledgeSourceInvalid, codeOf(err), "a URL source takes no uploads")

	_, _, err = e.svc.BeginUpload(e.other, src.ID, knowledge.UploadInput{Filename: "a.pdf", ContentType: knowledge.TypePDF, Size: 10})
	assert.True(t, knowledge.IsSourceNotFoundError(err), "another org's source: %v", err)
}

func TestUpload_DocumentCapPerSource(t *testing.T) {
	e := newEnv(t)
	src := e.uploadSource(t, "Policies")
	_, err := e.admin.ExecContext(t.Context(), `INSERT INTO knowledge_documents (id, public_id, org_id, project_id, source_id, title, status, created_at, updated_at)
		SELECT gen_random_uuid(), 'bulk' || g, $1, $2, $3, 't', 'ready', now(), now() FROM generate_series(1, 99) g`, e.org, e.prj, src.ID)
	require.NoError(t, err)
	_, err = e.admin.ExecContext(t.Context(), `INSERT INTO knowledge_documents (id, public_id, org_id, project_id, source_id, title, status, created_at, updated_at)
		VALUES (gen_random_uuid(), 'gone', $1, $2, $3, 't', 'failed', now(), now())`, e.org, e.prj, src.ID)
	require.NoError(t, err)

	e.begin(t, src)
	_, _, err = e.svc.BeginUpload(e.ctx, src.ID, knowledge.UploadInput{Filename: "b.pdf", ContentType: knowledge.TypePDF, Size: 10})
	var lim *knowledge.LimitReachedError
	require.ErrorAs(t, err, &lim, "a failed document does not count, the 100th live one does")
	assert.Equal(t, knowledge.LimitDocumentsPerSource, lim.Reason)
	assert.Equal(t, knowledge.MaxDocumentsPerSource, lim.Max)
}

func TestCompleteUpload_AttachesAndQueues(t *testing.T) {
	e := newEnv(t)
	src := e.uploadSource(t, "Policies")
	doc, _ := e.begin(t, src)
	e.driver.Put(e.fileKey(t, *doc.FileID), knowledge.TypePDF, pdf)
	kicks := 0
	e.svc.WithKick(func() { kicks++ })

	got, err := e.svc.CompleteUpload(e.ctx, doc.ID)
	assert.Equal(t, 1, kicks, "a queued document wakes the ingest worker")
	require.NoError(t, err)
	assert.Equal(t, knowledge.DocumentPending, got.Status)
	assert.Equal(t, "ready", e.fileStatus(t, *doc.FileID))

	again, err := e.svc.CompleteUpload(e.ctx, doc.ID)
	require.NoError(t, err, "a retried completion is a no-op")
	assert.Equal(t, knowledge.DocumentPending, again.Status)
}

func TestCompleteUpload_MarkReadyInSameTx(t *testing.T) {
	e := newEnv(t)
	src := e.uploadSource(t, "Policies")
	doc, _ := e.begin(t, src)
	e.driver.Put(e.fileKey(t, *doc.FileID), knowledge.TypePDF, pdf)

	boom := errors.New("commit lost")
	svc := knowledge.NewService(e.store, db.NewTransactor(e.pool), failingReady{Files: e.files, err: boom}, e.names, pgvectorOn,
		apperror.NewReporter(nil, false).Unexpected)
	_, err := svc.CompleteUpload(e.ctx, doc.ID)
	require.Error(t, err)

	got, err := e.svc.GetDocument(e.ctx, doc.ID)
	require.NoError(t, err)
	assert.Equal(t, knowledge.DocumentUploading, got.Status, "the document is queued only together with its file's MarkReady")
	assert.Equal(t, "pending", e.fileStatus(t, *doc.FileID), "and the file stays pending with it")
}

func TestCompleteUpload_MismatchFailsAndDeletes(t *testing.T) {
	e := newEnv(t)
	src := e.uploadSource(t, "Policies")
	doc, _ := e.begin(t, src)
	key := e.fileKey(t, *doc.FileID)
	e.driver.Put(key, knowledge.TypePDF, append([]byte("PK\x03\x04"), bytes.Repeat([]byte{1}, 96)...))

	_, err := e.svc.CompleteUpload(e.ctx, doc.ID)
	var rej *knowledge.UploadRejectedError
	require.ErrorAs(t, err, &rej)
	assert.Equal(t, knowledge.RejectContent, rej.Reason)
	assert.False(t, e.driver.Has(key), "the object is deleted")
	assert.Equal(t, "deleting", e.fileStatus(t, *doc.FileID))
	got, err := e.svc.GetDocument(e.ctx, doc.ID)
	require.NoError(t, err)
	assert.Equal(t, knowledge.DocumentFailed, got.Status)
	assert.Equal(t, &knowledge.Failure{Code: apperror.CodeKnowledgeUploadRejected, Reason: "content"}, got.Failure)
}

func TestCompleteUpload_MissingObjectKeepsTheUploadOpen(t *testing.T) {
	e := newEnv(t)
	src := e.uploadSource(t, "Policies")
	doc, _ := e.begin(t, src)

	_, err := e.svc.CompleteUpload(e.ctx, doc.ID)
	var rej *knowledge.UploadRejectedError
	require.ErrorAs(t, err, &rej)
	assert.Equal(t, knowledge.RejectMissing, rej.Reason)
	got, err := e.svc.GetDocument(e.ctx, doc.ID)
	require.NoError(t, err)
	assert.Equal(t, knowledge.DocumentUploading, got.Status, "the browser may still finish its PUT")

	e.driver.Put(e.fileKey(t, *doc.FileID), knowledge.TypePDF, pdf)
	got, err = e.svc.CompleteUpload(e.ctx, doc.ID)
	require.NoError(t, err)
	assert.Equal(t, knowledge.DocumentPending, got.Status)
}

func TestCompleteUpload_ExpiredFails(t *testing.T) {
	e := newEnv(t)
	src := e.uploadSource(t, "Policies")
	doc, _ := e.begin(t, src)
	e.driver.Put(e.fileKey(t, *doc.FileID), knowledge.TypePDF, pdf)
	_, err := e.admin.ExecContext(t.Context(), `UPDATE files SET expires_at = now() - interval '1 second' WHERE id = $1`, *doc.FileID)
	require.NoError(t, err)

	_, err = e.svc.CompleteUpload(e.ctx, doc.ID)
	var rej *knowledge.UploadRejectedError
	require.ErrorAs(t, err, &rej)
	assert.Equal(t, knowledge.RejectExpired, rej.Reason)
	got, err := e.svc.GetDocument(e.ctx, doc.ID)
	require.NoError(t, err)
	assert.Equal(t, knowledge.DocumentFailed, got.Status)
}

func TestUploadSweep_FailsAbandonedDocument(t *testing.T) {
	e := newEnv(t)
	src := e.uploadSource(t, "Policies")
	abandoned, _ := e.begin(t, src)
	done, _ := e.begin(t, src)
	e.driver.Put(e.fileKey(t, *done.FileID), knowledge.TypePDF, pdf)
	_, err := e.svc.CompleteUpload(e.ctx, done.ID)
	require.NoError(t, err)
	_, err = e.admin.ExecContext(t.Context(), `UPDATE files SET expires_at = now() - interval '1 second'`)
	require.NoError(t, err)

	n, err := e.files.Sweep(e.ctx, e.svc)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "only the abandoned upload goes; a completed one was marked ready")

	got, err := e.svc.GetDocument(e.ctx, abandoned.ID)
	require.NoError(t, err)
	assert.Equal(t, knowledge.DocumentFailed, got.Status)
	assert.Equal(t, &knowledge.Failure{Code: apperror.CodeKnowledgeUploadRejected, Reason: "expired"}, got.Failure)
	assert.Nil(t, got.FileID, "the file row is gone, the document stays with its failure")

	views, err := e.svc.ListSources(e.ctx, e.prj)
	require.NoError(t, err)
	require.Len(t, views, 1)
	assert.Equal(t, knowledge.DocumentCounts{Total: 2, Failed: 1}, views[0].Documents)
	assert.NoError(t, e.svc.UploadAbandoned(e.ctx, uuid.New()), "a file no document waits on is fine")
}

func TestCreateSource_NamesAndTenancy(t *testing.T) {
	e := newEnv(t)
	src := e.uploadSource(t, "Policies")
	_, err := e.svc.CreateSource(e.ctx, e.prj, knowledge.SourceInput{Kind: knowledge.KindUpload, Backend: knowledge.BackendPgvector, Name: "Policies"})
	var taken *knowledge.SourceAlreadyExistsError
	require.ErrorAs(t, err, &taken)
	assert.Equal(t, apperror.CodeKnowledgeSourceAlreadyExists, codeOf(err))

	_, err = e.svc.CreateSource(e.ctx, uuid.New(), knowledge.SourceInput{Kind: knowledge.KindUpload, Backend: knowledge.BackendPgvector, Name: "X"})
	assert.True(t, tenant.IsProjectOutsideOrgError(err), "%v", err)

	_, err = e.svc.GetSource(e.other, src.ID)
	assert.True(t, knowledge.IsSourceNotFoundError(err), "another org never sees the source")
	got, err := e.svc.GetSource(e.ctx, src.ID)
	require.NoError(t, err)
	assert.Equal(t, src.PublicID, got.PublicID)

	ref, err := e.svc.ResolveSource(t.Context(), src.PublicID)
	require.NoError(t, err)
	assert.Equal(t, knowledge.SourceRef{ID: src.ID, OrgID: e.org, ProjectID: e.prj}, ref)
	_, err = e.svc.ResolveSource(t.Context(), "nope")
	assert.True(t, knowledge.IsSourceNotFoundError(err))

	doc, _ := e.begin(t, src)
	dref, err := e.svc.ResolveDocument(t.Context(), doc.PublicID)
	require.NoError(t, err)
	assert.Equal(t, knowledge.DocumentRef{ID: doc.ID, OrgID: e.org, ProjectID: e.prj, SourceID: src.ID}, dref)
	_, err = e.svc.ResolveDocument(t.Context(), "nope")
	assert.True(t, knowledge.IsDocumentNotFoundError(err))
	docs, err := e.svc.ListDocuments(e.ctx, src.ID)
	require.NoError(t, err)
	assert.Len(t, docs, 1)
	_, err = e.svc.GetDocument(e.other, doc.ID)
	assert.True(t, knowledge.IsDocumentNotFoundError(err))
}

func TestUpload_NeedsStorage(t *testing.T) {
	e := newEnv(t)
	off := knowledge.NewService(e.store, db.NewTransactor(e.pool), storage.NewService(nil, nil, nil, storage.Config{}, nil), e.names, pgvectorOn, nil)
	assert.False(t, off.StorageConfigured())
	assert.Equal(t, pgvectorOn, off.Backends())
	_, _, err := off.BeginUpload(e.ctx, uuid.New(), knowledge.UploadInput{})
	assert.Equal(t, apperror.CodeKnowledgeStorageNotConfigured, codeOf(err))
	_, err = off.CompleteUpload(e.ctx, uuid.New())
	assert.Equal(t, apperror.CodeKnowledgeStorageNotConfigured, codeOf(err))
}
