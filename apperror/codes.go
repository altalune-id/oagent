package apperror

// Code is a stable error code.
type Code string

// Error codes: <DOM><NNN>, append-only; 900-999 is reserved for internal failures.
const (
	CodeTenantMissing Code = "GEN001"
	CodeValidation    Code = "GEN004"
	CodeNotFound      Code = "GEN005"
	CodeAlreadyExists Code = "GEN006"
	CodeUnexpected    Code = "GEN900"

	CodeOrgNotFound      Code = "ORG001"
	CodeOrgAlreadyExists Code = "ORG002"
	CodeOrgInvalidSlug   Code = "ORG003"
	CodeOrgInvalidName   Code = "ORG004"

	CodeProjectNotFound      Code = "PRJ001"
	CodeProjectAlreadyExists Code = "PRJ002"
	CodeProjectInvalidSlug   Code = "PRJ003"
	CodeProjectInvalidName   Code = "PRJ004"
)
