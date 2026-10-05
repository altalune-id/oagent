package migrator

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTargetVersion(t *testing.T) {
	m := New(fstest.MapFS{"VERSION": &fstest.MapFile{Data: []byte("3\n")}}, Config{})

	v, err := m.TargetVersion()
	require.NoError(t, err)
	assert.Equal(t, int64(3), v)
}

func TestTargetVersion_Invalid(t *testing.T) {
	m := New(fstest.MapFS{"VERSION": &fstest.MapFile{Data: []byte("abc")}}, Config{})
	_, err := m.TargetVersion()
	assert.Error(t, err)
}

func TestTargetVersion_Missing(t *testing.T) {
	m := New(fstest.MapFS{}, Config{})
	_, err := m.TargetVersion()
	assert.Error(t, err)
}

func TestRenderTemplate(t *testing.T) {
	base := fstest.MapFS{
		"a.sql": &fstest.MapFile{Data: []byte(
			"CREATE TABLE {{.Schema}}.{{.TablePrefix}}orgs (); SET ROLE {{.Role}};",
		)},
	}

	tfs := newTemplatedFS(base, templateVars{Schema: "auth", TablePrefix: "t_", Role: "owner"})
	got, err := fs.ReadFile(tfs, "a.sql")
	require.NoError(t, err)
	assert.Equal(t, "CREATE TABLE auth.t_orgs (); SET ROLE owner;", string(got))
}

func TestRenderTemplate_MissingKey(t *testing.T) {
	base := fstest.MapFS{"a.sql": &fstest.MapFile{Data: []byte("SELECT {{.Nope}}")}}

	tfs := newTemplatedFS(base, templateVars{})
	_, err := fs.ReadFile(tfs, "a.sql")
	assert.Error(t, err)
}

func TestWithDefaults(t *testing.T) {
	assert.Equal(t, "public", Config{}.withDefaults().Schema)
	assert.Equal(t, "auth", Config{Schema: "auth"}.withDefaults().Schema)
}

func TestUpToDate(t *testing.T) {
	tests := []struct {
		name   string
		target int64
		rows   []MigrationStatus
		want   bool
	}{
		{"no migrations", 1, nil, true},
		{"all applied", 2, []MigrationStatus{{Version: 1, Applied: true}, {Version: 2, Applied: true}}, true},
		{"pending within target", 2, []MigrationStatus{{Version: 1, Applied: true}, {Version: 2, Applied: false}}, false},
		{"pending beyond target is ignored", 3, []MigrationStatus{{Version: 1, Applied: true}, {Version: 4, Applied: false}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, upToDate(tt.target, tt.rows))
		})
	}
}
