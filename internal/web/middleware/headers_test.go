package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSecurityHeaders(t *testing.T) {
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	rec := httptest.NewRecorder()
	SecurityHeaders(next).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login", nil))

	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
	assert.Equal(t, "frame-ancestors 'none'", rec.Header().Get("Content-Security-Policy"))
	assert.Equal(t, "strict-origin-when-cross-origin", rec.Header().Get("Referrer-Policy"))
}

func TestSecurityHeaders_HandlerCanOverride(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Referrer-Policy", "no-referrer")
	})
	rec := httptest.NewRecorder()
	SecurityHeaders(next).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/invite/x", nil))

	assert.Equal(t, "no-referrer", rec.Header().Get("Referrer-Policy"))
}

func TestNoStore(t *testing.T) {
	for name, status := range map[string]int{"ok": http.StatusOK, "redirect": http.StatusSeeOther, "gone": http.StatusGone} {
		t.Run(name, func(t *testing.T) {
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) })
			rec := httptest.NewRecorder()
			NoStore(next).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/invite/x", nil))

			assert.Equal(t, status, rec.Code)
			assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
		})
	}
}
