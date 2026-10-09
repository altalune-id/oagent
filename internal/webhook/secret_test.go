package webhook

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/sealer"
)

func testSealer(t *testing.T) sealer.Sealer {
	t.Helper()
	key, err := sealer.GenerateKey()
	require.NoError(t, err)
	sl, err := sealer.New(key)
	require.NoError(t, err)
	return sl
}

func TestNewSecret(t *testing.T) {
	s, err := newSecret()
	require.NoError(t, err)
	assert.Contains(t, s, secretPrefix)
	assert.Len(t, s, len(secretPrefix)+43) // 32 bytes raw url base64
}

func TestSealOpenSecretRoundTrip(t *testing.T) {
	sl := testSealer(t)
	id := uuid.New()
	sealed, err := sealSecret(sl, id, slotPrimary, "whsec_value")
	require.NoError(t, err)

	got, err := openSecret(sl, id, slotPrimary, sealed)
	require.NoError(t, err)
	assert.Equal(t, "whsec_value", got)
}

func TestOpenSecretWrongSlot(t *testing.T) {
	sl := testSealer(t)
	id := uuid.New()
	sealed, err := sealSecret(sl, id, slotPrimary, "whsec_value")
	require.NoError(t, err)

	_, err = openSecret(sl, id, slotSecondary, sealed)
	assert.Error(t, err)
}

func TestSign(t *testing.T) {
	got := Sign("whsec_x", "1700000000", []byte(`{"a":1}`))
	assert.Contains(t, got, "v1=")
	assert.Len(t, got, len("v1=")+64)
	// stable
	assert.Equal(t, got, Sign("whsec_x", "1700000000", []byte(`{"a":1}`)))
	assert.NotEqual(t, got, Sign("whsec_y", "1700000000", []byte(`{"a":1}`)))
}
