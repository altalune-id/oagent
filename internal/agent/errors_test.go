package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/apperror"
)

func TestErrors(t *testing.T) {
	assert.Contains(t, (&NotFoundError{Slug: "s"}).Error(), "s")
	assert.Contains(t, (&NotFoundError{ID: "i"}).Error(), "i")
	assert.Contains(t, (*NotFoundError)(nil).Error(), "not found")
	assert.Contains(t, (&NotFoundError{}).Error(), "not found")
	assert.True(t, IsNotFoundError(&NotFoundError{}))
	assert.False(t, IsNotFoundError(nil))

	assert.Contains(t, (&AlreadyExistsError{Slug: "s"}).Error(), "s")
	assert.True(t, IsAlreadyExistsError(&AlreadyExistsError{}))
	assert.False(t, IsAlreadyExistsError(nil))

	assert.Contains(t, (&InvalidSlugError{Slug: "s"}).Error(), "slug")
	assert.Contains(t, (&InvalidSlugError{Reason: "r"}).Error(), "r")
	assert.True(t, IsInvalidSlugError(&InvalidSlugError{}))
	assert.False(t, IsInvalidSlugError(nil))

	assert.Contains(t, (&InvalidNameError{}).Error(), "name")
	assert.Contains(t, (&InvalidNameError{Reason: "r"}).Error(), "r")
	assert.True(t, IsInvalidNameError(&InvalidNameError{}))

	assert.Contains(t, (&InvalidDescriptionError{}).Error(), "description")
	assert.Contains(t, (&InvalidDescriptionError{Reason: "r"}).Error(), "r")
	assert.True(t, IsInvalidDescriptionError(&InvalidDescriptionError{}))
	assert.False(t, IsInvalidDescriptionError(nil))
}

func TestIsExpectedError(t *testing.T) {
	assert.True(t, IsExpectedError(&NotFoundError{}))
	assert.True(t, IsExpectedError(&AlreadyExistsError{}))
	assert.True(t, IsExpectedError(&InvalidSlugError{}))
	assert.True(t, IsExpectedError(&InvalidNameError{}))
	assert.True(t, IsExpectedError(&InvalidDescriptionError{}))
	assert.False(t, IsExpectedError(assert.AnError))
	assert.False(t, IsExpectedError(nil))
}

func TestToAppError(t *testing.T) {
	assert.Equal(t, apperror.CodeAgentNotFound, (&NotFoundError{Slug: "s"}).ToAppError().Code())
	assert.Equal(t, apperror.CodeAgentAlreadyExists, (&AlreadyExistsError{Slug: "s"}).ToAppError().Code())
	assert.Equal(t, apperror.CodeAgentInvalidSlug, (&InvalidSlugError{}).ToAppError().Code())
	assert.Equal(t, apperror.CodeAgentInvalidName, (&InvalidNameError{}).ToAppError().Code())
	assert.Equal(t, apperror.CodeAgentInvalidDescription, (&InvalidDescriptionError{}).ToAppError().Code())
}
