package project

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProject(t *testing.T) {
	orgID := uuid.New()
	p, err := NewProject(orgID, "acme", "Acme Inc", uuid.New())
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, p.ID)
	assert.Len(t, p.PublicID, 21)
	assert.Equal(t, orgID, p.OrgID)
	assert.Equal(t, "acme", p.Slug)
	assert.Equal(t, "Acme Inc", p.Name)
	assert.True(t, p.CreatedBy.Valid)
}

func TestNewProject_Genesis(t *testing.T) {
	p, err := NewProject(uuid.New(), "acme", "Acme", uuid.Nil)
	require.NoError(t, err)
	assert.False(t, p.CreatedBy.Valid)
}

func TestNewProject_Invalid(t *testing.T) {
	_, err := NewProject(uuid.New(), "ab", "Acme", uuid.New())
	assert.True(t, IsInvalidSlugError(err))

	_, err = NewProject(uuid.New(), "acme", "", uuid.New())
	assert.True(t, IsInvalidNameError(err))
}

func TestProject_Rename(t *testing.T) {
	p, err := NewProject(uuid.New(), "acme", "Acme", uuid.New())
	require.NoError(t, err)

	require.NoError(t, p.Rename("  Acme Two  "))
	assert.Equal(t, "Acme Two", p.Name)
	assert.True(t, IsInvalidNameError(p.Rename("")))
}

func TestValidateSlug(t *testing.T) {
	assert.NoError(t, ValidateSlug("acme"))
	assert.True(t, IsInvalidSlugError(ValidateSlug("ab")))
	assert.True(t, IsInvalidSlugError(ValidateSlug(strings.Repeat("a", 65))))
	assert.True(t, IsInvalidSlugError(ValidateSlug("Bad_Slug")))
}

func TestValidateName(t *testing.T) {
	assert.NoError(t, ValidateName("Acme"))
	assert.True(t, IsInvalidNameError(ValidateName("")))
	assert.True(t, IsInvalidNameError(ValidateName(strings.Repeat("a", 201))))
}
