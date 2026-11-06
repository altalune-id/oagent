package events_test

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/events"
)

func jsonKeys(t *testing.T, v any) []string {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	keys := make([]string, 0, len(m))
	for k, child := range m {
		keys = append(keys, k)
		if nested, ok := child.(map[string]any); ok {
			for nk := range nested {
				keys = append(keys, k+"."+nk)
			}
		}
	}
	sort.Strings(keys)
	return keys
}

func TestRunPayload_NoTextNoPhone(t *testing.T) {
	want := []string{
		"agent_id", "cost_micros", "run_id", "status", "thread_id", "usage",
		"usage.cache_read_input_tokens", "usage.cache_write_input_tokens", "usage.estimated", "usage.input_tokens", "usage.output_tokens",
	}
	for _, payload := range []any{events.RunCompletedV1{}, events.RunFailedV1{}} {
		keys := jsonKeys(t, payload)
		assert.Equal(t, want, keys, reflect.TypeOf(payload).Name())
		for _, k := range keys {
			for _, banned := range []string{"text", "message", "content", "phone", "lid", "prompt", "reply"} {
				assert.NotContains(t, k, banned, reflect.TypeOf(payload).Name())
			}
		}
		for f := range reflect.TypeOf(payload).Fields() {
			if f.Type.Kind() == reflect.String {
				assert.True(t, strings.HasSuffix(f.Name, "ID") || f.Name == "Status", f.Name)
			}
		}
	}
}

func TestRunPayload_UnpricedCostIsNull(t *testing.T) {
	raw, err := json.Marshal(events.RunCompletedV1{})
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"cost_micros":null`)
	cost := int64(1250)
	raw, err = json.Marshal(events.RunFailedV1{CostMicros: &cost})
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"cost_micros":1250`)
}
