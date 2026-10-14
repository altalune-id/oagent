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

func TestValidationMessages(t *testing.T) {
	for reason, want := range map[FieldReason]string{
		FieldRequired:      "org id is required",
		FieldTooShort:      "org id is too short",
		FieldTooLong:       "org id is too long",
		FieldInvalidFormat: "org id is not in the right format",
		FieldNotAllowed:    "org id is not an allowed value",
		FieldTooMany:       "org id has too many items",
		FieldInvalid:       "org id is invalid",
		"odd":              "org id is odd",
	} {
		assert.Equal(t, want, Validation([]Violation{{Field: "org_id", Reason: reason}}).Message())
	}
}
