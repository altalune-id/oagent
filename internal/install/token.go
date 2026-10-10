package install

import (
	"crypto/sha256"
	"crypto/subtle"

	"altalune.id/oagent/nanoid"
)

const tokenLen = 32

// Token is the one-time setup secret that unlocks /install.
type Token struct{ value string }

// NewToken returns fixed when it is set, else a fresh random token.
func NewToken(fixed string) (Token, error) {
	if fixed != "" {
		return Token{value: fixed}, nil
	}
	v, err := nanoid.New(tokenLen)
	if err != nil {
		return Token{}, err
	}
	return Token{value: v}, nil
}

// Value returns the token text.
func (t Token) Value() string { return t.value }

// Matches reports whether got is the token. SECURITY: constant time over hashes, and an empty token never matches.
func (t Token) Matches(got string) bool {
	if t.value == "" || got == "" {
		return false
	}
	g, w := sha256.Sum256([]byte(got)), sha256.Sum256([]byte(t.value))
	return subtle.ConstantTimeCompare(g[:], w[:]) == 1
}
