package storage

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
)

var (
	docPurpose = Purpose{
		Name: "knowledge", OwnerKind: OwnerProjects, Visibility: Private,
		Types: []string{"application/pdf", "text/plain"}, MaxBytes: 10 << 20,
	}
	pdfBody = append([]byte("%PDF-1.7\n"), bytes.Repeat([]byte{0xab}, 91)...)
)

type harness struct {
	svc    *Service
	store  *fakeStore
	driver *fakeDriver
	owner  Owner
	now    time.Time
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{store: newFakeStore(), driver: newFakeDriver(), now: time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)}
	h.svc = NewService(h.driver, h.store, directTx{}, Config{App: "oagent", UploadURLTTL: 15 * time.Minute, PendingTTL: time.Hour},
		apperror.NewReporter(nil, false).Unexpected)
	h.svc.now = func() time.Time { return h.now }
	h.owner = Owner{OrgID: uuid.New(), ID: uuid.New(), PublicID: "prj8h2kq4"}
	return h
}

func (h *harness) begin(t *testing.T) *File {
	t.Helper()
	f, _, err := h.svc.BeginUpload(context.Background(), BeginUploadInput{
		Purpose: docPurpose, Owner: h.owner, ContentType: "application/pdf", Size: int64(len(pdfBody)),
	})
	require.NoError(t, err)
	return f
}

func TestBegin_RecordsAPendingFileAndSignsItsUpload(t *testing.T) {
	h := newHarness(t)
	f, up, err := h.svc.BeginUpload(context.Background(), BeginUploadInput{
		Purpose: docPurpose, Owner: h.owner, ContentType: "application/pdf", Size: 100,
	})
	require.NoError(t, err)
	assert.Len(t, f.PublicID, publicIDLen)
	assert.Equal(t, "private/oagent/projects/prj8h2kq4/knowledge/"+f.PublicID+".pdf", f.ObjectKey)
	assert.Equal(t, StatusPending, f.Status)
	assert.Equal(t, h.now.Add(time.Hour), f.ExpiresAt)
	assert.Equal(t, h.owner.OrgID, f.OrgID)
	assert.Equal(t, h.owner.ID, f.ProjectID)
	assert.Equal(t, "knowledge", f.Purpose)
	assert.Equal(t, "https://s3.test/"+f.ObjectKey, up.URL)
	assert.Equal(t, UploadMethod, up.Method)
	assert.NotNil(t, h.store.get(f.ID))
}

func TestBegin_RefusesWhatThePurposeDoesNotAccept(t *testing.T) {
	h := newHarness(t)
	in := BeginUploadInput{Purpose: docPurpose, Owner: h.owner, ContentType: "image/gif", Size: 10}
	_, _, err := h.svc.BeginUpload(context.Background(), in)
	assert.True(t, IsTypeNotAllowedError(err), "%v", err)

	in.ContentType, in.Size = "application/pdf", docPurpose.MaxBytes+1
	_, _, err = h.svc.BeginUpload(context.Background(), in)
	assert.True(t, IsTooLargeError(err), "%v", err)
	assert.ErrorContains(t, err, "10485760")

	in.Size = 0
	_, _, err = h.svc.BeginUpload(context.Background(), in)
	assert.Equal(t, apperror.CodeValidation, codeOf(err))
	assert.Empty(t, h.store.files, "nothing is recorded for a refused upload")
}

func TestBegin_ReportsStoreAndDriverFailures(t *testing.T) {
	for _, tc := range []struct{ store, driver string }{{store: "insert"}, {driver: "presignPut"}} {
		h := newHarness(t)
		h.store.failOn, h.driver.failOn = tc.store, tc.driver
		_, _, err := h.svc.BeginUpload(context.Background(), BeginUploadInput{Purpose: docPurpose, Owner: h.owner, ContentType: "application/pdf", Size: 1})
		assert.Equal(t, apperror.CodeUnexpected, codeOf(err), "%+v: %v", tc, err)
	}
}

func TestVerify_AcceptsTheDeclaredFile(t *testing.T) {
	h := newHarness(t)
	f := h.begin(t)
	h.driver.put(f.ObjectKey, "application/pdf", pdfBody)

	got, err := h.svc.Verify(context.Background(), f.ID, docPurpose, h.owner)
	require.NoError(t, err)
	assert.Equal(t, f.ID, got.ID)
	assert.True(t, h.driver.has(f.ObjectKey))
}

