package approval

import (
	"encoding/json"
	"math"
	"slices"

	"altalune.id/oagent/canonjson"
)

// Decision is whether a gated call runs without asking or waits for a person.
type Decision string

// The decisions a rule makes.
const (
	Free Decision = "free"
	Ask  Decision = "ask"
)

// The modes of a tool's approval rule.
const (
	ModeNever       = "never"
	ModeAlways      = "always"
	ModeConditional = "conditional"
)

// Rule is a tool's approval rule as the runtime contract carries it.
type Rule struct {
	Mode     string      `json:"mode"`
	FreeWhen []Condition `json:"freeWhen,omitempty"`
}

// Condition holds when argument Arg equals one of Values, compared as canonical JSON, or is absent and Absent is set.
type Condition struct {
	Arg    string            `json:"arg"`
	Values []json.RawMessage `json:"values"`
	Absent bool              `json:"absent,omitempty"`
}

// Evaluate decides whether a call with args runs free; a nil schema skips the schema checks, and a broken rule asks.
func Evaluate(rule Rule, schema map[string]any, args map[string]json.RawMessage) Decision {
	switch rule.Mode {
	case ModeNever:
		return Free
	case ModeConditional:
	default:
		return Ask
	}
	if len(rule.FreeWhen) == 0 {
		return Ask
	}
	var props map[string]any
	if schema != nil {
		var ok bool
		if props, ok = schema["properties"].(map[string]any); !ok {
			return Ask
		}
	}
	for _, cond := range rule.FreeWhen {
		if !holds(cond, props, schema != nil, args) {
			return Ask
		}
	}
	return Free
}

// SECURITY: every check fails closed: an empty argument name, a value that is not JSON, a drifted schema or a missing argument asks.
func holds(cond Condition, props map[string]any, checkSchema bool, args map[string]json.RawMessage) bool {
	if cond.Arg == "" {
		return false
	}
	values := make([]string, 0, len(cond.Values))
	var declared any
	if checkSchema {
		prop, ok := props[cond.Arg]
		if !ok {
			return false
		}
		if p, isMap := prop.(map[string]any); isMap {
			declared = p["type"]
		}
	}
	for _, raw := range cond.Values {
		v, text, ok := canonical(raw)
		if !ok || checkSchema && !fitsType(v, declared) {
			return false
		}
		values = append(values, text)
	}
	raw, present := args[cond.Arg]
	if !present {
		return cond.Absent
	}
	_, got, ok := canonical(raw)
	return ok && slices.Contains(values, got)
}

func canonical(raw json.RawMessage) (value any, text string, ok bool) {
	out, err := canonjson.Of(raw)
	if err != nil {
		return nil, "", false
	}
	_ = json.Unmarshal(out, &value)
	return value, string(out), true
}

func fitsType(v, declared any) bool {
	if declared == nil {
		return true
	}
	var allowed []string
	switch d := declared.(type) {
	case string:
		allowed = []string{d}
	case []any:
		for _, x := range d {
			if s, ok := x.(string); ok {
				allowed = append(allowed, s)
			}
		}
	default:
		return false
	}
	actual := jsonType(v)
	if n, ok := v.(float64); ok && n == math.Trunc(n) && !math.IsInf(n, 0) && slices.Contains(allowed, "integer") {
		return true
	}
	return slices.Contains(allowed, actual)
}

func jsonType(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case float64:
		return "number"
	case string:
		return "string"
	case []any:
		return "array"
	default:
		return "object"
	}
}

// NeedsApproval runs the evaluator, with no live schema, for a gated tool's free_when JSON; anything that doesn't decode needs approval.
func NeedsApproval(requireApproval bool, freeWhen, args json.RawMessage) bool {
	if !requireApproval {
		return false
	}
	rule := Rule{Mode: ModeAlways}
	if len(freeWhen) > 0 {
		rule.Mode = ModeConditional
		if json.Unmarshal(freeWhen, &rule.FreeWhen) != nil {
			return true
		}
	}
	var values map[string]json.RawMessage
	if json.Unmarshal(args, &values) != nil {
		return true
	}
	return Evaluate(rule, nil, values) == Ask
}
