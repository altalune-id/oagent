package controlplane

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/testutil/fakes"
)

func openAPIServer(t *testing.T, enabled bool, auth *BasicAuth) *httptest.Server {
	t.Helper()
	srv := New(Services{Orgs: org.NewService(&fakes.OrgStore{}, fakes.Transactor{}, nil)}, nil, Options{OpenAPIEnabled: enabled, OpenAPIBasicAuth: auth})
	ts := httptest.NewServer(srv.Handler(""))
	t.Cleanup(ts.Close)
	return ts
}

func basicHeader(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}

func TestOpenAPI_Disabled_Returns404(t *testing.T) {
	ts := openAPIServer(t, false, nil)

	for _, path := range []string{"/api/openapi.yaml", "/api/openapi.json", "/api/docs"} {
		resp, err := http.Get(ts.URL + path)
		require.NoError(t, err)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode, path)
	}
}

func TestOpenAPI_Enabled_NoAuth_ReturnsYAMLAndJSON(t *testing.T) {
	ts := openAPIServer(t, true, nil)

	resp, err := http.Get(ts.URL + "/api/openapi.yaml")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.True(t, strings.HasPrefix(resp.Header.Get("Content-Type"), "application/yaml"))
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "openapi:")

	resp, err = http.Get(ts.URL + "/api/openapi.json")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ = io.ReadAll(resp.Body)
	assert.Contains(t, string(body), `"openapi"`)
}

func TestOpenAPI_BasicAuth(t *testing.T) {
	ts := openAPIServer(t, true, &BasicAuth{User: "docs", Password: "s3cret"})

	resp, err := http.Get(ts.URL + "/api/openapi.yaml")
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("WWW-Authenticate"), `Basic realm="oagent openapi"`)

	for _, creds := range []string{basicHeader("docs", "s3cret")} {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/openapi.yaml", nil)
		req.Header.Set("Authorization", creds)
		resp, err = http.DefaultClient.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	}

	for _, creds := range []string{basicHeader("docs", "nope"), basicHeader("wrong", "s3cret")} {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/openapi.yaml", nil)
		req.Header.Set("Authorization", creds)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	}
}

func TestOpenAPI_Docs(t *testing.T) {
	ts := openAPIServer(t, true, nil)

	resp, err := http.Get(ts.URL + "/api/docs")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.True(t, strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html"))
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "swagger-ui")
	assert.Contains(t, string(body), "/api/openapi.yaml")
}

func TestDocsHandler_SetsCSP(t *testing.T) {
	rec := httptest.NewRecorder()
	docsHandler("/app/api/openapi.yaml").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Security-Policy"), "nonce-")
	assert.Contains(t, rec.Body.String(), "swagger-ui")
}

func TestLoadOpenAPI_Merges(t *testing.T) {
	fsys := fstest.MapFS{
		"openapi/a/v1/a.openapi.yaml": &fstest.MapFile{Data: []byte(
			"openapi: 3.1.0\ninfo:\n  title: a\nx-custom: 1\ntags:\n  - name: A\npaths:\n  /a: {}\ncomponents:\n  schemas:\n    A: {type: object}\n")},
		"openapi/b/v1/b.openapi.yaml": &fstest.MapFile{Data: []byte(
			"openapi: 3.1.0\ninfo:\n  title: b\ntags:\n  - name: B\npaths:\n  /b: {}\ncomponents:\n  schemas:\n    B: {type: string}\n")},
	}

	yamlBody, jsonBody := loadOpenAPI(fsys)

	assert.Contains(t, string(yamlBody), "openapi:")
	assert.Contains(t, string(yamlBody), "/a")
	assert.Contains(t, string(yamlBody), "/b")
	assert.Contains(t, string(yamlBody), "x-custom")
	assert.Contains(t, string(jsonBody), `"/a"`)
	assert.Contains(t, string(jsonBody), `"/b"`)
}

func TestLoadOpenAPI_FallbackYAML(t *testing.T) {
	fsys := fstest.MapFS{
		"openapi/x/v1/x.openapi.yaml": &fstest.MapFile{Data: []byte(": not : valid : [")},
	}

	yamlBody, jsonBody := loadOpenAPI(fsys)

	assert.Contains(t, string(yamlBody), "# openapi/x/v1/x.openapi.yaml")
	assert.Equal(t, "{}\n", string(jsonBody))
}

func TestLoadOpenAPI_NoFiles(t *testing.T) {
	yamlBody, jsonBody := loadOpenAPI(fstest.MapFS{})
	assert.Nil(t, yamlBody)
	assert.Nil(t, jsonBody)
}

func TestMergeOpenAPI_Empty(t *testing.T) {
	assert.Nil(t, mergeOpenAPI(nil))
}

func TestOpenAPIGuard_NilAllows(t *testing.T) {
	h := openAPIGuard(nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusNoContent, rec.Code)
}
