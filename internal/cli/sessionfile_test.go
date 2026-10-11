package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionFile_SaveIsPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "session.json")
	f := &sessionFile{Profiles: map[string]profile{"dev": {URL: "http://a", Session: "s", ExpiresAt: time.Now().UTC()}}}
	require.NoError(t, saveSessionFile(path, f))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	dir, err := os.Stat(filepath.Dir(path))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), dir.Mode().Perm())
	_, err = os.Stat(path + ".tmp")
	assert.True(t, os.IsNotExist(err), "the temp file is renamed away")
}

func TestSessionFile_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	in := &sessionFile{Profiles: map[string]profile{"dev": {
		URL: "http://a", Session: "s", ExpiresAt: time.Now().UTC().Truncate(time.Second),
		Principal: principalView{Source: "local", Email: "ada@example.com", ActiveOrgSlug: "acme", Permissions: []string{"*"}},
	}}}
	require.NoError(t, saveSessionFile(path, in))

	out, err := loadSessionFile(path)
	require.NoError(t, err)
	assert.Equal(t, sessionFileVersion, out.Version)
	assert.Equal(t, in.Profiles, out.Profiles)
}

func TestSessionFile_MissingIsEmpty(t *testing.T) {
	f, err := loadSessionFile(filepath.Join(t.TempDir(), "nope.json"))
	require.NoError(t, err)
	assert.Empty(t, f.Profiles)
}

func TestSessionFile_CorruptNamesThePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	require.NoError(t, os.WriteFile(path, []byte("not json"), 0o600))

	_, err := loadSessionFile(path)
	require.True(t, IsSessionFileError(err), "%v", err)
	assert.Contains(t, err.Error(), path)
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".oagent", "session.json"), expandHome("~/.oagent/session.json"))
	assert.Equal(t, "/abs/session.json", expandHome("/abs/session.json"))
}
