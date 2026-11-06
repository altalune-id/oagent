package fakes_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/run/snapshotstoretest"
	"altalune.id/oagent/internal/testutil/fakes"
)

func TestSnapshotStore_Contract(t *testing.T) {
	snapshotstoretest.Run(t, func(*testing.T) snapshotstoretest.Harness {
		return snapshotstoretest.Harness{
			Store: &fakes.SnapshotStore{},
			Thread: func(*testing.T) (context.Context, run.Snapshot) {
				org := uuid.New()
				ctx := tenant.Into(context.Background(), tenant.Context{OrgID: org})
				return ctx, run.Snapshot{ThreadID: uuid.New(), OrgID: org, ProjectID: uuid.New(), AgentVersionID: uuid.New()}
			},
		}
	})
}
