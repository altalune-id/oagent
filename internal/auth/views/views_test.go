package views

import (
	"bytes"
	"context"
	"testing"

	"github.com/a-h/templ"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/web/layout"
)

func render(t *testing.T, c templ.Component) string {
	t.Helper()
	var b bytes.Buffer
	require.NoError(t, c.Render(context.Background(), &b))
	return b.String()
}

func TestLogin_PasswordOnly(t *testing.T) {
	out := render(t, Login(layout.LayoutData{Title: "Sign in"}, LoginData{ForgotURL: "/forgot-password"}))
	assert.Contains(t, out, `name="password"`)
	assert.Contains(t, out, "Forgot password?")
	assert.Contains(t, out, "required autofocus>", "the email field takes focus when it is the only way in")
}

func TestLogin_PasswordOff(t *testing.T) {
	out := render(t, Login(layout.LayoutData{Title: "Sign in"}, LoginData{PasswordOff: true, ForgotURL: "/forgot-password"}))
	for _, gone := range []string{`name="password"`, "Forgot password?", "<form"} {
		assert.NotContains(t, out, gone)
	}
}
