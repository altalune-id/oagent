package config

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWalkEnvKeys(t *testing.T) {
	byYAML := map[string]EnvKey{}
	for _, k := range WalkEnvKeys(EnvPrefix) {
		byYAML[k.YAML] = k
	}

	dsn, ok := byYAML["postgres.dsn"]
	require.True(t, ok)
	assert.Equal(t, "OAGENT_POSTGRES_DSN", dsn.Key)
	assert.Contains(t, dsn.Awareness, "required")
	assert.Contains(t, dsn.Awareness, "secret")

	addr, ok := byYAML["http.addr"]
	require.True(t, ok)
	assert.Equal(t, "OAGENT_HTTP_ADDR", addr.Key)
	assert.Equal(t, ":5150", addr.FormatDefault())

	assert.Contains(t, byYAML["mode"].Awareness, "bootstrap")
}

func TestFormatDefault(t *testing.T) {
	assert.Equal(t, "", EnvKey{}.FormatDefault())
	assert.Equal(t, "x", EnvKey{Default: "x"}.FormatDefault())
	assert.Equal(t, "true", EnvKey{Default: true}.FormatDefault())
	assert.Equal(t, "7", EnvKey{Default: 7}.FormatDefault())
	assert.Equal(t, "8", EnvKey{Default: int64(8)}.FormatDefault())
	assert.Equal(t, "30s", EnvKey{Default: 30 * time.Second}.FormatDefault())
	assert.Equal(t, "a,b", EnvKey{Default: []string{"a", "b"}}.FormatDefault())
	assert.Equal(t, "1,2", EnvKey{Default: []any{1, 2}}.FormatDefault())
	assert.Equal(t, "1.5", EnvKey{Default: 1.5}.FormatDefault())
}

type envWalkInner struct {
	Leaf string `mapstructure:"leaf" awareness:"secret"`
}

type envWalkSample struct {
	Time    time.Time               `mapstructure:"time"`
	Slice   []string                `mapstructure:"slice"`
	Ptr     *envWalkInner           `mapstructure:"ptr"`
	MapLeaf map[string]string       `mapstructure:"mapLeaf"`
	MapStru map[string]envWalkInner `mapstructure:"mapStru"`
	Dash    string                  `mapstructure:"-"`
	OptOut  string                  `mapstructure:"optout" awareness:"-,secret"`
}

func TestWalkForEnvKeys(t *testing.T) {
	var out []EnvKey
	walkForEnvKeys(&out, "OAGENT", "", reflect.TypeFor[envWalkSample](), []string{"parent"})

	byYAML := map[string]EnvKey{}
	for _, k := range out {
		byYAML[k.YAML] = k
	}

	assert.Contains(t, byYAML, "time")    // time.Time is a leaf
	assert.Contains(t, byYAML, "slice")   // slice leaf
	assert.Contains(t, byYAML, "mapLeaf") // map with scalar elem
	assert.NotContains(t, byYAML, "mapStru")
	assert.NotContains(t, byYAML, "Dash")

	ptr, ok := byYAML["ptr.leaf"]
	require.True(t, ok)
	assert.Contains(t, ptr.Awareness, "parent") // inherited
	assert.Contains(t, ptr.Awareness, "secret") // own

	assert.NotContains(t, byYAML["optout"].Awareness, "parent")
	assert.Contains(t, byYAML["optout"].Awareness, "secret")
}

func TestWalkForEnvKeys_NonStruct(t *testing.T) {
	var out []EnvKey
	walkForEnvKeys(&out, "OAGENT", "", reflect.TypeFor[string](), nil)
	assert.Empty(t, out)
}
