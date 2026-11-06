package snapshotstore_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/run/snapshotstore"
)

func TestPostgres_NeedsATenant(t *testing.T) {
	s := snapshotstore.NewPostgres(db.Pool{}, "", "")
	ctx := context.Background()
	_, err := s.Load(ctx, uuid.New())
	assert.Error(t, err)
	assert.Error(t, s.Save(ctx, &run.Snapshot{}))
	assert.Error(t, s.Delete(ctx, uuid.New()))
}

func TestPostgres_RefusesAnotherOrgsSnapshotBeforeWriting(t *testing.T) {
	s := snapshotstore.NewPostgres(db.Pool{}, "", "")
	ctx := tenant.Into(context.Background(), tenant.Context{OrgID: uuid.New()})
	err := s.Save(ctx, &run.Snapshot{ThreadID: uuid.New(), OrgID: uuid.New()})
	assert.True(t, run.IsThreadNotFoundError(err))
}
