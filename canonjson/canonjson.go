// Package canonjson writes JSON in one canonical form: object keys sorted, no spacing, no HTML escaping.
package canonjson

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Of rewrites raw in canonical form, so equal JSON values give equal bytes however they were written.
func Of(raw []byte) (json.RawMessage, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("canonjson: %w", err)
	}
	v = zeroSign(v)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("canonjson: %w", err)
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// NOTE: -0 equals 0, so it writes as 0 (RFC 8785 Section 3.2.2.3); equal values give equal bytes.
func zeroSign(v any) any {
	switch t := v.(type) {
	case float64:
		if t == 0 {
			return float64(0)
		}
	case []any:
		for i := range t {
			t[i] = zeroSign(t[i])
		}
	case map[string]any:
		for k := range t {
			t[k] = zeroSign(t[k])
		}
	}
	return v
}
