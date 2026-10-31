package tokens

import (
	"net/http"
	"net/url"

	"github.com/altalune-id/oauthrs"
)

// Resource is the identifier, mount and challenge of one protected resource. https://www.rfc-editor.org/rfc/rfc9728.html#section-3
type Resource struct {
	Identifier   string
	MetadataPath string
	MetadataURL  string
}

// NewResource derives the RFC 9728 metadata location of a resource identifier.
func NewResource(identifier string) (Resource, error) {
	metadataURL, err := oauthrs.MetadataURL(identifier)
	if err != nil {
		return Resource{}, &ResourceInvalidError{Resource: identifier, Cause: err}
	}
	u, err := url.Parse(metadataURL)
	if err != nil {
		return Resource{}, &ResourceInvalidError{Resource: identifier, Cause: err}
	}
	return Resource{Identifier: identifier, MetadataPath: u.EscapedPath(), MetadataURL: metadataURL}, nil
}

// MetadataHandler serves the RFC 9728 document naming issuer and scopes. SECURITY: unauthenticated, so it holds no tenant or caller data.
func (r Resource) MetadataHandler(issuer string, scopes []string) http.Handler {
	return oauthrs.MetadataHandler(oauthrs.ProtectedResource{
		Resource: r.Identifier, AuthorizationServers: []string{issuer}, ScopesSupported: scopes,
	})
}

// Challenge is the WWW-Authenticate value of a 401 without a credential (RFC 6750 Section 3); the zero Resource is plain Bearer.
func (r Resource) Challenge() string {
	return r.challenge(&oauthrs.MissingTokenError{})
}

// InvalidToken is the WWW-Authenticate value of a 401 for a presented credential this resource refuses (RFC 6750 Section 3.1).
func (r Resource) InvalidToken() string {
	return r.challenge(&oauthrs.InvalidTokenError{})
}

// InsufficientScope is the WWW-Authenticate value of a 403 that asks for scope on this resource (RFC 6750 Section 3.1).
func (r Resource) InsufficientScope(scope string) string {
	return r.challenge(&oauthrs.InsufficientScopeError{Scope: scope})
}

func (r Resource) challenge(err error) string {
	if r.MetadataURL == "" {
		return "Bearer"
	}
	return oauthrs.Challenge(r.MetadataURL, err)
}

// ResourceInvalidError reports a resource identifier that cannot address an RFC 9728 document.
type ResourceInvalidError struct {
	Resource string
	Cause    error
}

func (e *ResourceInvalidError) Error() string {
	return "tokens: resource identifier " + e.Resource + " is invalid: " + e.Cause.Error()
}

func (e *ResourceInvalidError) Unwrap() error { return e.Cause }
