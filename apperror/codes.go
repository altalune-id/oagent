package apperror

// Code is a stable error code.
type Code string

// Error codes: <DOM><NNN>, append-only; 900-999 is reserved for internal failures.
const (
	CodeTenantMissing Code = "GEN001"
	CodeValidation    Code = "GEN004"
	CodeUnexpected    Code = "GEN900"
)