func TestVerify_AcceptsAContentTypeWithParameters(t *testing.T) {
	h := newHarness(t)
	f := h.begin(t)
	h.driver.put(f.ObjectKey, "application/pdf; foo=bar", pdfBody)
	_, err := h.svc.Verify(context.Background(), f.ID, docPurpose, h.owner)
	assert.NoError(t, err)
}

func TestVerify_IsIdempotentOnceReady(t *testing.T) {
	h := newHarness(t)
	f := h.begin(t)
	require.NoError(t, h.svc.MarkReady(context.Background(), f.ID))
	h.now = h.now.Add(2 * time.Hour)
	got, err := h.svc.Verify(context.Background(), f.ID, docPurpose, h.owner)
	require.NoError(t, err)
	assert.Equal(t, StatusReady, got.Status, "a retried completion succeeds without storage")
}

// SECURITY: an upload made for another project or purpose answers the same as an unknown one.
func TestVerify_UnknownForeignExpiredOrDeletingIsNotFound(t *testing.T) {
	h := newHarness(t)
	f := h.begin(t)
	h.driver.put(f.ObjectKey, "application/pdf", pdfBody)
	other := h.owner
	other.ID = uuid.New()
	avatar := docPurpose
	avatar.Name = "avatar"

	for name, call := range map[string]func() error{
		"unknown": func() error {
			_, err := h.svc.Verify(context.Background(), uuid.New(), docPurpose, h.owner)
			return err
		},
		"other project": func() error { _, err := h.svc.Verify(context.Background(), f.ID, docPurpose, other); return err },
		"other purpose": func() error { _, err := h.svc.Verify(context.Background(), f.ID, avatar, h.owner); return err },
	} {
		assert.True(t, IsUploadNotFoundError(call()), name)
	}

	h.now = f.ExpiresAt
	_, err := h.svc.Verify(context.Background(), f.ID, docPurpose, h.owner)
	assert.True(t, IsUploadNotFoundError(err), "expired: %v", err)

	h.now = f.CreatedAt
	require.NoError(t, h.svc.MarkDeleting(context.Background(), f.ID))
	_, err = h.svc.Verify(context.Background(), f.ID, docPurpose, h.owner)
	assert.True(t, IsUploadNotFoundError(err), "deleting: %v", err)
}

func TestVerify_MissingObjectKeepsTheUploadOpen(t *testing.T) {
	h := newHarness(t)
	f := h.begin(t)
	_, err := h.svc.Verify(context.Background(), f.ID, docPurpose, h.owner)
	mm, ok := errors.AsType[*UploadMismatchError](err)
	require.True(t, ok, "%v", err)
	assert.Equal(t, MismatchMissing, mm.Reason)
	assert.Equal(t, StatusPending, h.store.get(f.ID).Status, "the browser may still finish its PUT")
}

func TestVerify_MismatchDeletesTheObjectAndHandsTheRowToTheSweep(t *testing.T) {
	for _, tc := range []struct {
		name        string
		contentType string
		body        []byte
		want        MismatchReason
	}{
		{"larger than declared", "application/pdf", append(append([]byte{}, pdfBody...), 0), MismatchSize},
		{"empty", "application/pdf", []byte{}, MismatchSize},
		{"another type", "text/plain", pdfBody, MismatchType},
		{"content is not the type", "application/pdf", append([]byte("PK\x03\x04"), bytes.Repeat([]byte{1}, 96)...), MismatchContent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			f := h.begin(t)
			h.driver.put(f.ObjectKey, tc.contentType, tc.body)

			_, err := h.svc.Verify(context.Background(), f.ID, docPurpose, h.owner)
			mm, ok := errors.AsType[*UploadMismatchError](err)
			require.True(t, ok, "%v", err)
			assert.Equal(t, tc.want, mm.Reason)
			assert.False(t, h.driver.has(f.ObjectKey), "the object is deleted")
			assert.Equal(t, StatusDeleting, h.store.get(f.ID).Status)
		})
	}
}

func TestVerify_SmallerThanDeclaredIsAccepted(t *testing.T) {
	h := newHarness(t)
	f := h.begin(t)
	h.driver.put(f.ObjectKey, "application/pdf", pdfBody[:50])
	_, err := h.svc.Verify(context.Background(), f.ID, docPurpose, h.owner)
	assert.NoError(t, err, "the cap is an upper bound; storage enforces the signed length")
}

