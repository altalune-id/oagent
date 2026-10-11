package authn

import (
	"encoding/base64"
	"net/http"
	"strings"

	"altalune.id/oagent/internal/kernel/session"
)

// Kind names the form of a Credential.
type Kind int

// The credential kinds; KindNone with present=true means something was sent but it is malformed.
const (
	KindNone Kind = iota
	KindAPIKey
	KindBasic
	KindSession
)

// Credential is what a request presents; exactly one kind per request.
type Credential struct {
	Kind     Kind
	Token    string
	User     string
	Password string
}

// FromHeader extracts the credential; Authorization beats the cookie. SECURITY: an unknown or malformed value is KindNone, present.
func FromHeader(h http.Header, keyPrefix string, cookieSecret []byte) (c Credential, present bool) {
	if v := strings.TrimSpace(h.Get("Authorization")); v != "" {
		return fromAuthorization(v, keyPrefix), true
	}
	if !session.HasCookie(h) {
		return Credential{}, false
	}
	sid, ok := session.ReadCookie(h, cookieSecret)
	if !ok {
		return Credential{}, true
	}
	return Credential{Kind: KindSession, Token: sid}, true
}

func fromAuthorization(v, keyPrefix string) Credential {
	scheme, rest, _ := strings.Cut(v, " ")
	rest = strings.TrimSpace(rest)
	switch {
	case strings.EqualFold(scheme, "Bearer"):
		if keyPrefix == "" || !strings.HasPrefix(rest, keyPrefix) {
			return Credential{}
		}
		return Credential{Kind: KindAPIKey, Token: rest}
	case strings.EqualFold(scheme, "Basic"):
		raw, err := base64.StdEncoding.DecodeString(rest)
		if err != nil {
			return Credential{}
		}
		user, pass, ok := strings.Cut(string(raw), ":")
		if !ok {
			return Credential{}
		}
		return Credential{Kind: KindBasic, User: user, Password: pass}
	}
	return Credential{}
}
