package apperror

// Code is a stable error code; the catalog lives in codes.json.
type Code string

// Kind is the class of failure a Code reports; transports map it to their own status.
type Kind int

// The failure kinds.
const (
	KindUnknown Kind = iota
	KindInvalid
	KindNotFound
	KindAlreadyExists
	KindUnauthenticated
	KindForbidden
	KindFailedPrecondition
	KindInternal
	KindResourceExhausted
)

// KindOf returns the Kind of code; unknown codes are internal.
func KindOf(code Code) Kind {
	if k, ok := kinds[code]; ok {
		return k
	}
	return KindInternal
}
