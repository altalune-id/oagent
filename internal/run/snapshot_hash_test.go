package run_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/internal/run"
)

func TestSnapshotHash_PinsVersionSDKAndBlob(t *testing.T) {
	version := uuid.New()
	base := run.Snapshot{ThreadID: uuid.New(), AgentVersionID: version, SDKVersion: "1.23.0", Blob: []byte("state")}
	same := base
	same.ThreadID, same.UpdatedAt = uuid.New(), same.UpdatedAt.AddDate(1, 0, 0)
	assert.Equal(t, run.SnapshotHash(&base), run.SnapshotHash(&same), "only the version, SDK and blob count")
	assert.Len(t, run.SnapshotHash(&base), 32)

	for name, edit := range map[string]func(*run.Snapshot){
		"another agent version, same blob": func(s *run.Snapshot) { s.AgentVersionID = uuid.New() },
		"another SDK":                      func(s *run.Snapshot) { s.SDKVersion = "1.24.0" },
		"another blob":                     func(s *run.Snapshot) { s.Blob = []byte("stat3") },
	} {
		s := base
		edit(&s)
		assert.NotEqual(t, run.SnapshotHash(&base), run.SnapshotHash(&s), name)
	}
	a := run.Snapshot{AgentVersionID: version, SDKVersion: "ab", Blob: []byte("c")}
	b := run.Snapshot{AgentVersionID: version, SDKVersion: "a", Blob: []byte("bc")}
	assert.NotEqual(t, run.SnapshotHash(&a), run.SnapshotHash(&b), "length prefixes keep the fields apart")
}
