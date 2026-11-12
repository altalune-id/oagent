package approval

import (
	"slices"
	"strings"
	"unicode"
)

// Word is what a WhatsApp reply to an approval prompt says.
type Word int

// The words a reply can be.
const (
	WordNone Word = iota
	WordYes
	WordNo
)

// MatchWord reads a reply as yes or no only when, once normalized, it is exactly one listed word.
func MatchWord(text string) Word {
	w := strings.TrimRightFunc(strings.ToLower(strings.TrimSpace(text)), trailing)
	switch {
	case slices.Contains(yesWords(), w):
		return WordYes
	case slices.Contains(noWords(), w):
		return WordNo
	}
	return WordNone
}

func yesWords() []string {
	return []string{"ya", "iya", "yes", "y", "ok", "oke", "sip", "gas", "lanjut"}
}

func noWords() []string { return []string{"tidak", "no", "batal", "ga", "nggak"} }

// NOTE: trailing drops punctuation, emoji and their modifiers, joiners and spaces from the end of a reply.
func trailing(r rune) bool {
	return unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) ||
		unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Cf, r)
}
