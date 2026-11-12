package approval_test

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/approval"
)

type vector struct {
	Name        string                     `json:"name"`
	Schema      map[string]any             `json:"schema"`
	Rule        approval.Rule              `json:"rule"`
	Args        map[string]json.RawMessage `json:"args"`
	Want        approval.Decision          `json:"want"`
	RuntimeOnly bool                       `json:"runtimeOnly"`
}

func vectors(t *testing.T) []vector {
	t.Helper()
	raw, err := os.ReadFile("testdata/rule_vectors.json")
	require.NoError(t, err)
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var out []vector
	require.NoError(t, dec.Decode(&out), "the shared vectors hold no field Go does not know")
	require.NotEmpty(t, out)
	return out
}

func TestRule_Vectors(t *testing.T) {
	for _, v := range vectors(t) {
		assert.Equal(t, v.Want, approval.Evaluate(v.Rule, v.Schema, v.Args), "with the live schema: %s", v.Name)
	}
}

// NOTE: the audit runs with a nil schema; the schema-broken vectors only apply where the live schema exists.
func TestRule_VectorsWithoutASchema(t *testing.T) {
	ran := 0
	for _, v := range vectors(t) {
		if v.RuntimeOnly {
			continue
		}
		ran++
		assert.Equal(t, v.Want, approval.Evaluate(v.Rule, nil, v.Args), "audit, nil schema: %s", v.Name)
	}
	assert.Positive(t, ran)
}

func TestRule_FailsClosedOnSchemaDrift(t *testing.T) {
	confirmFalse := approval.Rule{Mode: approval.ModeConditional, FreeWhen: []approval.Condition{
		{Arg: "confirm", Values: []json.RawMessage{json.RawMessage(`false`)}, Absent: true},
	}}
	args := map[string]json.RawMessage{"confirm": json.RawMessage(`false`)}
	for name, schema := range map[string]map[string]any{
		"no properties":           {"type": "object"},
		"properties not a map":    {"type": "object", "properties": []any{"confirm"}},
		"argument left out":       {"type": "object", "properties": map[string]any{"amount": map[string]any{"type": "number"}}},
		"argument type changed":   {"type": "object", "properties": map[string]any{"confirm": map[string]any{"type": "string"}}},
		"type list without match": {"type": "object", "properties": map[string]any{"confirm": map[string]any{"type": []any{"string", "null"}}}},
	} {
		assert.Equal(t, approval.Ask, approval.Evaluate(confirmFalse, schema, args), name)
	}
	live := map[string]any{"type": "object", "properties": map[string]any{"confirm": map[string]any{"type": "boolean"}}}
	assert.Equal(t, approval.Free, approval.Evaluate(confirmFalse, live, args))
	assert.Equal(t, approval.Free, approval.Evaluate(confirmFalse, nil, map[string]json.RawMessage{"amount": json.RawMessage(`1`)}),
		"a nil schema means not broken, so an absent argument with absent set is free")

	for name, rule := range map[string]approval.Rule{
		"unknown mode":                  {Mode: "sometimes"},
		"conditional with no condition": {Mode: approval.ModeConditional},
		"a value that is not JSON":      {Mode: approval.ModeConditional, FreeWhen: []approval.Condition{{Arg: "confirm", Values: []json.RawMessage{json.RawMessage(`fals`)}}}},
	} {
		assert.Equal(t, approval.Ask, approval.Evaluate(rule, nil, args), name)
	}
	assert.Equal(t, approval.Ask, approval.Evaluate(confirmFalse, nil, map[string]json.RawMessage{"confirm": json.RawMessage(`{`)}), "an argument that is not JSON")
}

func TestRule_EmptyArgNeverFree(t *testing.T) {
	rule := approval.Rule{Mode: approval.ModeConditional, FreeWhen: []approval.Condition{{Arg: "", Absent: true}}}
	for name, schema := range map[string]map[string]any{
		"no schema": nil,
		"a schema":  {"type": "object", "properties": map[string]any{"amount": map[string]any{"type": "number"}}},
	} {
		assert.Equal(t, approval.Ask, approval.Evaluate(rule, schema, map[string]json.RawMessage{"amount": json.RawMessage(`1`)}), name)
	}
}

func TestNeedsApproval(t *testing.T) {
	confirm := json.RawMessage(`[{"arg":"confirm","values":[false]}]`)
	for name, tc := range map[string]struct {
		gated          bool
		freeWhen, args string
		want           bool
	}{
		"free tool":              {false, "", `{}`, false},
		"gated, no conditions":   {true, "", `{}`, true},
		"gated, free_when holds": {true, string(confirm), `{"confirm":false}`, false},
		"gated, free_when fails": {true, string(confirm), `{"confirm":true}`, true},
		"args not JSON":          {true, string(confirm), `not json`, true},
		"conditions not JSON":    {true, `{`, `{}`, true},
	} {
		var fw json.RawMessage
		if tc.freeWhen != "" {
			fw = json.RawMessage(tc.freeWhen)
		}
		assert.Equal(t, tc.want, approval.NeedsApproval(tc.gated, fw, json.RawMessage(tc.args)), name)
	}
}
