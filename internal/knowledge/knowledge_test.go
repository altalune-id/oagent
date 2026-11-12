package knowledge_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/knowledge"
)

var (
	pgvectorOn = []knowledge.Backend{knowledge.BackendPgvector}
	now        = time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
)

func codeOf(err error) apperror.Code {
	if ae, ok := apperror.AsAppError(err); ok {
		return ae.Code()
	}
	return ""
}

func TestNewSource_URLStartsCrawlingAndUploadIdles(t *testing.T) {
	org, prj := uuid.New(), uuid.New()
	url, err := knowledge.NewSource(org, prj, knowledge.SourceInput{
		Kind: knowledge.KindURL, Backend: knowledge.BackendPgvector, Name: "  Help center ", SeedURL: " https://help.example.com/start ",
	}, pgvectorOn, now)
	require.NoError(t, err)
	assert.Equal(t, knowledge.SourceCrawling, url.Status)
	assert.Equal(t, "https://help.example.com/start", url.SeedURL)
	assert.Equal(t, "Help center", url.Name)
	assert.Len(t, url.PublicID, 21)
	assert.Equal(t, org, url.OrgID)
	assert.Equal(t, prj, url.ProjectID)
	assert.Equal(t, now, url.CreatedAt)

	up, err := knowledge.NewSource(org, prj, knowledge.SourceInput{Kind: knowledge.KindUpload, Backend: knowledge.BackendPgvector, Name: "Policies"}, pgvectorOn, now)
	require.NoError(t, err)
	assert.Equal(t, knowledge.SourceIdle, up.Status)
	assert.Empty(t, up.SeedURL)
}

func TestCreateSource_BackendNotEnabledRefused(t *testing.T) {
	in := knowledge.SourceInput{Kind: knowledge.KindUpload, Backend: knowledge.BackendPgvector, Name: "Docs"}
	for name, enabled := range map[string][]knowledge.Backend{"none": nil, "another": {"bedrock"}} {
		_, err := knowledge.NewSource(uuid.New(), uuid.New(), in, enabled, now)
		var inv *knowledge.InvalidSourceError
		require.ErrorAs(t, err, &inv, name)
		assert.Equal(t, knowledge.SourceBackendDisabled, inv.Reason)
		assert.Equal(t, apperror.CodeKnowledgeSourceInvalid, codeOf(err))
	}
}

func TestNewSource_RefusesBadInput(t *testing.T) {
	base := knowledge.SourceInput{Kind: knowledge.KindURL, Backend: knowledge.BackendPgvector, Name: "Docs", SeedURL: "https://e.com"}
	for name, tc := range map[string]struct {
		mutate func(in *knowledge.SourceInput)
		want   knowledge.SourceReason
		code   apperror.Code
	}{
		"http seed":      {func(in *knowledge.SourceInput) { in.SeedURL = "http://e.com" }, knowledge.SourceURLHTTPSRequired, ""},
		"no seed":        {func(in *knowledge.SourceInput) { in.SeedURL = "" }, knowledge.SourceURLRequired, ""},
		"long seed":      {func(in *knowledge.SourceInput) { in.SeedURL = "https://e.com/" + strings.Repeat("a", 2048) }, knowledge.SourceURLTooLong, ""},
		"seed with user": {func(in *knowledge.SourceInput) { in.SeedURL = "https://u:p@e.com" }, knowledge.SourceURLHasCredentials, ""},
		"no name":        {func(in *knowledge.SourceInput) { in.Name = " " }, "", apperror.CodeValidation},
		"long name":      {func(in *knowledge.SourceInput) { in.Name = strings.Repeat("n", knowledge.MaxNameLen+1) }, "", apperror.CodeValidation},
		"unknown kind":   {func(in *knowledge.SourceInput) { in.Kind = "bedrock_kb" }, "", apperror.CodeValidation},
	} {
		t.Run(name, func(t *testing.T) {
			in := base
			tc.mutate(&in)
			_, err := knowledge.NewSource(uuid.New(), uuid.New(), in, pgvectorOn, now)
			require.Error(t, err)
			if tc.want != "" {
				var inv *knowledge.InvalidSourceError
				require.ErrorAs(t, err, &inv)
				assert.Equal(t, tc.want, inv.Reason)
				return
			}
			assert.Equal(t, tc.code, codeOf(err))
		})
	}
}

