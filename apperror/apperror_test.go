package apperror

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInvalid(t *testing.T) {
	ae := Invalid("organization_id", FieldInvalid)
	assert.Equal(t, CodeValidation, ae.Code())
	assert.Equal(t, "organization id is invalid", ae.Message())
	assert.Equal(t, map[string]string{"field": "organization_id", "reason": "invalid"}, ae.Meta())

	assert.Equal(t, "owner is required", Invalid("owner", FieldRequired).Message())
}
