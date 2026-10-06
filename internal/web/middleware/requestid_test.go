package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/reqid"
)

func TestRequestID(t *testing.T) {
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})

	rec := httptest.NewRecorder()
	RequestID(next).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.NotEmpty(t, rec.Header().Get(reqid.Header))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(reqid.Header, "incoming-id")
	rec = httptest.NewRecorder()
	RequestID(next).ServeHTTP(rec, req)
	assert.Equal(t, "incoming-id", rec.Header().Get(reqid.Header))
}
