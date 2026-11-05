package modelprovider

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

func TestSealKey_RoundTrip(t *testing.T) {
	sl := testSealer(t)
	orgID, id := uuid.New(), uuid.New()
	sealed, err := sealKey(sl, orgID, id, "sk-test")
	require.NoError(t, err)
	assert.NotContains(t, string(sealed), "sk-test")

	got, err := openKey(sl, orgID, id, sealed)
	require.NoError(t, err)
	assert.Equal(t, "sk-test", got)
}

func TestSealKey_AAD(t *testing.T) {
	orgID, id := uuid.New(), uuid.New()
	assert.Equal(t, "modelprovider:"+orgID.String()+":"+id.String()+":api_key", string(keyAAD(orgID, id)))
}

func TestOpenKey_RefusesOtherRow(t *testing.T) {
	sl := testSealer(t)
	orgID := uuid.New()
	sealed, err := sealKey(sl, orgID, uuid.New(), "sk-test")
	require.NoError(t, err)

	_, err = openKey(sl, orgID, uuid.New(), sealed)
	assert.True(t, sealer.IsOpenFailedError(err), "got %v", err)
}

func TestOpenKey_RefusesOtherOrg(t *testing.T) {
	sl := testSealer(t)
	id := uuid.New()
	sealed, err := sealKey(sl, uuid.New(), id, "sk-test")
	require.NoError(t, err)

	_, err = openKey(sl, uuid.New(), id, sealed)
	assert.True(t, sealer.IsOpenFailedError(err), "got %v", err)
}

func TestSealKey_DisabledSealer(t *testing.T) {
	_, err := sealKey(sealer.Disabled(), uuid.New(), uuid.New(), "sk-disabled")
	assert.True(t, sealer.IsUnavailableError(err), "got %v", err)

	_, err = openKey(sealer.Disabled(), uuid.New(), uuid.New(), []byte{1})
	assert.True(t, sealer.IsUnavailableError(err), "got %v", err)
}
