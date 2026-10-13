package layout

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/a-h/templ"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMail(t *testing.T) {
	var buf bytes.Buffer
	body := templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		_, err := io.WriteString(w, "<p>hello</p>")
		return err
	})
	ctx := templ.WithChildren(t.Context(), body)
	require.NoError(t, Mail("Join <Acme>").Render(ctx, &buf))
	html := buf.String()

	assert.Contains(t, html, "<title>Join &lt;Acme&gt;</title>")
	assert.Contains(t, html, `font-weight:600;">Join &lt;Acme&gt;</h1>`)
	assert.Contains(t, html, "<p>hello</p>")
}
