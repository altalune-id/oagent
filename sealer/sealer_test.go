package sealer_test

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/sealer"
)

func TestSealOpenRoundTrip(t *testing.T) {
	key, err := sealer.GenerateKey()
	require.NoError(t, err)
	s, err := sealer.New(key)
	require.NoError(t, err)

	ct, err := s.Seal([]byte("secret"), []byte("aad"))
	require.NoError(t, err)
	assert.NotEqual(t, []byte("secret"), ct)

	pt, err := s.Open(ct, []byte("aad"))
	require.NoError(t, err)
	assert.Equal(t, "secret", string(pt))
}

func TestOpenRejectsWrongAAD(t *testing.T) {
	key, err := sealer.GenerateKey()
	require.NoError(t, err)
	s, err := sealer.New(key)
	require.NoError(t, err)
	ct, err := s.Seal([]byte("secret"), []byte("aad"))
	require.NoError(t, err)

	_, err = s.Open(ct, []byte("other"))
	assert.True(t, sealer.IsOpenFailedError(err))
}

func TestOpenRejectsShortCiphertext(t *testing.T) {
	key, err := sealer.GenerateKey()
	require.NoError(t, err)
	s, err := sealer.New(key)
	require.NoError(t, err)

	_, err = s.Open([]byte("short"), nil)
	assert.True(t, sealer.IsOpenFailedError(err))
}

func TestNewRejectsWrongKeyLen(t *testing.T) {
	_, err := sealer.New(make([]byte, sealer.KeyLen-1))
	assert.True(t, sealer.IsInvalidKeyError(err))
}

func TestDisabledRefuses(t *testing.T) {
	d := sealer.Disabled()
	_, err := d.Seal(nil, nil)
	assert.True(t, sealer.IsUnavailableError(err))
	_, err = d.Open(nil, nil)
	assert.True(t, sealer.IsUnavailableError(err))
}

func TestParseKey(t *testing.T) {
	key, err := sealer.GenerateKey()
	require.NoError(t, err)

	got, err := sealer.ParseKey(hex.EncodeToString(key))
	require.NoError(t, err)
	assert.Equal(t, key, got)

	got, err = sealer.ParseKey(base64.StdEncoding.EncodeToString(key))
	require.NoError(t, err)
	assert.Equal(t, key, got)

	_, err = sealer.ParseKey("")
	assert.True(t, sealer.IsUnavailableError(err))

	_, err = sealer.ParseKey("zz-not-a-key")
	assert.Error(t, err)
}

func TestParseKeyRejectsWrongLength(t *testing.T) {
	_, err := sealer.ParseKey(hex.EncodeToString(make([]byte, 16)))
	assert.True(t, sealer.IsInvalidKeyError(err))

	_, err = sealer.ParseKey(base64.StdEncoding.EncodeToString(make([]byte, 16)))
	assert.True(t, sealer.IsInvalidKeyError(err))
}

func TestErrorPredicates(t *testing.T) {
	assert.False(t, sealer.IsUnavailableError(assertErr()))
	assert.False(t, sealer.IsInvalidKeyError(assertErr()))
	assert.False(t, sealer.IsOpenFailedError(assertErr()))
	assert.Contains(t, (&sealer.OpenFailedError{}).Error(), "authentication")
}

func assertErr() error { return errors.New("x") }

func TestErrorMessages(t *testing.T) {
	assert.Contains(t, (&sealer.UnavailableError{}).Error(), "unavailable")
	assert.Contains(t, (&sealer.InvalidKeyError{Len: 4}).Error(), "want 32")

	cause := assertErr()
	open := &sealer.OpenFailedError{Cause: cause}
	assert.Contains(t, open.Error(), "authentication")
	assert.Equal(t, cause, errors.Unwrap(open))
}
