package migrator

import (
	"io"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type openOnlyFS struct{ fs.FS }

func TestTemplatedFS_Open_RendersSQL(t *testing.T) {
	base := fstest.MapFS{
		"a.sql":   &fstest.MapFile{Data: []byte("SELECT {{.Schema}}")},
		"VERSION": &fstest.MapFile{Data: []byte("1\n")},
	}
	tfs := newTemplatedFS(base, templateVars{Schema: "auth"})

	f, err := tfs.Open("a.sql")
	require.NoError(t, err)
	got, err := io.ReadAll(f)
	require.NoError(t, err)
	assert.Equal(t, "SELECT auth", string(got))
	assert.NoError(t, f.Close())

	info, err := f.Stat()
	require.NoError(t, err)
	assert.Equal(t, "a.sql", info.Name())
	assert.Equal(t, int64(len("SELECT auth")), info.Size())
	assert.Equal(t, fs.FileMode(0o444), info.Mode())
	assert.False(t, info.IsDir())
	assert.Nil(t, info.Sys())
	assert.NotPanics(t, func() { _ = info.ModTime() })
}

func TestTemplatedFS_Open_NonSQL(t *testing.T) {
	base := fstest.MapFS{"VERSION": &fstest.MapFile{Data: []byte("1\n")}}
	tfs := newTemplatedFS(base, templateVars{})

	f, err := tfs.Open("VERSION")
	require.NoError(t, err)
	got, err := io.ReadAll(f)
	require.NoError(t, err)
	assert.Equal(t, "1\n", string(got))
}

func TestTemplatedFS_ReadFile(t *testing.T) {
	base := fstest.MapFS{
		"a.sql":   &fstest.MapFile{Data: []byte("SELECT {{.Schema}}")},
		"VERSION": &fstest.MapFile{Data: []byte("1\n")},
	}
	tfs := newTemplatedFS(base, templateVars{Schema: "auth"})

	got, err := tfs.(fs.ReadFileFS).ReadFile("a.sql")
	require.NoError(t, err)
	assert.Equal(t, "SELECT auth", string(got))

	raw, err := tfs.(fs.ReadFileFS).ReadFile("VERSION")
	require.NoError(t, err)
	assert.Equal(t, "1\n", string(raw))
}

func TestTemplatedFS_ReadDir(t *testing.T) {
	base := fstest.MapFS{"a.sql": &fstest.MapFile{Data: []byte("SELECT 1")}}
	tfs := newTemplatedFS(base, templateVars{})

	entries, err := tfs.(fs.ReadDirFS).ReadDir(".")
	require.NoError(t, err)
	assert.Len(t, entries, 1)

	fallback := newTemplatedFS(openOnlyFS{base}, templateVars{})
	entries, err = fallback.(fs.ReadDirFS).ReadDir(".")
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

func TestRenderTemplate_ParseError(t *testing.T) {
	_, err := renderTemplate("bad.sql", []byte("{{.Schema"), templateVars{})
	assert.Error(t, err)
}
