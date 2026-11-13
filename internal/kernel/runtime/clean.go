package runtime

import (
	"strings"
	"unicode/utf8"
)

// Clean trims each string, drops blank ones, cuts each to runes runes and keeps the first n.
func Clean(in []string, runes, n int) []string {
	out := make([]string, 0, min(len(in), n))
	for _, s := range in {
		if len(out) == n {
			break
		}
		s = strings.TrimSpace(s)
		if utf8.RuneCountInString(s) > runes {
			s = strings.TrimSpace(string([]rune(s)[:runes]))
		}
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
