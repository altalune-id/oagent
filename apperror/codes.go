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

	CodeAgentNotFound           Code = "AGT001"
	CodeAgentAlreadyExists      Code = "AGT002"
	CodeAgentInvalidSlug        Code = "AGT003"
	CodeAgentInvalidName        Code = "AGT004"
	CodeAgentInvalidDescription Code = "AGT005"

	CodeAPIKeyNotFound      Code = "KEY001"
	CodeAPIKeyInvalidName   Code = "KEY002"
	CodeAPIKeyInvalidScope  Code = "KEY003"
	CodeAPIKeyInvalidExpiry Code = "KEY004"
	CodeAPIKeyInvalidGrant  Code = "KEY005"

	CodeUserNotFound      Code = "USR001"
	CodeUserAlreadyExists Code = "USR002"
	CodeUserInvalidEmail  Code = "USR003"
	CodeUserInvalidName   Code = "USR004"

	CodeMemberNotFound      Code = "MBR001"
	CodeMemberAlreadyExists Code = "MBR002"
	CodeMemberInvalidRole   Code = "MBR003"
)
