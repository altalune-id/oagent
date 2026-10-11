package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parsedRoot(t *testing.T, args ...string) *cobra.Command {
	t.Helper()
	root := &cobra.Command{Use: "x", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().String("url", "", "")
	root.PersistentFlags().String("token", "", "")
	root.PersistentFlags().String("token-file", "", "")
	leaf := &cobra.Command{Use: "y", RunE: func(*cobra.Command, []string) error { return nil }}
	root.AddCommand(leaf)
	root.SetArgs(args)
	require.NoError(t, root.Execute())
	return leaf
}

func TestResolveURL_Flag(t *testing.T) {
	assert.Equal(t, "http://flag", resolveURL(parsedRoot(t, "y", "--url", "http://flag"), nil))
}

func TestResolveURL_Env(t *testing.T) {
	t.Setenv("OAGENT_URL", "http://env")
	assert.Equal(t, "http://env", resolveURL(parsedRoot(t, "y"), nil))
}

func TestResolveURL_Default(t *testing.T) {
	t.Setenv("OAGENT_URL", "")
	assert.Equal(t, DefaultURL, resolveURL(parsedRoot(t, "y"), nil))
}

func TestResolveToken_Flag(t *testing.T) {
	tok, err := resolveToken(parsedRoot(t, "y", "--token", "tok"))
	require.NoError(t, err)
	assert.Equal(t, "tok", tok)
}

func TestResolveToken_File(t *testing.T) {
	p := filepath.Join(t.TempDir(), "tok")
	require.NoError(t, os.WriteFile(p, []byte("filetok\n"), 0o600))

	tok, err := resolveToken(parsedRoot(t, "y", "--token-file", p))
	require.NoError(t, err)
	assert.Equal(t, "filetok", tok)
}

func TestResolveToken_FileError(t *testing.T) {
	_, err := resolveToken(parsedRoot(t, "y", "--token-file", filepath.Join(t.TempDir(), "nope")))
	assert.Error(t, err)
}

func TestResolveToken_Env(t *testing.T) {
	t.Setenv("OAGENT_TOKEN", "envtok")
	tok, err := resolveToken(parsedRoot(t, "y"))
	require.NoError(t, err)
	assert.Equal(t, "envtok", tok)
}
