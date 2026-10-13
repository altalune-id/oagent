package config

import (
	"testing"
	"time"

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

func TestLoad_RequireAcceptance(t *testing.T) {
	assert.False(t, validConfig(t).Compliance.RequireAcceptance)
	t.Setenv("OAGENT_COMPLIANCE_REQUIRE_ACCEPTANCE", "true")
	assert.True(t, validConfig(t).Compliance.RequireAcceptance)
}

func TestValidate_TermsUpdatedAt(t *testing.T) {
	c := validConfig(t)
	c.Compliance.TermsUpdatedAt = "2026-10-01"
	require.NoError(t, c.Validate())
	assert.Equal(t, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), c.Compliance.TermsUpdated())

	c.Compliance.TermsUpdatedAt = "1 Oct 2026"
	require.Error(t, c.Validate())

	c.Compliance.TermsUpdatedAt = time.Now().UTC().AddDate(0, 0, 2).Format(time.DateOnly)
	err := c.Validate()
	require.Error(t, err, "a future date would send everyone back to /welcome forever")
	assert.Contains(t, err.Error(), "compliance.termsUpdatedAt")

	c.Compliance.TermsUpdatedAt = ""
	require.NoError(t, c.Validate())
	assert.True(t, c.Compliance.TermsUpdated().IsZero())
}

func TestLoad_TermsUpdatedAtFromEnv(t *testing.T) {
	t.Setenv("OAGENT_COMPLIANCE_TERMS_UPDATED_AT", "2026-09-30")
	assert.Equal(t, "2026-09-30", validConfig(t).Compliance.TermsUpdatedAt)
}
