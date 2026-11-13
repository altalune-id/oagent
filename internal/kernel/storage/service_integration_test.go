//go:build integration

package storage_test

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/storage"
	storages3 "altalune.id/oagent/internal/kernel/storage/s3"
	"altalune.id/oagent/internal/testutil/s3test"
	s3conn "altalune.id/oagent/s3"
)

var knowledge = storage.Purpose{
	Name: "knowledge", OwnerKind: storage.OwnerProjects, Visibility: storage.Private,
	Types: []string{"application/pdf", "text/plain"}, MaxBytes: 10 << 20,
}

func put(t *testing.T, up storage.Upload, body []byte) int {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), up.Method, up.URL, bytes.NewReader(body))
	require.NoError(t, err)
	for k, v := range up.Headers {
		if k != "Content-Length" {
			req.Header.Set(k, v)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	return resp.StatusCode
}

type abandonedLog struct{ files []uuid.UUID }

func (a *abandonedLog) UploadAbandoned(_ context.Context, id uuid.UUID) error {
	a.files = append(a.files, id)
	return nil
}

type garage struct {
	svc    *storage.Service
	driver storage.Driver
}

func newGarage(t *testing.T, f *fixture) garage {
	t.Helper()
	h := s3test.New(t)
	h.Client(t)
	conn, err := s3conn.Open(t.Context(), s3conn.Config{
		Endpoint: h.Endpoint, Region: h.Region, Bucket: h.Bucket, AccessKeyID: h.AccessKeyID,
		SecretAccessKey: h.SecretAccessKey, UsePathStyle: true, Timeout: 5 * time.Second,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	d := storages3.New(conn)
	return garage{
		svc: storage.NewService(d, f.store, db.NewTransactor(f.pool), storage.Config{App: h.Prefix + "oagent", UploadURLTTL: time.Minute, PendingTTL: time.Hour},
			apperror.NewReporter(nil, false).Unexpected),
		driver: d,
	}
}

func TestService_UploadVerifyReadyDeleteSweep_OnGarageAndPostgres(t *testing.T) {
	f := newFixture(t)
	s := f.seed(t)
	g := newGarage(t, f)
	owner := storage.Owner{OrgID: s.org, ID: s.project, PublicID: "prj1"}
	pdf := append([]byte("%PDF-1.7\n"), bytes.Repeat([]byte{0xab}, 91)...)

	begin := func() (*storage.File, storage.Upload) {
		file, up, err := g.svc.BeginUpload(s.ctx, storage.BeginUploadInput{Purpose: knowledge, Owner: owner, ContentType: "application/pdf", Size: int64(len(pdf))})
		require.NoError(t, err)
		return file, up
	}

	first, up := begin()
	assert.Equal(t, http.StatusOK, put(t, up, pdf))
	got, err := g.svc.Verify(t.Context(), first.ID, knowledge, owner)
	require.NoError(t, err)
	require.NoError(t, db.RunInTx(s.ctx, f.pool, func(ctx context.Context) error { return g.svc.MarkReady(ctx, got.ID) }))

	second, up := begin()
	docx := append([]byte("PK\x03\x04"), bytes.Repeat([]byte{1}, 96)...)
	assert.Equal(t, http.StatusOK, put(t, up, docx), "storage only checks the signed headers")
	_, err = g.svc.Verify(t.Context(), second.ID, knowledge, owner)
	var mm *storage.UploadMismatchError
	require.ErrorAs(t, err, &mm)
	assert.Equal(t, storage.MismatchContent, mm.Reason)

	require.NoError(t, db.RunInTx(s.ctx, f.pool, func(ctx context.Context) error { return g.svc.MarkDeleting(ctx, first.ID) }))

	ab := &abandonedLog{}
	n, err := g.svc.Sweep(s.ctx, ab)
	require.NoError(t, err)
	assert.Equal(t, 2, n, "the deleted document's file and the rejected upload")
	assert.Empty(t, ab.files, "neither was abandoned")
	for _, key := range []string{first.ObjectKey, second.ObjectKey} {
		_, err = g.driver.Head(t.Context(), key)
		assert.True(t, storage.IsObjectNotFoundError(err), key)
	}
}

func TestService_AnUploadNeverMarkedReadyIsSweptAsAbandoned(t *testing.T) {
	f := newFixture(t)
	s := f.seed(t)
	g := newGarage(t, f)
	owner := storage.Owner{OrgID: s.org, ID: s.project, PublicID: "prj1"}
	body := []byte("plain text")

	file, up, err := g.svc.BeginUpload(s.ctx, storage.BeginUploadInput{Purpose: knowledge, Owner: owner, ContentType: "text/plain", Size: int64(len(body))})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, put(t, up, body))
	_, err = g.svc.Verify(t.Context(), file.ID, knowledge, owner)
	require.NoError(t, err, "verified, but no caller marks it ready")

	f.exec(t, `UPDATE files SET expires_at = now() - interval '1 second' WHERE id = $1`, file.ID)
	ab := &abandonedLog{}
	n, err := g.svc.Sweep(s.ctx, ab)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Equal(t, []uuid.UUID{file.ID}, ab.files)
	_, err = g.driver.Head(t.Context(), file.ObjectKey)
	assert.True(t, storage.IsObjectNotFoundError(err), "a verified file still needs MarkReady, or the sweep deletes it")
}

type completingStore struct {
	storage.Store
	between func()
}

func (c completingStore) Sweepable(ctx context.Context, now time.Time, limit int) ([]*storage.File, error) {
	files, err := c.Store.Sweepable(ctx, now, limit)
	c.between()
	return files, err
}

// SECURITY: an upload completed after the sweep listed it is never deleted.
func TestSweep_RacesCompleteUpload(t *testing.T) {
	f := newFixture(t)
	s := f.seed(t)
	g := newGarage(t, f)
	owner := storage.Owner{OrgID: s.org, ID: s.project, PublicID: "prj1"}
	body := []byte("plain text")
	file, up, err := g.svc.BeginUpload(s.ctx, storage.BeginUploadInput{Purpose: knowledge, Owner: owner, ContentType: "text/plain", Size: int64(len(body))})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, put(t, up, body))
	f.exec(t, `UPDATE files SET expires_at = now() + interval '2 seconds' WHERE id = $1`, file.ID)

	racing := storage.NewService(g.driver, completingStore{Store: f.store, between: func() {
		require.NoError(t, db.RunInTx(s.ctx, f.pool, func(ctx context.Context) error { return g.svc.MarkReady(ctx, file.ID) }))
	}}, db.NewTransactor(f.pool), storage.Config{App: "oagent", UploadURLTTL: time.Minute, PendingTTL: time.Hour}, apperror.NewReporter(nil, false).Unexpected).
		WithClock(func() time.Time { return time.Now().Add(time.Minute) })
	n, err := racing.Sweep(s.ctx, nil)
	require.NoError(t, err)
	assert.Zero(t, n, "the file was completed before the sweep claimed it")
	_, err = g.driver.Head(t.Context(), file.ObjectKey)
	require.NoError(t, err, "the object survives")
	got, err := f.store.ByID(s.ctx, file.ID)
	require.NoError(t, err)
	assert.Equal(t, storage.StatusReady, got.Status, "and so does the row")
}

func TestMarkReady_RefusesAnExpiredUpload(t *testing.T) {
	f := newFixture(t)
	s := f.seed(t)
	g := newGarage(t, f)
	file, _, err := g.svc.BeginUpload(s.ctx, storage.BeginUploadInput{Purpose: knowledge, Owner: storage.Owner{OrgID: s.org, ID: s.project, PublicID: "p"}, ContentType: "text/plain", Size: 3})
	require.NoError(t, err)
	f.exec(t, `UPDATE files SET expires_at = now() - interval '1 second' WHERE id = $1`, file.ID)
	err = db.RunInTx(s.ctx, f.pool, func(ctx context.Context) error { return g.svc.MarkReady(ctx, file.ID) })
	assert.True(t, storage.IsUploadNotFoundError(err), "the sweep owns an expired upload: %v", err)
}
