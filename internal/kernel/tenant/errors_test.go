package tenant_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/tenant"
)

func TestErrors(t *testing.T) {
	assert.Equal(t, "tenant: missing context", (&tenant.MissingError{}).Error())
	assert.Equal(t, "tenant: context names no org", (&tenant.UnscopedError{}).Error())

	assert.Equal(t, apperror.CodeTenantMissing, (&tenant.MissingError{}).ToAppError().Code())
	assert.Equal(t, apperror.CodeTenantMissing, (&tenant.UnscopedError{}).ToAppError().Code())

	assert.False(t, tenant.IsMissingError(nil))
	assert.False(t, tenant.IsUnscopedError(nil))
}
