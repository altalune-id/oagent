package prompt_test

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/prompt"
)

func answers(given ...string) (func(string) (string, error), *[]string) {
	var asked []string
	return func(label string) (string, error) {
		asked = append(asked, label)
		if len(given) == 0 {
			return "", errors.New("no more input")
		}
		a := given[0]
		given = given[1:]
		return a, nil
	}, &asked
}

func TestConfirm_Match(t *testing.T) {
	read, asked := answers("a brand new password", "a brand new password")
	got, err := prompt.Confirm(read, "New password: ", "Repeat password: ")
	require.NoError(t, err)
	assert.Equal(t, "a brand new password", got)
	assert.Equal(t, []string{"New password: ", "Repeat password: "}, *asked)
}

func TestConfirm_Mismatch(t *testing.T) {
	read, _ := answers("a brand new password", "another password")
	_, err := prompt.Confirm(read, "New password: ", "Repeat password: ")
	require.ErrorIs(t, err, prompt.ErrMismatch)
	assert.NotContains(t, err.Error(), "a brand new password")
}

func TestLine(t *testing.T) {
	tests := []struct {
		name, in, want string
		wantErr        bool
	}{
		{name: "newline", in: "secret\n", want: "secret"},
		{name: "crlf", in: "secret\r\n", want: "secret"},
		{name: "last line without newline", in: "secret", want: "secret"},
		{name: "empty input", in: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := prompt.Line(bufio.NewReader(strings.NewReader(tt.in)))
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConfirm_ReadErrors(t *testing.T) {
	tests := []struct {
		name      string
		given     []string
		wantAsked []string
	}{
		{name: "first answer", wantAsked: []string{"New password: "}},
		{name: "second answer", given: []string{"a brand new password"}, wantAsked: []string{"New password: ", "Repeat password: "}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			read, asked := answers(tt.given...)
			got, err := prompt.Confirm(read, "New password: ", "Repeat password: ")
			require.EqualError(t, err, "no more input")
			assert.Empty(t, got)
			assert.Equal(t, tt.wantAsked, *asked)
		})
	}
}

func fileStdin(t *testing.T, content string) *os.File {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stdin")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	f, err := os.Open(path) //nolint:gosec // G304: a test temp file.
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })
	return f
}

type secretFn func(cmd *cobra.Command, in *bufio.Reader, fromStdin bool) (string, error)

func TestSecretsReadOneLineWithoutATerminal(t *testing.T) {
	fns := map[string]secretFn{
		"Secret": func(cmd *cobra.Command, in *bufio.Reader, fromStdin bool) (string, error) {
			return prompt.Secret(cmd, in, "Password: ", fromStdin)
		},
		"NewSecret": func(cmd *cobra.Command, in *bufio.Reader, fromStdin bool) (string, error) {
			return prompt.NewSecret(cmd, in, "New password: ", "Repeat password: ", fromStdin)
		},
	}
	stdins := []struct {
		name      string
		stdin     func(t *testing.T) io.Reader
		fromStdin bool
	}{
		{name: "buffer", stdin: func(*testing.T) io.Reader { return bytes.NewBufferString("s3cret pass\nnext\n") }},
		{name: "buffer from stdin", stdin: func(*testing.T) io.Reader { return bytes.NewBufferString("s3cret pass\nnext\n") }, fromStdin: true},
		{name: "regular file", stdin: func(t *testing.T) io.Reader { return fileStdin(t, "s3cret pass\nnext\n") }},
		{name: "regular file from stdin", stdin: func(t *testing.T) io.Reader { return fileStdin(t, "s3cret pass\nnext\n") }, fromStdin: true},
	}
	for fnName, fn := range fns {
		for _, tt := range stdins {
			t.Run(fnName+"/"+tt.name, func(t *testing.T) {
				stdin := tt.stdin(t)
				cmd := &cobra.Command{}
				cmd.SetIn(stdin)
				errOut := new(bytes.Buffer)
				cmd.SetErr(errOut)

				got, err := fn(cmd, bufio.NewReader(stdin), tt.fromStdin)
				require.NoError(t, err)
				assert.Equal(t, "s3cret pass", got)
				assert.Empty(t, errOut.String(), "no label is printed without a terminal")
			})
		}
	}
}

