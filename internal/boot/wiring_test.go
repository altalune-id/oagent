package boot

import (
	"bytes"
	"context"
	"encoding/hex"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/testutil/fakes"
	"altalune.id/oagent/sealer"
)

func TestBuildSealer(t *testing.T) {
	key := hex.EncodeToString(bytes.Repeat([]byte{1}, sealer.KeyLen))

	s := buildSealer(config.SecretsConfig{EncryptionKey: key})
	_, err := s.Seal([]byte("x"), nil)
	assert.NoError(t, err)

	d := buildSealer(config.SecretsConfig{EncryptionKey: ""})
	_, err = d.Seal(nil, nil)
	assert.True(t, sealer.IsUnavailableError(err))

	bad := buildSealer(config.SecretsConfig{EncryptionKey: "zz"})
	_, err = bad.Seal(nil, nil)
	assert.True(t, sealer.IsUnavailableError(err))
}

func TestRefResolver(t *testing.T) {
	p, err := project.NewProject(uuid.New(), "acme", "Acme", uuid.Nil)
	require.NoError(t, err)
	o, err := org.NewOrg("acme", "Acme", uuid.Nil)
	require.NoError(t, err)

	r := refResolver{
		projects: project.NewService(&fakes.ProjectStore{Project: p}, testUnexpected()),
		orgs:     org.NewService(&fakes.OrgStore{Org: o}, testUnexpected()),
	}

	slug, publicID, err := r.ProjectRef(context.Background(), p.ID)
	require.NoError(t, err)
	assert.Equal(t, "acme", slug)
	assert.Equal(t, p.PublicID, publicID)

	orgPublicID, err := r.OrgRef(context.Background(), o.ID)
	require.NoError(t, err)
	assert.Equal(t, o.PublicID, orgPublicID)
}
