package extract

import (
	"regexp"
	"strings"
)

var atxHeading = regexp.MustCompile(`^ {0,3}#{1,6}(?:[ \t]+(.*?))?[ \t]*#*[ \t]*$`)

func extractMarkdown(data []byte) ([]Section, error) {
	text, err := utf8Text(data)
	if err != nil {
		return nil, err
	}
	var b builder
	fence := ""
	for line := range strings.SplitSeq(text, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case fence != "":
			if strings.HasPrefix(trimmed, fence) {
				fence = ""
			}
			b.addText(line)
		case strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~"):
			fence = trimmed[:3]
			b.addText(line)
		default:
			if m := atxHeading.FindStringSubmatch(line); m != nil {
				b.addHeading(m[1])
				continue
			}
			b.addText(line)
		}
	}
	return b.done(), nil
}
