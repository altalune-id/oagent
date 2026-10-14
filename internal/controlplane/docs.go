package controlplane

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
)

const swaggerUIVersion = "5.17.14"

// NOTE: this page sets its own CSP so the third-party Swagger UI assets and inline bootstrap load.
func docsHandler(specURL string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		nonce := docsNonce()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Security-Policy", docsCSP(nonce))
		w.Header().Del("Content-Security-Policy-Report-Only")
		_, _ = w.Write(swaggerHTML(specURL, nonce))
	})
}

func docsCSP(nonce string) string {
	return "default-src 'self'; " +
		"script-src 'self' 'nonce-" + nonce + "' https://unpkg.com; " +
		"style-src 'self' 'unsafe-inline' https://unpkg.com; " +
		"img-src 'self' data:; font-src 'self' data:; connect-src 'self'; " +
		"object-src 'none'; base-uri 'self'; frame-ancestors 'none'"
}

func docsNonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.RawStdEncoding.EncodeToString(b)
}

func swaggerHTML(specURL, nonce string) []byte {
	const tmpl = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>oagent API docs</title>
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@%[1]s/swagger-ui.css">
  <style>body{margin:0;background:#fafafa;}</style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@%[1]s/swagger-ui-bundle.js" nonce="%[3]s" crossorigin></script>
  <script src="https://unpkg.com/swagger-ui-dist@%[1]s/swagger-ui-standalone-preset.js" nonce="%[3]s" crossorigin></script>
  <script nonce="%[3]s">
    window.addEventListener('load', function () {
      window.ui = SwaggerUIBundle({
        url: %[2]q,
        dom_id: '#swagger-ui',
        deepLinking: true,
        presets: [SwaggerUIBundle.presets.apis, SwaggerUIStandalonePreset],
        plugins: [SwaggerUIBundle.plugins.DownloadUrl],
        layout: 'StandaloneLayout',
        docExpansion: 'list',
        defaultModelsExpandDepth: 0,
        withCredentials: true
      });
    });
  </script>
</body>
</html>
`
	return fmt.Appendf(nil, tmpl, swaggerUIVersion, specURL, nonce)
}
