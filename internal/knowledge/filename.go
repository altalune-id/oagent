package knowledge

import (
	"strings"
	"unicode"
)

// HasControlCharacter reports whether name holds a Unicode control character, which a filename must not.
func HasControlCharacter(name string) bool { return strings.ContainsFunc(name, unicode.IsControl) }
