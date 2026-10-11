package health

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/internal/web"
)

type flag struct{ ready bool }

func (f *flag) Ready() bool { return f.ready }

func probe(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestHealthz(t *testing.T) {
	srv := web.NewServer(web.ServerOpts{Probes: NewWeb(NewService(&flag{}))})
	rec := probe(t, srv, "/healthz")
	assert.Equal(t, http.StatusOK, rec.Code, "liveness ignores readiness")
	assert.Equal(t, "ok", rec.Body.String())
}

func TestReadyz(t *testing.T) {
	db, queue := &flag{}, &flag{ready: true}
	srv := web.NewServer(web.ServerOpts{Probes: NewWeb(NewService(db, nil, queue))})
	assert.Equal(t, http.StatusServiceUnavailable, probe(t, srv, "/readyz").Code)

	db.ready = true
	assert.Equal(t, http.StatusOK, probe(t, srv, "/readyz").Code)
}

func TestReadyz_NoChecksIsReady(t *testing.T) {
	srv := web.NewServer(web.ServerOpts{Probes: NewWeb(nil)})
	assert.Equal(t, http.StatusOK, probe(t, srv, "/readyz").Code)
}

func TestProbesStayRootAnchoredUnderBasePath(t *testing.T) {
	srv := web.NewServer(web.ServerOpts{BasePath: "/app", Probes: NewWeb(nil)})
	for _, path := range []string{"/healthz", "/readyz"} {
		assert.Equal(t, http.StatusOK, probe(t, srv, path).Code, path)
	}
	assert.Equal(t, http.StatusNotFound, probe(t, srv, "/app/healthz").Code)
}

func TestProbesSkipTheSessionChain(t *testing.T) {
	touched := false
	session := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			touched = true
			next.ServeHTTP(w, r)
		})
	}
	srv := web.NewServer(web.ServerOpts{Probes: NewWeb(nil), Session: session, Routes: []web.Registrar{NewWeb(nil)}})
	assert.Equal(t, http.StatusOK, probe(t, srv, "/readyz").Code)
	assert.False(t, touched)
}
