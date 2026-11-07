package channel

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	maxPartRunes = 4000
	maxParts     = 3
)

var (
	boldStars       = regexp.MustCompile(`\*\*(\S(?:.*?\S)?)\*\*`)
	boldUnderscores = regexp.MustCompile(`__(\S(?:.*?\S)?)__`)
	heading         = regexp.MustCompile(`^#{1,6}\s+(.+?)\s*#*\s*$`)
)

// Format turns a model's Markdown into WhatsApp text, split at paragraph breaks into at most 3 parts of at most 4,000 runes.
func Format(text string) []string {
	return split(whatsApp(strings.TrimSpace(text)))
}

func whatsApp(text string) string {
	lines := strings.Split(text, "\n")
	fenced := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		if m := heading.FindStringSubmatch(line); m != nil {
			line = "**" + m[1] + "**"
		}
		line = boldStars.ReplaceAllString(line, "*$1*")
		lines[i] = boldUnderscores.ReplaceAllString(line, "_${1}_")
	}
	return strings.Join(lines, "\n")
}

func split(text string) []string {
	var parts []string
	current := ""
	for para := range strings.SplitSeq(text, "\n\n") {
		for _, piece := range cut(para) {
			switch {
			case current == "":
				current = piece
			case utf8.RuneCountInString(current)+2+utf8.RuneCountInString(piece) <= maxPartRunes:
				current += "\n\n" + piece
			default:
				parts = append(parts, current)
				current = piece
			}
		}
	}
	if strings.TrimSpace(current) != "" {
		parts = append(parts, current)
	}
	if len(parts) > maxParts {
		last := []rune(strings.Join(parts[maxParts-1:], "\n\n"))
		parts = append(parts[:maxParts-1], string(last[:maxPartRunes-1])+"…")
	}
	return parts
}

func cut(para string) []string {
	runes := []rune(para)
	var out []string
	for len(runes) > maxPartRunes {
		out = append(out, string(runes[:maxPartRunes]))
		runes = runes[maxPartRunes:]
	}
	return append(out, string(runes))
}
