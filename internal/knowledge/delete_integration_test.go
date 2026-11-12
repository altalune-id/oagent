//go:build integration

package knowledge_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/knowledge"
)

type fakeBindings struct {
	inUse map[uuid.UUID]bool
	used  map[uuid.UUID][]knowledge.UsedBy
}

func (f fakeBindings) SourceInUse(_ context.Context, id uuid.UUID) (bool, error) {
	return f.inUse[id], nil
}

func (f fakeBindings) SourcesUsedBy(context.Context, uuid.UUID) (map[uuid.UUID][]knowledge.UsedBy, error) {
	return f.used, nil
}

func TestDeleteSource_InUseRefused(t *testing.T) {
	e := newEnv(t)
	src := e.uploadSource(t, "Policies")
	doc := e.uploaded(t, src, faq)
	bound := fakeBindings{
		inUse: map[uuid.UUID]bool{src.ID: true},
		used:  map[uuid.UUID][]knowledge.UsedBy{src.ID: {{AgentPublicID: "a1", AgentName: "Support"}}},
	}
	svc := e.svc.WithBindings(bound)

	views, err := svc.ListSources(e.ctx, e.prj)
	require.NoError(t, err)
	assert.Equal(t, []knowledge.UsedBy{{AgentPublicID: "a1", AgentName: "Support"}}, views[0].UsedBy)

	err = svc.DeleteSource(e.ctx, src.ID)
	assert.Equal(t, apperror.CodeKnowledgeSourceInUse, codeOf(err))
	_, err = svc.GetSource(e.ctx, src.ID)
	require.NoError(t, err, "nothing deleted")

	svc.WithBindings(fakeBindings{})
	require.NoError(t, svc.DeleteSource(e.ctx, src.ID))
	_, err = svc.GetSource(e.ctx, src.ID)
	assert.True(t, knowledge.IsSourceNotFoundError(err))
	_, err = svc.GetDocument(e.ctx, doc.ID)
	assert.True(t, knowledge.IsDocumentNotFoundError(err), "its documents go with it")
	assert.Equal(t, "deleting", e.fileStatus(t, *doc.FileID), "and their files go to the sweep")

	assert.True(t, knowledge.IsSourceNotFoundError(svc.DeleteSource(e.ctx, uuid.New())))
}

func TestDeleteDocument_ObjectSwept(t *testing.T) {
	e := newEnv(t)
	svc, _ := e.ingesting(t, 768)
	src := e.uploadSource(t, "Policies")
	doc := e.uploaded(t, src, faq)
	key := e.fileKey(t, *doc.FileID)
	e.ingestRound(t, svc)
	n, _ := e.chunkRows(t, doc.ID)
	require.Positive(t, n)

	require.NoError(t, svc.DeleteDocument(e.ctx, doc.ID))
	n, _ = e.chunkRows(t, doc.ID)
	assert.Zero(t, n, "its chunks cascade")
	swept, err := e.files.Sweep(e.ctx, svc)
	require.NoError(t, err)
	assert.Equal(t, 1, swept)
	assert.False(t, e.driver.Has(key), "the sweep deletes the object")
	assert.True(t, knowledge.IsDocumentNotFoundError(svc.DeleteDocument(e.ctx, doc.ID)))
}

func TestSourceExistsAndLocks(t *testing.T) {
	e := newEnv(t)
	src := e.uploadSource(t, "Policies")
	assert.NoError(t, e.svc.SourceExists(e.ctx, e.prj, src.ID))
	assert.True(t, knowledge.IsSourceNotFoundError(e.svc.SourceExists(e.ctx, uuid.New(), src.ID)), "another project's source")
	assert.True(t, knowledge.IsSourceNotFoundError(e.svc.SourceExists(e.other, e.prj, src.ID)), "another org's source")

	require.NoError(t, db.RunInTx(e.ctx, e.pool, func(ctx context.Context) error { return e.svc.LockSourcesShared(ctx, []uuid.UUID{src.ID}) }))
	err := db.RunInTx(e.ctx, e.pool, func(ctx context.Context) error { return e.svc.LockSourcesShared(ctx, []uuid.UUID{src.ID, uuid.New()}) })
	assert.True(t, knowledge.IsSourceNotFoundError(err), "a gone source fails the lock")
}
