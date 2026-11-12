package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/cli"
	"altalune.id/oagent/internal/knowledge/extract"
)

type extractResult struct {
	Sections []extract.Section `json:"sections"`
	Error    *struct {
		Kind   string `json:"kind"`
		Reason string `json:"reason"`
	} `json:"error"`
}

func runExtract(t *testing.T, stdin []byte, args ...string) (extractResult, string) {
	t.Helper()
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)
	root := cli.NewRootCmd(nil, nil)
	root.SetIn(bytes.NewReader(stdin))
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs(append([]string{"extract"}, args...))
	require.NoError(t, root.ExecuteContext(t.Context()), errOut.String())
	dec := json.NewDecoder(out)
	dec.DisallowUnknownFields()
	var res extractResult
	require.NoError(t, dec.Decode(&res), out.String())
	return res, errOut.String()
}

func TestExtractCmd_ServesStdinWithoutConfig(t *testing.T) {
	cases := map[string]func(t *testing.T){
		"invalid config": func(t *testing.T) { t.Setenv("OAGENT_MODE", "not-a-mode") },
		"no OAGENT_ variable": func(t *testing.T) {
			for _, kv := range os.Environ() {
				if k, _, _ := strings.Cut(kv, "="); strings.HasPrefix(k, "OAGENT_") {
					t.Setenv(k, "")
					require.NoError(t, os.Unsetenv(k))
				}
			}
		},
	}
	for name, env := range cases {
		t.Run(name, func(t *testing.T) {
			env(t)
			res, stderr := runExtract(t, []byte("# Refunds\n\nWithin 7 days.\n"), "markdown")
			assert.Nil(t, res.Error)
			assert.Equal(t, []extract.Section{{Heading: "Refunds", Text: "Within 7 days."}}, res.Sections)
			assert.Empty(t, stderr)
		})
	}
}

func TestExtractCmd_OnlyTheTopLevelCommandSkipsConfig(t *testing.T) {
	t.Setenv("OAGENT_MODE", "not-a-mode")
	ran := false
	parent := &cobra.Command{Use: "probe"}
	parent.AddCommand(&cobra.Command{Use: "extract", RunE: func(*cobra.Command, []string) error {
		ran = true
		return nil
	}})
	root := cli.NewRootCmd(nil, nil)
	root.AddCommand(parent)
	root.SetOut(new(bytes.Buffer))
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"probe", "extract"})
	require.Error(t, root.ExecuteContext(t.Context()), "a nested command named extract still loads config")
	assert.False(t, ran)
}

func TestExtractCmd_ReadsPDFsWithTheLibrary(t *testing.T) {
	pdf, err := os.ReadFile(filepath.Join("..", "knowledge", "testdata", "docs", "text_3pages.pdf"))
	require.NoError(t, err)
	res, _ := runExtract(t, pdf, "pdf")
	require.Nil(t, res.Error)
	require.Len(t, res.Sections, 3)
	assert.Equal(t, 1, res.Sections[0].Page)
	assert.Contains(t, res.Sections[0].Text, "Refund policy")
}

func TestExtractCmd_ReportsErrorsAsJSON(t *testing.T) {
	res, _ := runExtract(t, []byte("anything"), "spreadsheet")
	require.NotNil(t, res.Error)
	assert.Equal(t, "rejected", res.Error.Kind)
	assert.Equal(t, string(extract.UnsupportedType), res.Error.Reason)
}

func TestExtractCmd_IsHiddenAndTakesOneKind(t *testing.T) {
	buf, err := runRoot(t, nil, "--help")
	require.NoError(t, err)
	assert.NotContains(t, buf.String(), "extract")

	root := cli.NewRootCmd(nil, nil)
	root.SetOut(new(bytes.Buffer))
	root.SetErr(new(bytes.Buffer))
	root.SetIn(strings.NewReader(""))
	root.SetArgs([]string{"extract"})
	assert.Error(t, root.ExecuteContext(t.Context()))
}
