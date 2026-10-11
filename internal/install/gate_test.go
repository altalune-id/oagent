package install

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstallGate(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	required := &atomic.Bool{}
	required.Store(true)

	for _, base := range []string{"", "/app"} {
		gate := InstallGate(base, required)(ok)
		cases := map[string]int{
			"/healthz":               http.StatusTeapot,
			"/readyz":                http.StatusTeapot,
			base + "/install":        http.StatusTeapot,
			base + "/static/app.css": http.StatusTeapot,
			base + "/":               http.StatusSeeOther,
			base + "/login":          http.StatusSeeOther,
			base + "/console/":       http.StatusSeeOther,
			base + "/installer":      http.StatusSeeOther,
			base + "/api/x.v1.S/M":   http.StatusServiceUnavailable,
			base + "/mcp":            http.StatusServiceUnavailable,
			base + "/mcp/messages":   http.StatusServiceUnavailable,
		}
		for path, want := range cases {
			t.Run(base+path, func(t *testing.T) {
				rec := get(t, gate, path)
				assert.Equal(t, want, rec.Code)
				switch want {
				case http.StatusSeeOther:
					assert.Equal(t, base+"/install", rec.Header().Get("Location"))
				case http.StatusServiceUnavailable:
					assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
					var body map[string]string
					require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
					assert.Equal(t, map[string]string{"code": "INS002", "message": "setup required"}, body)
				}
			})
		}
	}
}

func TestInstallGate_OpenOnceInstalled(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	required := &atomic.Bool{}
	gate := InstallGate("", required)(ok)
	assert.Equal(t, http.StatusTeapot, get(t, gate, "/console/").Code)

	assert.Equal(t, http.StatusTeapot, get(t, InstallGate("", nil)(ok), "/console/").Code)
}
