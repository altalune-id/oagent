package smtp

import (
	"errors"
	"fmt"
)

// StartTLSUnsupportedError reports an SMTP server that does not offer STARTTLS when the driver requires it.
type StartTLSUnsupportedError struct{ Host string }

func (e *StartTLSUnsupportedError) Error() string {
	return fmt.Sprintf("mailer: smtp server %s does not offer STARTTLS", e.Host)
}

// IsStartTLSUnsupportedError reports whether err's tree contains a *StartTLSUnsupportedError.
func IsStartTLSUnsupportedError(err error) bool {
	_, ok := errors.AsType[*StartTLSUnsupportedError](err)
	return ok
}
