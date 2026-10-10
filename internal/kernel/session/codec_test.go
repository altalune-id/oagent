package session

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/sealer"
)

func newSealer(t *testing.T) sealer.Sealer {
	t.Helper()
	key, err := sealer.GenerateKey()
	require.NoError(t, err)
	s, err := sealer.New(key)
	require.NoError(t, err)
	return s
}

func TestCodec_RoundTrip(t *testing.T) {
	s := newSealer(t)
	p := Principal{UserID: uuid.New(), Email: "a@example.com", Source: SourceLocal, ActiveOrgID: uuid.New()}
	blob, err := sealPrincipal(s, "sid-rt", p)
	require.NoError(t, err)
	got, err := openPrincipal(s, "sid-rt", blob)
	require.NoError(t, err)
	assert.Equal(t, p.UserID, got.UserID)
	assert.Equal(t, p.ActiveOrgID, got.ActiveOrgID)
}

func TestCodec_BoundToSID(t *testing.T) {
	s := newSealer(t)
	blob, err := sealPrincipal(s, "sid-1", Principal{UserID: uuid.New()})
	require.NoError(t, err)
	_, err = openPrincipal(s, "sid-2", blob)
	assert.True(t, sealer.IsOpenFailedError(err), "a payload copied to another sid must not open; got %v", err)
}

func TestCodec_Tampered(t *testing.T) {
	s := newSealer(t)
	blob, err := sealPrincipal(s, "sid-1", Principal{UserID: uuid.New()})
	require.NoError(t, err)
	blob[len(blob)-1] ^= 0xff
	_, err = openPrincipal(s, "sid-1", blob)
	assert.True(t, sealer.IsOpenFailedError(err), "got %v", err)
}

func TestCodec_SealUnavailable(t *testing.T) {
	_, err := sealPrincipal(sealer.Disabled(), "sid-2", Principal{UserID: uuid.New()})
	assert.True(t, sealer.IsUnavailableError(err), "got %v", err)
}

func TestCodec_OpenRejectsNonPrincipal(t *testing.T) {
	s := newSealer(t)
	blob, err := s.Seal([]byte("not json"), []byte("sid-1"))
	require.NoError(t, err)
	_, err = openPrincipal(s, "sid-1", blob)
	require.Error(t, err)
	assert.False(t, sealer.IsOpenFailedError(err))
}
