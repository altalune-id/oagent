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

func TestValidation(t *testing.T) {
	ae := Validation([]Violation{
		{Field: "members[0].display_name", Reason: FieldTooLong, Param: "200"},
		{Field: "email", Reason: FieldInvalidFormat},
	})
	assert.Equal(t, CodeValidation, ae.Code())
	assert.Equal(t, "members[0].display name is too long", ae.Message())
	assert.Equal(t, map[string]string{"field": "members[0].display_name", "reason": "too_long", "param": "200"}, ae.Meta())
	assert.Equal(t, []Violation{
		{Field: "members[0].display_name", Reason: FieldTooLong, Param: "200"},
		{Field: "email", Reason: FieldInvalidFormat},
	}, ae.Violations())
}

func TestValidationWithoutParamLeavesItOutOfMeta(t *testing.T) {
	ae := Validation([]Violation{{Field: "id", Reason: FieldInvalidFormat}})
	assert.Equal(t, map[string]string{"field": "id", "reason": "invalid_format"}, ae.Meta())
}

func TestViolationsIsACopy(t *testing.T) {
	ae := Validation([]Violation{{Field: "id", Reason: FieldRequired}})
	ae.Violations()[0].Field = "changed"
	assert.Equal(t, "id", ae.Violations()[0].Field)
}

func TestViolationsSurviveCopies(t *testing.T) {
	ae := Validation([]Violation{{Field: "id", Reason: FieldRequired}}).WithMeta(map[string]string{"x": "y"}).WithCause(assert.AnError)
	assert.Equal(t, []Violation{{Field: "id", Reason: FieldRequired}}, ae.Violations())
}

func TestInvalidHasNoViolations(t *testing.T) {
	assert.Empty(t, Invalid("owner", FieldRequired).Violations())
}

func TestValidationWithNoViolations(t *testing.T) {
	ae := Validation(nil)
	assert.Equal(t, CodeValidation, ae.Code())
	assert.Empty(t, ae.Meta())
}
