// Package auth signs people in with a password, checks basic credentials and rebuilds a Principal's authority.
package auth

// Credentials is a local sign-in attempt.
type Credentials struct {
	Email    string
	Password string
}

// BasicCredentials is the shared basic-mode login; both fields are empty when basic mode is off.
type BasicCredentials struct {
	User     string
	Password string
}
