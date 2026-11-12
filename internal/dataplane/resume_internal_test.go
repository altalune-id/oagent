package dataplane

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/approval"
)

type resumeVector struct {
	Name     string          `json:"name"`
	Open     []string        `json:"open"`
	Resume   json.RawMessage `json:"resume"`
	Want     string          `json:"want"`
	Approved bool            `json:"approved"`
	Field    string          `json:"field"`
	Missing  []string        `json:"missing"`
}

func TestResume_Vectors(t *testing.T) {
	raw, err := os.ReadFile("testdata/resume_vectors.json")
	require.NoError(t, err)
	var golden struct {
		Vectors []resumeVector `json:"vectors"`
	}
	require.NoError(t, json.Unmarshal(raw, &golden))
	require.NotEmpty(t, golden.Vectors)
	for _, v := range golden.Vectors {
		for range 20 {
			checkVector(t, v)
		}
	}
}

// NOTE: checkVector runs one vector; TestResume_Vectors repeats each, so an answer that depends on map order shows up.
func checkVector(t *testing.T, v resumeVector) {
	t.Helper()
	answer, err := resumeAnswer(v.Resume, v.Open)
	switch v.Want {
	case "ok":
		require.NoError(t, err, v.Name)
		assert.Equal(t, v.Approved, answer.Approved, v.Name)
	case "APR004":
		var incomplete *approval.ResumeIncompleteError
		require.ErrorAs(t, err, &incomplete, v.Name)
		assert.Equal(t, v.Missing, incomplete.Missing, v.Name)
	case "APR005":
		var edited *approval.EditedArgsRefusedError
		require.ErrorAs(t, err, &edited, v.Name)
	case "invalid":
		ae, ok := apperror.AsAppError(err)
		require.True(t, ok, "%s: %v", v.Name, err)
		require.NotEmpty(t, ae.Violations(), v.Name)
		require.Equal(t, v.Field, ae.Violations()[0].Field, v.Name)
	default:
		t.Fatalf("%s: unknown want %q", v.Name, v.Want)
	}
}

func TestResume_EditedArgsRefused(t *testing.T) {
	_, err := resumeAnswer(json.RawMessage(`[{"interruptId":"call_1","status":"resolved","payload":{"approved":true,"args":{"amount":1}}}]`), []string{"call_1"})
	var edited *approval.EditedArgsRefusedError
	assert.ErrorAs(t, err, &edited, "any payload key but approved and text is APR005")
}

func TestResume_IncompleteIs400(t *testing.T) {
	_, err := resumeAnswer(json.RawMessage(`[{"interruptId":"call_1","status":"cancelled"}]`), []string{"call_1", "call_2"})
	ae, ok := apperror.AsAppError(err)
	require.True(t, ok, "%v", err)
	assert.Equal(t, apperror.CodeResumeIncomplete, ae.Code())
	assert.Equal(t, 400, statusOf(ae.Code()))
	assert.Equal(t, "call_2", ae.Meta()["missing"])
}

func TestDecode_ResumeWithEmptyMessages(t *testing.T) {
	body := `{"threadId":"c-1","messages":[],"resume":[{"interruptId":"call_1","status":"cancelled"}]}`
	in, err := decodeInput(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
	require.NoError(t, err, "the widget sends messages: [] with a resume")
	assert.Equal(t, "c-1", in.ThreadID)
	assert.JSONEq(t, `[{"interruptId":"call_1","status":"cancelled"}]`, string(in.Resume))
	assert.Empty(t, in.Text, "a resume carries no new message")

	_, err = decodeInput(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"threadId":"c-1","messages":[]}`)))
	require.Error(t, err, "without a resume a message is still required")
}
