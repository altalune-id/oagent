package mcpserver

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

func TestSealAuth_RoundTrip(t *testing.T) {
	sl := testSealer(t)
	orgID, id := uuid.New(), uuid.New()
	sealed, err := sealAuth(sl, orgID, id, "tok-1")
	require.NoError(t, err)
	assert.NotContains(t, string(sealed), "tok-1")
	got, err := openAuth(sl, orgID, id, sealed)
	require.NoError(t, err)
	assert.Equal(t, "tok-1", got)
}

func TestSealAuth_AAD(t *testing.T) {
	orgID, id := uuid.New(), uuid.New()
	assert.Equal(t, "mcpserver:"+orgID.String()+":"+id.String()+":auth", string(authAAD(orgID, id)))
}

func TestOpenAuth_RefusesOtherRowAndOrg(t *testing.T) {
	sl := testSealer(t)
	orgID, id := uuid.New(), uuid.New()
	sealed, err := sealAuth(sl, orgID, id, "tok-2")
	require.NoError(t, err)
	_, err = openAuth(sl, orgID, uuid.New(), sealed)
	assert.True(t, sealer.IsOpenFailedError(err), "other row: %v", err)
	_, err = openAuth(sl, uuid.New(), id, sealed)
	assert.True(t, sealer.IsOpenFailedError(err), "other org: %v", err)
}

func TestSealAuth_DisabledSealer(t *testing.T) {
	_, err := sealAuth(sealer.Disabled(), uuid.New(), uuid.New(), "tok-3")
	assert.True(t, sealer.IsUnavailableError(err))
	_, err = openAuth(sealer.Disabled(), uuid.New(), uuid.New(), []byte{1})
	assert.True(t, sealer.IsUnavailableError(err))
}
