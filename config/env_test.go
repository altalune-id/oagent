package config

import (
	"reflect"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestEnvVar(t *testing.T) {
	assert.Equal(t, "OAGENT_POSTGRES_TABLE_PREFIX", EnvVar("postgres.tablePrefix"))
	assert.Equal(t, "OAGENT_POSTGRES_MAX_IDLE_CONNS", EnvVar("postgres.maxIdleConns"))
	assert.Equal(t, "OAGENT_HTTP_BASE_URL", EnvVar("http.baseURL"))
}

func TestSegmentFor(t *testing.T) {
	typ := reflect.TypeOf(struct {
		NoTag string
		Skip  string `mapstructure:"-"`
		Named string `mapstructure:"named,omitempty"`
	}{})

	name, ok := segmentFor(typ.Field(0))
	assert.True(t, ok)
	assert.Equal(t, "NoTag", name)

	_, ok = segmentFor(typ.Field(1))
	assert.False(t, ok)

	name, ok = segmentFor(typ.Field(2))
	assert.True(t, ok)
	assert.Equal(t, "named", name)
}

type envSub struct {
	Value string `mapstructure:"value"`
}

type envSample struct {
	Sub  *envSub   `mapstructure:"sub"`
	When time.Time `mapstructure:"when"`
}

func TestBindEnv_PointerAndTimeLeaf(t *testing.T) {
	v := viper.NewWithOptions(viper.KeyDelimiter("."))
	bindEnv(v, "", reflect.TypeFor[envSample]())

	assert.Contains(t, v.AllKeys(), "sub.value")
	assert.Contains(t, v.AllKeys(), "when")
}
