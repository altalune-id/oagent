package dataplane

import (
	"bufio"
	"bytes"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/run"
)

func TestReplay_MatchesTheGolden(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "sse", "replay_settled.ndjson"))
	require.NoError(t, err)
	var msgs []run.Message
	sc := bufio.NewScanner(bytes.NewReader(raw))
	for sc.Scan() {
		msgs = append(msgs, run.Message{Body: append([]byte(nil), sc.Bytes()...)})
	}
	settled := &run.Run{PublicID: "Rn5Q8wYk2HhT3mVz7XcLp", Status: run.StatusCompleted}
	rec := httptest.NewRecorder()
	w := NewSSEWriter(rec)
	for _, e := range replayEvents("client-thread-1", settled, msgs) {
		require.NoError(t, w.Event(e))
	}
	want, err := os.ReadFile(filepath.Join("testdata", "sse", "replay_settled.sse"))
	require.NoError(t, err)
	assert.Equal(t, string(want), rec.Body.String())
}

func TestReplay_EndsWithTheRunsOutcome(t *testing.T) {
	for status, want := range map[run.Status]string{
		run.StatusCompleted: `"type":"RUN_FINISHED"`, run.StatusLimitReached: `"type":"RUN_FINISHED"`,
		run.StatusCancelled: `"code":"cancelled"`, run.StatusFailed: `"code":"RUN006"`,
	} {
		r := &run.Run{PublicID: "r", Status: status, ErrorCode: "RUN006"}
		events := replayEvents("c", r, nil)
		rec := httptest.NewRecorder()
		w := NewSSEWriter(rec)
		for _, e := range events {
			require.NoError(t, w.Event(e))
		}
		assert.Contains(t, rec.Body.String(), want, status)
	}
}
