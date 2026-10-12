package password

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/argon2"
)

func TestHash_RoundTrip(t *testing.T) {
	h, err := Hash("correct horse battery")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(h, "$argon2id$v=19$m=19456,t=2,p=1$"), h)
	require.NoError(t, Verify(h, "correct horse battery"))
}

func TestHash_UniqueSalt(t *testing.T) {
	a, err := Hash("same")
	require.NoError(t, err)
	b, err := Hash("same")
	require.NoError(t, err)
	assert.NotEqual(t, a, b)
}

func TestVerify_Mismatch(t *testing.T) {
	h, err := Hash("right")
	require.NoError(t, err)
	err = Verify(h, "wrong")
	assert.True(t, IsMismatchError(err), "got %v", err)
}

func TestVerify_AcceptsOtherParams(t *testing.T) {
	salt := []byte("0123456789abcdef")
	key := argon2.IDKey([]byte("pw"), salt, 1, 8192, 2, 32)
	b64 := base64.RawStdEncoding
	h := fmt.Sprintf("$argon2id$v=19$m=8192,t=1,p=2$%s$%s", b64.EncodeToString(salt), b64.EncodeToString(key))
	require.NoError(t, Verify(h, "pw"))
}

func TestVerify_RejectsMalformed(t *testing.T) {
	for name, h := range map[string]string{
		"empty":       "",
		"plain":       "plain",
		"argon2i":     "$argon2i$v=19$m=19456,t=2,p=1$c2FsdHNhbHRzYWx0$a2V5a2V5a2V5a2V5a2V5a2V5a2V5a2V5",
		"bad version": "$argon2id$v=1$m=19456,t=2,p=1$c2FsdHNhbHRzYWx0$a2V5a2V5a2V5a2V5a2V5a2V5a2V5a2V5",
		"bad params":  "$argon2id$v=19$m=x,t=2,p=1$c2FsdHNhbHRzYWx0$a2V5a2V5a2V5a2V5a2V5a2V5a2V5a2V5",
		"bad salt":    "$argon2id$v=19$m=19456,t=2,p=1$!!!$a2V5a2V5a2V5a2V5a2V5a2V5a2V5a2V5",
		"bad key":     "$argon2id$v=19$m=19456,t=2,p=1$c2FsdHNhbHRzYWx0$!!!",
	} {
		t.Run(name, func(t *testing.T) {
			assert.True(t, IsInvalidHashError(Verify(h, "pw")))
		})
	}
}

func TestVerify_RejectsUnsafeParams(t *testing.T) {
	salt := "c2FsdHNhbHRzYWx0"
	key := "a2V5a2V5a2V5a2V5a2V5a2V5a2V5a2V5"
	for name, params := range map[string]string{
		"zero threads": "m=19456,t=2,p=0",
		"zero time":    "m=19456,t=0,p=1",
		"huge memory":  "m=4294967295,t=2,p=1",
		"huge time":    "m=19456,t=1000,p=1",
		"tiny memory":  "m=1,t=2,p=1",
		"over memory":  "m=262145,t=2,p=1",
		"many threads": "m=19456,t=2,p=17",
	} {
		t.Run(name, func(t *testing.T) {
			h := "$argon2id$v=19$" + params + "$" + salt + "$" + key
			assert.True(t, IsInvalidHashError(Verify(h, "pw")))
		})
	}
	t.Run("short key", func(t *testing.T) {
		h := "$argon2id$v=19$m=19456,t=2,p=1$" + salt + "$a2V5"
		assert.True(t, IsInvalidHashError(Verify(h, "pw")))
	})
}

func TestVerifyDummy_DoesNotPanic(t *testing.T) {
	assert.NotPanics(t, func() { VerifyDummy("anything") })
}

func TestPolicyError_PerReason(t *testing.T) {
	for reason, text := range map[Reason]string{
		ReasonTooShort:      "must be 12 to 256 characters",
		ReasonTooLong:       "must be 12 to 256 characters",
		ReasonCommon:        "is too common",
		ReasonRepeated:      "is a repeated pattern",
		ReasonSequential:    "is a simple sequence",
		ReasonContainsEmail: "must not contain your email",
		ReasonContainsName:  "must not contain your name",
		Reason("other"):     "is not allowed",
	} {
		t.Run(string(reason), func(t *testing.T) {
			e := &PolicyError{Reason: reason, Min: MinLen, Max: MaxLen}
			assert.Equal(t, "password: "+text, e.Error())
			ae := e.ToAppError()
			assert.Equal(t, "AUT002", string(ae.Code()))
			assert.Equal(t, "Password "+text, ae.Message())
			assert.Equal(t, map[string]string{"reason": string(reason), "min": "12", "max": "256"}, ae.Meta())
			assert.True(t, IsPolicyError(ae))
		})
	}
}

func TestErrors_Messages(t *testing.T) {
	assert.Equal(t, "password: mismatch", (&MismatchError{}).Error())
	assert.Equal(t, "password: invalid hash format", (&InvalidHashError{}).Error())
}