func TestVerify_ReportsInfrastructureFailures(t *testing.T) {
	for _, tc := range []struct{ store, driver string }{
		{store: "byID"}, {driver: "head"}, {driver: "read"}, {driver: "delete"}, {store: "markDeleting"},
	} {
		h := newHarness(t)
		f := h.begin(t)
		body := pdfBody
		if tc.driver == "delete" || tc.store == "markDeleting" {
			body = append(append([]byte{}, pdfBody...), 0)
		}
		h.driver.put(f.ObjectKey, "application/pdf", body)
		h.store.failOn, h.driver.failOn = tc.store, tc.driver
		_, err := h.svc.Verify(context.Background(), f.ID, docPurpose, h.owner)
		assert.Equal(t, apperror.CodeUnexpected, codeOf(err), "%+v: %v", tc, err)
	}
}

func TestMarkReadyAndDeleting(t *testing.T) {
	h := newHarness(t)
	f := h.begin(t)
	require.NoError(t, h.svc.MarkReady(context.Background(), f.ID))
	got := h.store.get(f.ID)
	assert.Equal(t, StatusReady, got.Status)
	assert.Equal(t, h.now, *got.CompletedAt)
	require.NoError(t, h.svc.MarkReady(context.Background(), f.ID), "marking ready twice is fine")

	require.NoError(t, h.svc.MarkDeleting(context.Background(), f.ID))
	assert.Equal(t, StatusDeleting, h.store.get(f.ID).Status)
	assert.True(t, IsUploadNotFoundError(h.svc.MarkReady(context.Background(), f.ID)), "a deleting file is never attached")
	assert.True(t, IsUploadNotFoundError(h.svc.MarkDeleting(context.Background(), uuid.New())))

	h.store.failOn = "markReady"
	assert.Equal(t, apperror.CodeUnexpected, codeOf(h.svc.MarkReady(context.Background(), f.ID)))
}

func TestOpen_ReadsAReadyFileCapped(t *testing.T) {
	h := newHarness(t)
	f := h.begin(t)
	h.driver.put(f.ObjectKey, "application/pdf", pdfBody)
	rc, err := h.svc.Open(context.Background(), f.ObjectKey, docPurpose.MaxBytes)
	require.NoError(t, err)
	defer func() { _ = rc.Close() }()
	b := make([]byte, 5)
	_, err = rc.Read(b)
	require.NoError(t, err)
	assert.Equal(t, "%PDF-", string(b))
}

func TestDisabled_EveryCallSaysNotConfigured(t *testing.T) {
	svc := NewService(nil, newFakeStore(), directTx{}, Config{}, apperror.NewReporter(nil, false).Unexpected)
	assert.False(t, svc.Enabled())
	assert.False(t, (*Service)(nil).Enabled())
	ctx := context.Background()
	_, _, err := svc.BeginUpload(ctx, BeginUploadInput{Purpose: docPurpose, ContentType: "application/pdf", Size: 1})
	assert.True(t, IsNotConfiguredError(err))
	_, err = svc.Verify(ctx, uuid.New(), docPurpose, Owner{})
	assert.True(t, IsNotConfiguredError(err))
	assert.True(t, IsNotConfiguredError(svc.MarkReady(ctx, uuid.New())))
	assert.True(t, IsNotConfiguredError(svc.MarkDeleting(ctx, uuid.New())))
	_, err = svc.Open(ctx, "k", 1)
	assert.True(t, IsNotConfiguredError(err))
	n, err := svc.Sweep(ctx, nil)
	assert.Zero(t, n)
	assert.NoError(t, err)
	assert.Empty(t, svc.Jobs(nil))
}

func TestErrors_AreExpected(t *testing.T) {
	for _, err := range []error{
		&TypeNotAllowedError{Purpose: "knowledge", ContentType: "image/gif"},
		&TooLargeError{Purpose: "knowledge", MaxBytes: 1},
		&UploadNotFoundError{PublicID: "f"},
		&UploadMismatchError{PublicID: "f", Reason: MismatchSize},
		&NotConfiguredError{},
	} {
		assert.True(t, IsExpectedError(err), "%T", err)
		assert.NotEmpty(t, err.Error())
	}
	assert.False(t, IsExpectedError(errors.New("boom")))
}

func codeOf(err error) apperror.Code {
	if app, ok := apperror.AsAppError(err); ok {
		return app.Code()
	}
	return ""
}

func TestFile_LoadsByID(t *testing.T) {
	h := newHarness(t)
	f := h.begin(t)
	got, err := h.svc.File(context.Background(), f.ID)
	require.NoError(t, err)
	assert.Equal(t, f.ObjectKey, got.ObjectKey)
	_, err = h.svc.File(context.Background(), uuid.New())
	assert.True(t, IsUploadNotFoundError(err))
	_, err = NewService(nil, newFakeStore(), directTx{}, Config{}, nil).File(context.Background(), f.ID)
	assert.True(t, IsNotConfiguredError(err))
}
