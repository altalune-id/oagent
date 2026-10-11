package apperror

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
)

//nolint:gochecknoglobals // immutable lookup table.
var kinds = map[Code]Kind{
	CodeTenantMissing:   KindUnauthenticated,
	CodeUnauthenticated: KindUnauthenticated,
	CodeValidation:      KindInvalid,
	CodeNotFound:        KindNotFound,
	CodeAlreadyExists:   KindAlreadyExists,
	CodeForbidden:       KindForbidden,
	CodeUnexpected:      KindInternal,

	CodeOrgNotFound:         KindNotFound,
	CodeOrgAlreadyExists:    KindAlreadyExists,
	CodeOrgInvalidSlug:      KindInvalid,
	CodeOrgInvalidName:      KindInvalid,
	CodeOrgCreationDisabled: KindFailedPrecondition,

	CodeProjectNotFound:      KindNotFound,
	CodeProjectAlreadyExists: KindAlreadyExists,
	CodeProjectInvalidSlug:   KindInvalid,
	CodeProjectInvalidName:   KindInvalid,

	CodeAgentNotFound:           KindNotFound,
	CodeAgentAlreadyExists:      KindAlreadyExists,
	CodeAgentInvalidSlug:        KindInvalid,
	CodeAgentInvalidName:        KindInvalid,
	CodeAgentInvalidDescription: KindInvalid,

	CodeAPIKeyNotFound:        KindNotFound,
	CodeAPIKeyInvalidName:     KindInvalid,
	CodeAPIKeyInvalidScope:    KindInvalid,
	CodeAPIKeyInvalidExpiry:   KindInvalid,
	CodeAPIKeyInvalidGrant:    KindInvalid,
	CodeAPIKeyPersonOnlyScope: KindInvalid,

	CodeUserNotFound:      KindNotFound,
	CodeUserAlreadyExists: KindAlreadyExists,
	CodeUserInvalidEmail:  KindInvalid,
	CodeUserInvalidName:   KindInvalid,

	CodeMemberNotFound:      KindNotFound,
	CodeMemberAlreadyExists: KindAlreadyExists,
	CodeMemberInvalidRole:   KindInvalid,

	CodeAuthInvalidCredentials: KindUnauthenticated,

	CodeInstallAlreadyDone: KindAlreadyExists,
}

// KindOf returns the Kind of code; unknown codes are internal.
func KindOf(code Code) Kind {
	if k, ok := kinds[code]; ok {
		return k
	}
	return KindInternal
}
