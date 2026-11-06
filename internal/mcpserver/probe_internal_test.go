package mcpserver

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

func TestRedact(t *testing.T) {
	err := redact(errors.New(`calling "initialize": 401 bad token Bearer tok-secret, tok-secret again`), "tok-secret")
	assert.NotContains(t, err.Error(), "tok-secret")
	assert.Equal(t, `calling "initialize": 401 bad token Bearer [redacted], [redacted] again`, err.Error())
	plain := errors.New("dial tcp: refused")
	assert.Same(t, plain, redact(plain, ""), "no credential, nothing to redact")
}

func TestSanitizeDescription(t *testing.T) {
	assert.Equal(t, "Line one\nline twotab[31m", sanitizeDescription("Line one\nline two\ttab\x1b[31m\x00"), "newline stays; tab, ESC and NUL go")
	assert.Equal(t, "ab\ncd", sanitizeDescription("a\x00b\r\nc\x7fd"))
	long := strings.Repeat("é", maxDescriptionRunes+10)
	got := sanitizeDescription(long)
	assert.Equal(t, maxDescriptionRunes, utf8.RuneCountInString(got))
	assert.True(t, utf8.ValidString(got))
	assert.Equal(t, "ok", sanitizeDescription("ok"))
}

func TestSanitizeDescription_FormatCharacters(t *testing.T) {
	for name, c := range map[string]struct{ in, want string }{
		"bidi override U+202E": {"safe\u202Eevil", "safeevil"},
		"zero width U+200B":    {"a\u200Bb", "ab"},
		"tag char U+E0041":     {"x\U000E0041y", "xy"},
		"C1 U+0085":            {"p\u0085q", "pq"},
		"private use U+E000":   {"m\uE000n", "mn"},
	} {
		assert.Equal(t, c.want, sanitizeDescription(c.in), name)
	}
}

func TestSanitizeSchema(t *testing.T) {
	in := []byte(`{"type":"object","title":"T\u200Bx","description":"top\u202E","properties":{"q":{"type":"string","description":"inner\u0000\u202Eok","title":"q\u200B"}},"examples":["keep\u202E"]}`)
	got := string(sanitizeSchema(in))
	assert.JSONEq(t, `{"type":"object","title":"Tx","description":"top","properties":{"q":{"type":"string","description":"innerok","title":"q"}},"examples":["keep\u202E"]}`, got,
		"description and title strings are sanitized at every depth; other strings are left alone")

	huge := []byte(`{"type":"object","properties":{"big":{"type":"string","enum":["` + strings.Repeat("e", maxSchemaBytes) + `"]}}}`)
	assert.JSONEq(t, `{"type":"object"}`, string(sanitizeSchema(huge)), "an oversized schema becomes an empty object schema")
	assert.JSONEq(t, `{"type":"object"}`, string(sanitizeSchema([]byte(`not json`))))
}
