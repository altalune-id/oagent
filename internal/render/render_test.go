package render

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("boom") }

func TestJSON(t *testing.T) {
	var b bytes.Buffer
	require.NoError(t, JSON(&b, map[string]string{"a": "b"}))
	assert.Contains(t, b.String(), `"a": "b"`)
}

func TestNDJSON(t *testing.T) {
	var b bytes.Buffer
	require.NoError(t, NDJSON(&b, []map[string]string{{"a": "1"}, {"a": "2"}}))
	assert.Equal(t, 2, strings.Count(b.String(), "\n"))
}

func TestNDJSON_NonSlice(t *testing.T) {
	var b bytes.Buffer
	require.NoError(t, NDJSON(&b, map[string]string{"a": "1"}))
	assert.Equal(t, 1, strings.Count(b.String(), "\n"))
}

func TestTable(t *testing.T) {
	var b bytes.Buffer
	require.NoError(t, Table(&b, []string{"A", "B"}, [][]string{{"1", "2"}}))
	assert.Contains(t, b.String(), "A")
	assert.Contains(t, b.String(), "2")
}

func detectCmd(run func(cmd *cobra.Command)) *cobra.Command {
	root := &cobra.Command{Use: "x", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().String("output", "", "")
	sub := &cobra.Command{Use: "y", RunE: func(cmd *cobra.Command, _ []string) error {
		run(cmd)
		return nil
	}}
	root.AddCommand(sub)
	return root
}

func TestDetect_FlagBeatsEnv(t *testing.T) {
	t.Setenv(EnvVar, "ndjson")
	var got Format
	root := detectCmd(func(cmd *cobra.Command) { got = Detect(cmd) })
	root.SetArgs([]string{"y", "--output", "json"})

	require.NoError(t, root.Execute())
	assert.Equal(t, FormatJSON, got)
}

func TestDetect_Env(t *testing.T) {
	t.Setenv(EnvVar, "ndjson")
	var got Format
	root := detectCmd(func(cmd *cobra.Command) { got = Detect(cmd) })
	root.SetArgs([]string{"y"})

	require.NoError(t, root.Execute())
	assert.Equal(t, FormatNDJSON, got)
}

func TestError_JSON(t *testing.T) {
	var out, errBuf bytes.Buffer
	root := detectCmd(func(cmd *cobra.Command) { Error(cmd, assert.AnError) })
	root.SetOut(&out)
	root.SetErr(&errBuf)
	root.SetArgs([]string{"y", "--output", "json"})

	require.NoError(t, root.Execute())
	assert.Contains(t, errBuf.String(), `"error"`)
}

func TestDetect_Default(t *testing.T) {
	var got Format
	root := detectCmd(func(cmd *cobra.Command) { got = Detect(cmd) })
	root.SetArgs([]string{"y"})

	require.NoError(t, root.Execute())
	assert.Equal(t, FormatJSON, got) // piped stdout is not a TTY
}

func TestDetect_UnknownFlagFallsBackToText(t *testing.T) {
	var got Format
	root := detectCmd(func(cmd *cobra.Command) { got = Detect(cmd) })
	root.SetArgs([]string{"y", "--output", "bogus"})

	require.NoError(t, root.Execute())
	assert.Equal(t, FormatText, got)
}

func TestError_TextAndNDJSON(t *testing.T) {
	for _, format := range []string{"text", "ndjson"} {
		errBuf := new(bytes.Buffer)
		root := detectCmd(func(cmd *cobra.Command) { Error(cmd, assert.AnError) })
		root.SetErr(errBuf)
		root.SetArgs([]string{"y", "--output", format})

		require.NoError(t, root.Execute())
		assert.Contains(t, errBuf.String(), "error")
	}
}

func TestTable_NoHeaders(t *testing.T) {
	var b bytes.Buffer
	require.NoError(t, Table(&b, nil, [][]string{{"1", "2"}}))
	assert.Contains(t, b.String(), "1")
}

func TestWriters_Error(t *testing.T) {
	assert.Error(t, JSON(failWriter{}, map[string]string{"a": "b"}))
	assert.Error(t, NDJSON(failWriter{}, []int{1}))
	assert.Error(t, Table(failWriter{}, []string{"A"}, [][]string{{"1"}}))
}
