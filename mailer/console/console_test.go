package console

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/mailer"
)

func TestSend(t *testing.T) {
	buf := &bytes.Buffer{}
	err := New("", buf).Send(t.Context(), mailer.Message{To: "a@b", From: "f@x", Subject: "hi", TextBody: "hello", HTMLBody: "<b>rich</b>"})
	require.NoError(t, err)
	out := buf.String()
	for _, want := range []string{"From: f@x", "To: a@b", "Subject: hi", "hello", "<b>rich</b>"} {
		assert.Contains(t, out, want)
	}
}

func TestSend_RejectsInjection(t *testing.T) {
	buf := &bytes.Buffer{}
	err := New("", buf).Send(t.Context(), mailer.Message{To: "t@x", From: "f@x", Subject: "hi\r\nBcc: e@v", TextBody: "b"})
	assert.True(t, mailer.IsHeaderInjectionError(err), "got %T: %v", err, err)
	assert.Empty(t, buf.String())
}

func TestSend_DefaultsFromWhenMissing(t *testing.T) {
	buf := &bytes.Buffer{}
	require.NoError(t, New("default@x", buf).Send(t.Context(), mailer.Message{To: "a@b", Subject: "s", TextBody: "b"}))
	assert.Contains(t, buf.String(), "From: default@x")
}

func TestNew_NilWriterUsesStdout(t *testing.T) {
	assert.Same(t, os.Stdout, New("", nil).out)
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestSend_WriteError(t *testing.T) {
	err := New("", failingWriter{}).Send(t.Context(), mailer.Message{To: "a@b", Subject: "s", TextBody: "b"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mailer: console")
}
