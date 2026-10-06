package middleware

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
)

func TestRecoverer(t *testing.T) {
	panicking := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") })

	for _, srv := range []http.Handler{
		Recoverer(nil, nil, panicking),
		Recoverer(slog.New(slog.NewTextHandler(io.Discard, nil)), nil, panicking),
	} {
		rec := httptest.NewRecorder()
		require.NotPanics(t, func() {
			srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		})
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	}
}

func TestRecoverer_Reports(t *testing.T) {
	var got error
	unexpected := func(_ context.Context, _ string, cause error, _ ...any) *apperror.AppError {
		got = cause
		return apperror.New(apperror.CodeUnexpected, "x")
	}

	panicking := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") })
	rec := httptest.NewRecorder()
	Recoverer(nil, unexpected, panicking).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.Error(t, got)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
