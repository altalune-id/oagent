package approval

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"altalune.id/oagent/canonjson"
)

const (
	maxPreviewLines = 12
	maxValueRunes   = 200
	wordJoiner      = "\u2060"
)

// Preview renders the calls as plain text: "<server> · <tool>", then "key: value" lines in schema order, from the stored args only.
func Preview(calls []Call) string {
	// SECURITY: every call keeps its header and first argument; the rest share the lines left round-robin, and a last line counts what is cut.
	type arg struct {
		line string
		cut  bool
	}
	headers := make([]string, len(calls))
	args := make([][]arg, len(calls))
	total := 0
	for i, c := range calls {
		headers[i] = clean(c.Server) + " · " + clean(c.Tool)
		var raw map[string]json.RawMessage
		_ = json.Unmarshal(c.Args, &raw)
		for _, k := range orderedKeys(raw, c.Keys) {
			v := valueText(raw[k])
			cut := utf8.RuneCountInString(v) > maxValueRunes
			if cut {
				v = capRunes(v, maxValueRunes) + "…"
			}
			args[i] = append(args[i], arg{line: clean(k) + ": " + clean(v), cut: cut})
		}
		total += len(args[i])
	}
	limit := max(maxPreviewLines, 2*len(calls)+1)
	budget := limit - len(calls)
	anyCut := slices.ContainsFunc(args, func(as []arg) bool { return slices.ContainsFunc(as, func(a arg) bool { return a.cut }) })
	if total > budget || anyCut {
		budget--
	}
	shown := make([]int, len(calls))
	for given := 0; given < budget; {
		progressed := false
		for i := range calls {
			if given < budget && shown[i] < len(args[i]) {
				shown[i]++
				given++
				progressed = true
			}
		}
		if !progressed {
			break
		}
	}
	lines := make([]string, 0, limit)
	more := 0
	for i, h := range headers {
		lines = append(lines, h)
		for j, a := range args[i] {
			switch {
			case j >= shown[i]:
				more++
			case a.cut:
				more++
				lines = append(lines, a.line)
			default:
				lines = append(lines, a.line)
			}
		}
	}
	if more > 0 {
		lines = append(lines, "+"+strconv.Itoa(more)+" more not shown")
	}
	return strings.Join(lines, "\n")
}

// NOTE: the runtime's order: the schema's keys present in the args, then the rest sorted.
func orderedKeys(args map[string]json.RawMessage, schemaOrder []string) []string {
	out := make([]string, 0, len(args))
	for _, k := range schemaOrder {
		if _, ok := args[k]; ok && !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	var rest []string
	for k := range args {
		if !slices.Contains(out, k) {
			rest = append(rest, k)
		}
	}
	slices.Sort(rest)
	return append(out, rest...)
}

func valueText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	if text, err := canonjson.Of(raw); err == nil {
		return string(text)
	}
	return string(raw)
}

// SECURITY: a value can't add a line or fake a decision prompt; control and line-separator characters become spaces.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) {
			return ' '
		}
		return r
	}, s)
}

// WhatsAppText escapes WhatsApp formatting in a preview for the WhatsApp send path; the stored preview stays plain text.
func WhatsAppText(preview string) string {
	// NOTE: WhatsApp formats only markers at a word's edge, so a marker between two letters or digits (record_expense) stays as it is.
	rs := []rune(preview)
	var b strings.Builder
	for i, r := range rs {
		b.WriteRune(r)
		if strings.ContainsRune("*_~`", r) && (i == 0 || !wordRune(rs[i-1]) || i+1 == len(rs) || !wordRune(rs[i+1])) {
			b.WriteString(wordJoiner)
		}
	}
	return b.String()
}

func wordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }
