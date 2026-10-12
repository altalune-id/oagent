// Package password hashes and verifies passwords with Argon2id in PHC string format.
package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/crypto/argon2"
)

// NOTE: OWASP baseline for Argon2id; the params are encoded into every hash so they can be retuned without a migration.
const (
	defaultMemoryKiB uint32 = 19456
	defaultTime      uint32 = 2
	defaultThreads   uint8  = 1
	saltLen                 = 16
	keyLen           uint32 = 32
)

// SECURITY: bounds on decoded params stop a tampered stored hash from panicking argon2 or exhausting memory.
const (
	minMemoryKiB uint32 = 8192
	maxMemoryKiB uint32 = 256 * 1024
	maxTime      uint32 = 10
	maxThreads   uint8  = 16
	minSaltLen          = 8
	minKeyLen           = 16
	maxKeyLen           = 64
)

//nolint:gochecknoglobals // computed once, read-only afterwards.
var dummyHash = sync.OnceValue(func() string {
	h, err := Hash("oagent-dummy-password")
	if err != nil {
		return ""
	}
	return h
})

// Hash returns an Argon2id PHC string for plain.
func Hash(plain string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("password: read salt: %w", err)
	}
	key := argon2.IDKey([]byte(plain), salt, defaultTime, defaultMemoryKiB, defaultThreads, keyLen)
	return encode(defaultMemoryKiB, defaultTime, defaultThreads, salt, key), nil
}

// Verify returns nil when plain matches hash, a *MismatchError when it does not, and an *InvalidHashError for a malformed hash.
func Verify(hash, plain string) error {
	h, err := decode(hash)
	if err != nil {
		return err
	}
	n := uint32(len(h.key)) //nolint:gosec // G115: len(key) is bounded by maxKeyLen.
	other := argon2.IDKey([]byte(plain), h.salt, h.time, h.memKiB, h.threads, n)
	if subtle.ConstantTimeCompare(h.key, other) != 1 {
		return &MismatchError{}
	}
	return nil
}

// VerifyDummy spends the work of Verify on a fixed hash. SECURITY: call it for unknown users so timing does not reveal which emails exist.
func VerifyDummy(plain string) {
	_ = Verify(dummyHash(), plain)
}

func encode(mem, t uint32, threads uint8, salt, key []byte) string {
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, mem, t, threads, b64.EncodeToString(salt), b64.EncodeToString(key))
}

type phc struct {
	memKiB, time uint32
	threads      uint8
	salt, key    []byte
}

func decode(hash string) (phc, error) {
	var h phc
	invalid := &InvalidHashError{}
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return phc{}, invalid
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return phc{}, invalid
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &h.memKiB, &h.time, &h.threads); err != nil {
		return phc{}, invalid
	}
	if h.threads < 1 || h.threads > maxThreads || h.time < 1 || h.time > maxTime || h.memKiB < minMemoryKiB || h.memKiB > maxMemoryKiB {
		return phc{}, invalid
	}
	b64 := base64.RawStdEncoding
	var err error
	if h.salt, err = b64.DecodeString(parts[4]); err != nil || len(h.salt) < minSaltLen {
		return phc{}, invalid
	}
	if h.key, err = b64.DecodeString(parts[5]); err != nil || len(h.key) < minKeyLen || len(h.key) > maxKeyLen {
		return phc{}, invalid
	}
	return h, nil
}
