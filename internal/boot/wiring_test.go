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

func TestProjectSlugsSlugOf(t *testing.T) {
	p, err := project.NewProject(uuid.New(), "acme", "Acme", uuid.Nil)
	require.NoError(t, err)
	svc := project.NewService(&fakes.ProjectStore{Project: p}, testUnexpected())

	slug, err := projectSlugs{projects: svc}.SlugOf(context.Background(), p.ID)
	require.NoError(t, err)
	assert.Equal(t, "acme", slug)
}
