package pg

import (
	"fmt"
	"strings"
)

// MaxIdentifierBytes is the Postgres identifier length limit.
const MaxIdentifierBytes = 63

// ValidateIdentifier reports whether name is usable as a Postgres identifier.
func ValidateIdentifier(name string) error {
	switch {
	case name == "":
		return &InvalidIdentifierError{Name: name, Reason: "empty"}
	case len(name) > MaxIdentifierBytes:
		return &InvalidIdentifierError{Name: name, Reason: fmt.Sprintf("longer than %d bytes", MaxIdentifierBytes)}
	case strings.ContainsFunc(name, func(r rune) bool { return r < 0x20 || r == 0x7f }):
		return &InvalidIdentifierError{Name: name, Reason: "contains a control character"}
	}
	return nil
}

// QuoteIdentifier escapes name for interpolation into SQL.
func QuoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
