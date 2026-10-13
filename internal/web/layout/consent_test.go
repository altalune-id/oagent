package layout

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func renderConsent(t *testing.T, c ConsentData) string {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, Consent(LayoutData{BasePath: "/x"}, c).Render(t.Context(), &buf))
	return strings.Join(strings.Fields(buf.String()), " ")
}

func TestConsent_Required(t *testing.T) {
	html := renderConsent(t, ConsentData{Required: true})
	assert.Contains(t, html, `type="checkbox" name="accept_terms" value="1" required`)
	assert.Contains(t, html, "I agree to the ")
	assert.Contains(t, html, `href="/x/terms" target="_blank" rel="noopener">Terms of Service</a> and the `)
	assert.Contains(t, html, `href="/x/privacy" target="_blank" rel="noopener">Privacy Policy</a>.`)
}

func TestConsent_NotRequired(t *testing.T) {
	html := renderConsent(t, ConsentData{TermsURL: "https://legal.example.com/tos", PrivacyURL: "https://legal.example.com/pp"})
	assert.NotContains(t, html, "checkbox")
	assert.NotContains(t, html, "accept_terms")
	assert.Contains(t, html, "By continuing, you agree to the ")
	assert.Contains(t, html, `href="https://legal.example.com/tos" target="_blank" rel="noopener">Terms of Service</a>`)
	assert.Contains(t, html, `href="https://legal.example.com/pp" target="_blank" rel="noopener">Privacy Policy</a>`)
}