func TestSecret_EmptyInputFails(t *testing.T) {
	cmd := &cobra.Command{}
	stdin := new(bytes.Buffer)
	cmd.SetIn(stdin)
	_, err := prompt.Secret(cmd, bufio.NewReader(stdin), "Password: ", false)
	require.ErrorIs(t, err, io.EOF)
}

type fakeTerminal struct{ answers []string }

func (f *fakeTerminal) Read([]byte) (int, error) { return 0, io.EOF }

func (f *fakeTerminal) ReadPassword() ([]byte, error) {
	if len(f.answers) == 0 {
		return nil, io.ErrUnexpectedEOF
	}
	a := f.answers[0]
	f.answers = f.answers[1:]
	return []byte(a), nil
}

func terminalCmd(t *testing.T, answers ...string) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.SetIn(&fakeTerminal{answers: answers})
	stderr := new(bytes.Buffer)
	cmd.SetErr(stderr)
	return cmd, stderr
}

func TestSecret_OnATerminalReadsHidden(t *testing.T) {
	cmd, stderr := terminalCmd(t, "s3cret")
	got, err := prompt.Secret(cmd, bufio.NewReader(strings.NewReader("ignored\n")), "Password: ", false)
	require.NoError(t, err)
	assert.Equal(t, "s3cret", got)
	assert.Equal(t, "Password: \n", stderr.String())
}

func TestSecret_FromStdinSkipsTheTerminal(t *testing.T) {
	cmd, stderr := terminalCmd(t, "hidden")
	got, err := prompt.Secret(cmd, bufio.NewReader(strings.NewReader("piped\n")), "Password: ", true)
	require.NoError(t, err)
	assert.Equal(t, "piped", got)
	assert.Empty(t, stderr.String())
}

func TestNewSecret_OnATerminalAsksTwice(t *testing.T) {
	cmd, stderr := terminalCmd(t, "same", "same")
	got, err := prompt.NewSecret(cmd, bufio.NewReader(strings.NewReader("")), "New: ", "Again: ", false)
	require.NoError(t, err)
	assert.Equal(t, "same", got)
	assert.Equal(t, "New: \nAgain: \n", stderr.String())

	cmd, _ = terminalCmd(t, "one", "two")
	_, err = prompt.NewSecret(cmd, bufio.NewReader(strings.NewReader("")), "New: ", "Again: ", false)
	require.ErrorIs(t, err, prompt.ErrMismatch)

	cmd, _ = terminalCmd(t)
	_, err = prompt.NewSecret(cmd, bufio.NewReader(strings.NewReader("")), "New: ", "Again: ", false)
	assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func TestYesNo(t *testing.T) {
	ask := func(in io.Reader) (asked string, err error) {
		var errOut bytes.Buffer
		cmd := &cobra.Command{}
		cmd.SetIn(in)
		cmd.SetErr(&errOut)
		err = prompt.YesNo(cmd, "Go on? [y/N] ")
		return errOut.String(), err
	}

	for _, answer := range []string{"y\n", "yes\n", " YES \r\n", "Y"} {
		asked, err := ask(strings.NewReader(answer))
		require.NoError(t, err, "%q", answer)
		assert.Equal(t, "Go on? [y/N] ", asked)
	}
	for _, answer := range []string{"", "\n", "n\n", "no\n", "sure\n"} {
		_, err := ask(strings.NewReader(answer))
		assert.ErrorIs(t, err, prompt.ErrCancelled, "%q", answer)
	}
	_, err := ask(failingReader{})
	assert.EqualError(t, err, "read failed")
}
