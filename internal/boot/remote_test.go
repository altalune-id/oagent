package boot

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/config"
	authv1 "altalune.id/oagent/gen/go/auth/v1"
	"altalune.id/oagent/internal/apiclient"
)

func TestBootRemote(t *testing.T) {
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/proto")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	cfg := config.Config{Env: config.EnvDevelopment}
	log := testLogger()
	r, err := BootRemote(t.Context(), cfg, log, srv.URL+"/", apiclient.Credential{Token: "oagent_tok"})
	require.NoError(t, err)
	assert.Equal(t, cfg, *r.Cfg)
	assert.Same(t, log, r.Log)

	_, err = r.Conn.Auth.Whoami(t.Context(), connect.NewRequest(&authv1.WhoamiRequest{}))
	require.NoError(t, err)
	assert.Equal(t, "/api/auth.v1.AuthService/Whoami", gotPath)
	assert.Equal(t, "Bearer oagent_tok", gotAuth)
}
