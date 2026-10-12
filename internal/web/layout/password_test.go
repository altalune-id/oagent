package layout

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPasswordField(t *testing.T) {
	var buf bytes.Buffer
	f := PasswordFieldData{Name: "password", Label: "Password", EmailInput: "email", NameInput: "name"}
	require.NoError(t, PasswordField(LayoutData{BasePath: "/x"}, f).Render(t.Context(), &buf))
	html := buf.String()

	for _, want := range []string{
		`data-password-field data-email-input="email" data-name-input="name"`,
		`type="password"`,
		`name="password"`,
		`autocomplete="new-password"`,
		`minlength="12"`,
		` required`,
		`aria-describedby="password-rules"`,
		`id="password-rules"`,
		`<li data-rule="length">At least 12 characters<span class="sr-only" data-rule-state></span></li>`,
		`<li data-rule="common">`,
		`<li data-rule="personal">`,
		`<progress class="progress w-full hidden" max="4" value="0" data-password-meter`,
		`<script type="module" src="/x/static/password.js"></script>`,
	} {
		assert.Contains(t, html, want)
	}
	assert.Equal(t, 3, strings.Count(html, "data-rule="))
	assert.Equal(t, 3, strings.Count(html, "data-rule-state"))
	assert.NotContains(t, html, `data-password-rules aria-live`, "rule state is read on demand, not announced per keystroke")
}
