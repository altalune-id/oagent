package org

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOrg(t *testing.T) {
	o, err := NewOrg("acme", "Acme Inc", uuid.New())
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, o.ID)
	assert.Len(t, o.PublicID, 21)
	assert.Equal(t, "acme", o.Slug)
	assert.Equal(t, "Acme Inc", o.Name)
	assert.True(t, o.CreatedBy.Valid)
}

func TestNewOrg_Genesis(t *testing.T) {
	o, err := NewOrg("acme", "Acme", uuid.Nil)
	require.NoError(t, err)
	assert.False(t, o.CreatedBy.Valid)
}

func TestNewOrg_EmptySlug(t *testing.T) {
	_, err := NewOrg("  ", "Acme", uuid.New())
	assert.True(t, IsInvalidSlugError(err))
}

func TestNewOrg_Invalid(t *testing.T) {
	_, err := NewOrg("ab", "Acme", uuid.New())
	assert.True(t, IsInvalidSlugError(err))

	_, err = NewOrg("acme", "", uuid.New())
	assert.True(t, IsInvalidNameError(err))
}

func TestOrg_Update(t *testing.T) {
	o, err := NewOrg("acme", "Acme", uuid.New())
	require.NoError(t, err)

	require.NoError(t, o.Update("  Acme Two  ", "acme-two"))
	assert.Equal(t, "Acme Two", o.Name)
	assert.Equal(t, "acme-two", o.Slug)
	assert.True(t, IsInvalidNameError(o.Update("", "acme-two")))
	assert.True(t, IsInvalidSlugError(o.Update("Acme", "Bad_Slug")))
}

func TestValidateSlug(t *testing.T) {
	assert.NoError(t, ValidateSlug("acme"))
	assert.True(t, IsInvalidSlugError(ValidateSlug("ab")))
	assert.True(t, IsInvalidSlugError(ValidateSlug(strings.Repeat("a", 65))))
	assert.True(t, IsInvalidSlugError(ValidateSlug("Bad_Slug")))
	assert.True(t, IsInvalidSlugError(ValidateSlug("new")))
}

func TestValidateName(t *testing.T) {
	assert.NoError(t, ValidateName("Acme"))
	assert.True(t, IsInvalidNameError(ValidateName("")))
	assert.True(t, IsInvalidNameError(ValidateName(strings.Repeat("a", 201))))
}

func TestNewSystemOrg(t *testing.T) {
	o, err := NewSystemOrg("acme", "Acme", uuid.Nil)
	require.NoError(t, err)
	assert.True(t, o.IsSystem)
	assert.False(t, o.CreatedBy.Valid, "basic mode creates it without an owner")

	owner := uuid.New()
	o, err = NewSystemOrg("acme", "Acme", owner)
	require.NoError(t, err)
	assert.Equal(t, owner, o.CreatedBy.UUID)

	_, err = NewSystemOrg("A", "Acme", uuid.Nil)
	assert.True(t, IsInvalidSlugError(err))

	plain, err := NewOrg("acme", "Acme", uuid.Nil)
	require.NoError(t, err)
	assert.False(t, plain.IsSystem)
}
