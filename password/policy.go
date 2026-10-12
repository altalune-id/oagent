package password

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// The length bounds of a new password, in characters (runes).
const (
	MinLen = 12
	MaxLen = 256
)

const (
	hintMinLen = 4
	maxUnit    = 4
)

// Reason is why a password breaks the policy, sent to clients as meta.reason.
type Reason string

// The reasons, in the order Violations reports them.
const (
	ReasonTooShort      Reason = "too_short"
	ReasonTooLong       Reason = "too_long"
	ReasonCommon        Reason = "common"
	ReasonRepeated      Reason = "repeated"
	ReasonSequential    Reason = "sequential"
	ReasonContainsEmail Reason = "contains_email"
	ReasonContainsName  Reason = "contains_name"
)

// Hints are facts about the account that a password must not contain.
type Hints struct{ Email, Name string }

// Violations lists every rule plain breaks, in a fixed order; nil means it passes.
func Violations(plain string, h Hints) []Reason {
	n := utf8.RuneCountInString(plain)
	if n > MaxLen {
		return []Reason{ReasonTooLong}
	}
	var out []Reason
	if n < MinLen {
		out = append(out, ReasonTooShort)
	}
	lower := strings.ToLower(plain)
	if _, ok := commonSet()[lower]; ok {
		out = append(out, ReasonCommon)
	}
	runes := []rune(lower)
	if repeated(runes) {
		out = append(out, ReasonRepeated)
	}
	if sequential(runes) {
		out = append(out, ReasonSequential)
	}
	if containsEmail(lower, h.Email) {
		out = append(out, ReasonContainsEmail)
	}
	if containsName(lower, h.Name) {
		out = append(out, ReasonContainsName)
	}
	return out
}

// Check returns a *PolicyError carrying the first violation, or nil.
func Check(plain string, h Hints) error {
	if v := Violations(plain, h); len(v) > 0 {
		return &PolicyError{Reason: v[0], Min: MinLen, Max: MaxLen}
	}
	return nil
}

func repeated(r []rune) bool {
	for unit := 1; unit <= maxUnit && unit < len(r); unit++ {
		if len(r)%unit != 0 {
			continue
		}
		same := true
		for i := unit; i < len(r) && same; i++ {
			same = r[i] == r[i-unit]
		}
		if same {
			return true
		}
	}
	return false
}

func sequential(r []rune) bool {
	if len(r) < 3 {
		return false
	}
	step := r[1] - r[0]
	if step != 1 && step != -1 {
		return false
	}
	for i := 2; i < len(r); i++ {
		if r[i]-r[i-1] != step {
			return false
		}
	}
	return true
}

func containsEmail(lower, email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return false
	}
	local, _, _ := strings.Cut(email, "@")
	return strings.Contains(lower, email) || containsPart(lower, local)
}

func containsName(lower, name string) bool {
	return containsPart(lower, strings.ToLower(name))
}

func containsPart(lower, hint string) bool {
	for _, part := range strings.FieldsFunc(hint, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
		if utf8.RuneCountInString(part) >= hintMinLen && strings.Contains(lower, part) {
			return true
		}
	}
	return false
}
