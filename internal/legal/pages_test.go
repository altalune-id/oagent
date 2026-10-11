package legal

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/internal/web"
	"altalune.id/oagent/internal/web/layout"
)

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestLegalPages(t *testing.T) {
	for _, base := range []string{"", "/app"} {
		srv := web.NewServer(web.ServerOpts{BasePath: base, Routes: []web.Registrar{NewWeb(WebOpts{Kit: layout.Kit{BasePath: base}})}})
		for path, title := range map[string]string{"/terms": "Terms of Service", "/privacy": "Privacy Policy"} {
			t.Run(base+path, func(t *testing.T) {
				rec := get(t, srv, base+path)
				assert.Equal(t, http.StatusOK, rec.Code)
				assert.Equal(t, "text/html; charset=utf-8", rec.Header().Get("Content-Type"))
				assert.Contains(t, rec.Body.String(), "<title>"+title)
				assert.Contains(t, rec.Body.String(), `href="`+base+`/static/app.css"`)
				assert.Contains(t, rec.Body.String(), "<h2>")
			})
		}
	}
}

func TestLegalPages_URLOverride(t *testing.T) {
	srv := web.NewServer(web.ServerOpts{Routes: []web.Registrar{NewWeb(WebOpts{
		TermsURL:   "https://example.com/terms",
		PrivacyURL: "https://example.com/privacy",
	})}})

	rec := get(t, srv, "/terms")
	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "https://example.com/terms", rec.Header().Get("Location"))

	rec = get(t, srv, "/privacy")
	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "https://example.com/privacy", rec.Header().Get("Location"))
}
