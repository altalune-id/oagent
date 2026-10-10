package session

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
)

const idBytes = 32

// NewID returns a fresh session id: 32 random bytes, base64url without padding.
func NewID() (string, error) {
	b := make([]byte, idBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("session: new id: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Sign returns "value|mac" where mac is base64url HMAC-SHA256(value) keyed on secret.
func Sign(secret []byte, value string) string {
	return value + "|" + base64.RawURLEncoding.EncodeToString(mac(secret, value))
}

// Verify checks a signed cookie in constant time and returns its value.
func Verify(secret []byte, raw string) (string, error) {
	i := strings.LastIndexByte(raw, '|')
	if i <= 0 || i == len(raw)-1 {
		return "", &BadCookieError{}
	}
	value := raw[:i]
	sig, err := base64.RawURLEncoding.DecodeString(raw[i+1:])
	if err != nil || !hmac.Equal(sig, mac(secret, value)) {
		return "", &BadCookieError{}
	}
	return value, nil
}

func mac(secret []byte, value string) []byte {
	h := hmac.New(sha256.New, secret)
	h.Write([]byte(value))
	return h.Sum(nil)
}
