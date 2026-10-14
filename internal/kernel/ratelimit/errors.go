package ratelimit

import (
	"errors"
	"fmt"
)

// InvalidRuleError reports a Rule a store cannot count.
type InvalidRuleError struct {
	Rule   string
	Reason string
}

func (e *InvalidRuleError) Error() string {
	return fmt.Sprintf("ratelimit: rule %q: %s", e.Rule, e.Reason)
}

// IsInvalidRuleError reports whether err's tree contains an *InvalidRuleError.
func IsInvalidRuleError(err error) bool {
	_, ok := errors.AsType[*InvalidRuleError](err)
	return ok
}
