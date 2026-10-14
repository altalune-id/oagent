package layout

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/a-h/templ"
	templruntime "github.com/a-h/templ/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
)

func TestLayoutDataHref(t *testing.T) {
	cases := []struct {
		name     string
		basePath string
		sub      string
		want     string
	}{
		{"empty base", "", "/login", "/login"},
		{"base", "/app", "/login", "/app/login"},
		{"base with trailing slash", "/app/", "/login", "/app/login"},
		{"root base", "/", "/login", "/login"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, LayoutData{BasePath: c.basePath}.Href(c.sub))
		})
	}
}

func TestLayoutDataStatic(t *testing.T) {
	assert.Equal(t, "/app/static/app.css", LayoutData{BasePath: "/app/"}.Static("/app.css"))
	assert.Equal(t, "/static/favicon.ico", LayoutData{}.Static("/favicon.ico"))
}

func TestLayoutDataLang(t *testing.T) {
	assert.Equal(t, "en", LayoutData{}.lang())
	assert.Equal(t, "id", LayoutData{Lang: "id"}.lang())
}

func TestUnderAny(t *testing.T) {
	prefixes := []string{"/console", "/api"}
	cases := []struct {
		name string
		p    string
		want bool
	}{
		{"exact match", "/console", true},
		{"below a prefix", "/api/v1/orgs", true},
		{"shared prefix but not below", "/consoles", false},
		{"unrelated", "/login", false},
		{"root", "/", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, UnderAny(c.p, prefixes))
		})
	}
	assert.False(t, UnderAny("/console", nil))
}

func TestKitPathAndLayout(t *testing.T) {
	k := Kit{BasePath: "/app/"}
	assert.Equal(t, "/app/login", k.Path("login"))
	assert.Equal(t, "/app", k.Path(""))
	assert.Equal(t, LayoutData{BasePath: "/app/", Title: "Sign in"}, k.Layout("Sign in"))
}

func renderString(t *testing.T, c templ.Component) string {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, c.Render(context.Background(), &buf))
	return buf.String()
}

func TestLayoutTemplate(t *testing.T) {
	body := templ.Raw("<p>inner</p>")
	ctx := templ.WithChildren(context.Background(), body)
	var buf bytes.Buffer
	require.NoError(t, Layout(LayoutData{BasePath: "/app", Title: "Home <x>", Lang: "id"}).Render(ctx, &buf))
	out := buf.String()

	assert.Contains(t, out, "<!doctype html>")
	assert.Contains(t, out, `<html lang="id" data-theme="oagent-light">`)
	assert.Contains(t, out, "<title>Home &lt;x&gt; · oagent</title>")
	assert.Contains(t, out, `href="/app/static/favicon.ico"`)
	assert.Contains(t, out, `href="/app/static/favicon-32x32.png"`)
	assert.Contains(t, out, `href="/app/static/apple-touch-icon.png"`)
	assert.Contains(t, out, `href="/app/static/app.css"`)
	assert.Contains(t, out, "<p>inner</p>")
}

func TestLayoutTemplateDefaultsLang(t *testing.T) {
	out := renderString(t, Layout(LayoutData{Title: "Home"}))
	assert.Contains(t, out, `<html lang="en"`)
}

func TestCardTemplate(t *testing.T) {
	ctx := templ.WithChildren(context.Background(), templ.Raw("<p>body</p>"))
	var buf bytes.Buffer
	require.NoError(t, Card("Welcome & hi").Render(ctx, &buf))
	out := buf.String()

	assert.Contains(t, out, `<h1 class="card-title text-2xl">Welcome &amp; hi</h1>`)
	assert.Contains(t, out, "<p>body</p>")
}

func TestErrorTemplate(t *testing.T) {
	out := renderString(t, Error(LayoutData{BasePath: "/app", Title: "Oops"}, "Try <again>"))

	assert.Contains(t, out, "<title>Oops · oagent</title>")
	assert.Contains(t, out, `<h1 class="card-title text-2xl">Oops</h1>`)
	assert.Contains(t, out, "<p>Try &lt;again&gt;</p>")
	assert.Contains(t, out, `href="/app/console/"`)
}

func TestKitRender(t *testing.T) {
	k := Kit{BasePath: "/app"}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/app/x", nil)

	k.Render(rec, req, http.StatusTeapot, templ.Raw("<p>hello</p>"))

	assert.Equal(t, http.StatusTeapot, rec.Code)
	assert.Equal(t, "text/html; charset=utf-8", rec.Header().Get("Content-Type"))
	assert.Equal(t, "<p>hello</p>", rec.Body.String())
}