func TestNewUploadDocument(t *testing.T) {
	src, err := knowledge.NewSource(uuid.New(), uuid.New(), knowledge.SourceInput{Kind: knowledge.KindUpload, Backend: knowledge.BackendPgvector, Name: "D"}, pgvectorOn, now)
	require.NoError(t, err)
	file := uuid.New()
	d, err := knowledge.NewUploadDocument(src, " handbook.pdf ", file, now)
	require.NoError(t, err)
	assert.Equal(t, knowledge.DocumentUploading, d.Status)
	assert.Equal(t, "handbook.pdf", d.Title)
	assert.Equal(t, &file, d.FileID)
	assert.Equal(t, src.ID, d.SourceID)
	assert.Equal(t, src.ProjectID, d.ProjectID)

	_, err = knowledge.NewUploadDocument(src, "", file, now)
	assert.Equal(t, apperror.CodeValidation, codeOf(err))
	_, err = knowledge.NewUploadDocument(src, strings.Repeat("t", knowledge.MaxTitleLen+1), file, now)
	assert.Equal(t, apperror.CodeValidation, codeOf(err))
	src.Kind = knowledge.KindURL
	_, err = knowledge.NewUploadDocument(src, "a.pdf", file, now)
	assert.Equal(t, apperror.CodeKnowledgeSourceInvalid, codeOf(err), "a URL source takes no uploads")
}

func TestDocument_FailKeepsCodeAndReason(t *testing.T) {
	d := &knowledge.Document{Status: knowledge.DocumentUploading}
	d.Fail(&knowledge.UploadRejectedError{Reason: knowledge.RejectExpired}, now)
	assert.Equal(t, knowledge.DocumentFailed, d.Status)
	assert.Equal(t, &knowledge.Failure{Code: apperror.CodeKnowledgeUploadRejected, Reason: "expired"}, d.Failure)

	assert.Equal(t, knowledge.Failure{Code: apperror.CodeKnowledgeNoTextFound}, knowledge.FailureOf(&knowledge.NoTextFoundError{}))
	assert.Equal(t, knowledge.Failure{Code: apperror.CodeUnexpected}, knowledge.FailureOf(errors.New("boom")))
}

func TestErrors_AreExpected(t *testing.T) {
	for _, err := range []error{
		&knowledge.SourceNotFoundError{}, &knowledge.InvalidSourceError{Reason: knowledge.SourceBackendDisabled},
		&knowledge.SourceAlreadyExistsError{Name: "n"}, &knowledge.SourceInUseError{}, &knowledge.DocumentNotFoundError{},
		&knowledge.UploadRejectedError{Reason: knowledge.RejectSize}, &knowledge.NoTextFoundError{},
		&knowledge.UnreadableError{Reason: knowledge.UnreadableCrashed}, &knowledge.EmbeddingModelInvalidError{Reason: knowledge.EmbeddingSize, Dims: 1},
		&knowledge.EmbeddingModelNotSetError{}, &knowledge.EmbeddingFailedError{Reason: knowledge.EmbeddingRejected, Cause: errors.New("x")},
		&knowledge.StorageNotConfiguredError{}, &knowledge.LimitReachedError{Reason: knowledge.LimitChunksPerProject, Max: 1},
		&knowledge.CrawlFailedError{Reason: knowledge.CrawlNoPages}, apperror.Invalid("name", apperror.FieldRequired),
	} {
		assert.True(t, knowledge.IsExpectedError(err), "%T", err)
		assert.NotEmpty(t, err.Error())
	}
	assert.False(t, knowledge.IsExpectedError(errors.New("boom")))
	assert.ErrorContains(t, &knowledge.EmbeddingFailedError{Reason: knowledge.EmbeddingRejected, Cause: errors.New("http 400")}, "http 400")
	assert.Contains(t, (&knowledge.EmbeddingModelInvalidError{Reason: knowledge.EmbeddingUnpriced}).Error(), "unpriced")
}
