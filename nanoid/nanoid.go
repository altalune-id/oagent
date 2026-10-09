// Package nanoid produces URL-safe short public IDs.
package nanoid

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

const alphabet = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKMNPQRSTUVWXYZ23456789"
const alphabetLen = byte(len(alphabet))

var ErrInvalidLength = errors.New("nanoid: length must be > 0")

// New returns a length-n nanoid.
func New(length int) (string, error) {
	if length <= 0 {
		return "", ErrInvalidLength
	}
	mask := byte(63)
	buf := make([]byte, 0, length)
	pool := make([]byte, length*2)
	for len(buf) < length {
		if _, err := rand.Read(pool); err != nil {
			return "", err
		}
		for _, b := range pool {
			if b&mask < alphabetLen {
				buf = append(buf, alphabet[b&mask])
				if len(buf) == length {
					break
				}
			}
		}
	}
	return string(buf), nil
}

// MustNew is New that panics on an invalid length; use only with constants.
func MustNew(length int) string {
	s, err := New(length)
	if err != nil {
		panic(err)
	}
	return s
}

// NewInviteToken returns a raw token and its sha256 hex hash.
func NewInviteToken() (raw, hash string, err error) {
	raw, err = New(32)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(raw))
	return raw, hex.EncodeToString(sum[:]), nil
}

// IsValid reports whether s is a length-n nanoid.
func IsValid(s string, length int) bool {
	if length <= 0 || len(s) != length {
		return false
	}
	for i := 0; i < len(s); i++ {
		if strings.IndexByte(alphabet, s[i]) < 0 {
			return false
		}
	}
	return true
}
