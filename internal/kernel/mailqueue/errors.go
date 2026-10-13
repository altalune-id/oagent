package mailqueue

import "errors"

// SealerRequiredError reports an Enqueue refused because no encryption key is configured.
type SealerRequiredError struct{}

func (*SealerRequiredError) Error() string {
	return "mailqueue: queued mail is sealed, so set secrets.encryptionKey (only development with the console driver may skip it)"
}

// IsSealerRequiredError reports whether err's tree contains a *SealerRequiredError.
func IsSealerRequiredError(err error) bool {
	_, ok := errors.AsType[*SealerRequiredError](err)
	return ok
}

// PayloadError reports a queued payload that cannot be opened; it never carries the payload.
type PayloadError struct {
	ID     string
	Reason string
}

func (e *PayloadError) Error() string { return "mailqueue: entry " + e.ID + " payload: " + e.Reason }

// IsPayloadError reports whether err's tree contains a *PayloadError.
func IsPayloadError(err error) bool {
	_, ok := errors.AsType[*PayloadError](err)
	return ok
}

// SettleRefusedError reports a settle refused because the entry was re-claimed or already settled.
type SettleRefusedError struct{ ID string }

func (e *SettleRefusedError) Error() string {
	return "mailqueue: entry " + e.ID + " is no longer held by this claim"
}

// IsSettleRefusedError reports whether err's tree contains a *SettleRefusedError.
func IsSettleRefusedError(err error) bool {
	_, ok := errors.AsType[*SettleRefusedError](err)
	return ok
}
