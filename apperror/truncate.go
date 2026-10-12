package apperror

import "strings"

// TruncateCause cuts cause to at most limit bytes and drops any rune split by the cut, so it is safe to persist or send.
func TruncateCause(cause string, limit int) string {
	limit = max(limit, 0)
	if len(cause) > limit {
		cause = cause[:limit]
	}
	return strings.ToValidUTF8(cause, "")
}

// EchoLimit caps a caller-supplied value echoed back in meta.
const EchoLimit = 100

// Echo cuts a caller-supplied value to EchoLimit bytes before it goes into meta.
func Echo(v string) string { return TruncateCause(v, EchoLimit) }
