package agent

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	orgID, projectID := uuid.New(), uuid.New()
	a, err := New(orgID, projectID, "acme", "Acme Bot", "Helpful bot", uuid.New())
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, a.ID)
	assert.Len(t, a.PublicID, 21)
	assert.Equal(t, orgID, a.OrgID)
	assert.Equal(t, projectID, a.ProjectID)
	assert.Equal(t, "acme", a.Slug)
	assert.Equal(t, "Acme Bot", a.Name)
	assert.Equal(t, "Helpful bot", a.Description)
	assert.True(t, a.CreatedBy.Valid)
}

func TestNew_Genesis(t *testing.T) {
	a, err := New(uuid.New(), uuid.New(), "acme", "Acme", "", uuid.Nil)
	require.NoError(t, err)
	assert.False(t, a.CreatedBy.Valid)
	assert.Empty(t, a.Description)
}

func TestNew_Invalid(t *testing.T) {
	o, p := uuid.New(), uuid.New()
	_, err := New(o, p, "ab", "Acme", "", uuid.New())
	assert.True(t, IsInvalidSlugError(err))

	_, err = New(o, p, "acme", "", "", uuid.New())
	assert.True(t, IsInvalidNameError(err))

	_, err = New(o, p, "acme", "Acme", strings.Repeat("a", 2001), uuid.New())
	assert.True(t, IsInvalidDescriptionError(err))
}

func TestAgent_Update(t *testing.T) {
	a, err := New(uuid.New(), uuid.New(), "acme", "Acme", "old", uuid.New())
	require.NoError(t, err)

	require.NoError(t, a.Update("  Acme Two  ", "new"))
	assert.Equal(t, "Acme Two", a.Name)
	assert.Equal(t, "new", a.Description)
	assert.True(t, IsInvalidNameError(a.Update("", "x")))
	assert.True(t, IsInvalidDescriptionError(a.Update("Acme", strings.Repeat("a", 2001))))
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