func failing(err error) templ.Component {
	return templ.ComponentFunc(func(context.Context, io.Writer) error { return err })
}

func TestKitRenderLogsFailure(t *testing.T) {
	var logs bytes.Buffer
	k := Kit{Log: slog.New(slog.NewTextHandler(&logs, nil))}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	k.Render(rec, req, http.StatusOK, failing(errors.New("boom")))

	assert.Contains(t, logs.String(), "web: render failed")
	assert.Contains(t, logs.String(), "boom")
}

func TestKitRenderSkipsLogWhenClientGone(t *testing.T) {
	var logs bytes.Buffer
	k := Kit{Log: slog.New(slog.NewTextHandler(&logs, nil))}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)

	k.Render(rec, req, http.StatusOK, failing(context.Canceled))

	assert.Empty(t, logs.String())
}

func TestKitRenderFallsBackToDefaultLogger(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	Kit{}.Render(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil), http.StatusOK, failing(errors.New("boom")))

	assert.Contains(t, logs.String(), "web: render failed")
}

func TestKitFail(t *testing.T) {
	var gotMsg string
	var gotErr error
	k := Kit{
		BasePath: "/app",
		Unexpected: func(_ context.Context, msg string, cause error, _ ...any) *apperror.AppError {
			gotMsg, gotErr = msg, cause
			return nil
		},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/app/x", nil)
	cause := errors.New("secret db detail")

	k.Fail(rec, req, "load page", cause)

	assert.Equal(t, "load page", gotMsg)
	assert.Equal(t, cause, gotErr)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "Something went wrong")
	assert.Contains(t, body, "Please try again in a moment.")
	assert.NotContains(t, body, "secret db detail")
}

func TestKitFailWithoutReporter(t *testing.T) {
	rec := httptest.NewRecorder()

	Kit{}.Fail(rec, httptest.NewRequest(http.MethodGet, "/", nil), "load page", errors.New("boom"))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "Something went wrong")
}

type limitWriter struct {
	left int
}

func (w *limitWriter) Write(p []byte) (int, error) {
	if len(p) > w.left {
		n := w.left
		w.left = 0
		return n, errWriteLimit
	}
	w.left -= len(p)
	return len(p), nil
}

var errWriteLimit = errors.New("write limit reached")

func unbufferedTo(w io.Writer) *templruntime.Buffer {
	prev := templruntime.DefaultBufferSize
	templruntime.DefaultBufferSize = 1
	defer func() { templruntime.DefaultBufferSize = prev }()
	b := &templruntime.Buffer{}
	b.Reset(w)
	return b
}

func TestTemplatesReturnWriteErrors(t *testing.T) {
	d := LayoutData{BasePath: "/app", Title: "Oops"}
	cases := []struct {
		name string
		c    templ.Component
	}{
		{"Layout", Layout(d)},
		{"Card", Card("Title")},
		{"Error", Error(d, "message")},
		{"PasswordField", PasswordField(d, PasswordFieldData{Name: "password", Label: "Password"})},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			full := len(renderString(t, tt.c))
			for limit := range full {
				b := unbufferedTo(&limitWriter{left: limit})
				err := tt.c.Render(context.Background(), b)
				if err == nil {
					err = b.Flush()
				}
				require.ErrorIs(t, err, errWriteLimit, "limit %d", limit)
			}
			b := unbufferedTo(&limitWriter{left: full})
			require.NoError(t, tt.c.Render(context.Background(), b))
			assert.NoError(t, b.Flush())
		})
	}
}

func TestTemplatesStopOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	d := LayoutData{Title: "Oops"}
	for _, c := range []templ.Component{Layout(d), Card("Title"), Error(d, "message"), PasswordField(d, PasswordFieldData{})} {
		var buf bytes.Buffer
		require.ErrorIs(t, c.Render(ctx, &buf), context.Canceled)
		assert.Empty(t, buf.String())
	}
}

func TestTemplatesReturnChildErrors(t *testing.T) {
	ctx := templ.WithChildren(context.Background(), failing(errWriteLimit))
	for _, c := range []templ.Component{Layout(LayoutData{}), Card("Title")} {
		var buf bytes.Buffer
		assert.ErrorIs(t, c.Render(ctx, &buf), errWriteLimit)
	}
}
