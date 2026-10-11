package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_ComplianceDefaults(t *testing.T) {
	c := validConfig(t)
	assert.Empty(t, c.Compliance.TermsURL)
	assert.Empty(t, c.Compliance.PrivacyURL)
}

func TestLoad_ComplianceFromEnv(t *testing.T) {
	t.Setenv("OAGENT_COMPLIANCE_TERMS_URL", "https://example.com/terms")
	t.Setenv("OAGENT_COMPLIANCE_PRIVACY_URL", "https://example.com/privacy")
	c := validConfig(t)
	assert.Equal(t, "https://example.com/terms", c.Compliance.TermsURL)
	assert.Equal(t, "https://example.com/privacy", c.Compliance.PrivacyURL)
}

func TestValidate_ComplianceURLs(t *testing.T) {
	c := validConfig(t)
	c.Compliance.TermsURL = "not a url"
	require.Error(t, c.Validate())

	c = validConfig(t)
	c.Compliance.PrivacyURL = "::"
	require.Error(t, c.Validate())
}
