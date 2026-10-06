package middleware

import (
	"net/http"

	"altalune.id/oagent/reqid"
)

// RequestID sanitizes the inbound X-Request-Id (or mints one), echoes it, and puts it on the context.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := reqid.FromHTTPHeader(r)
		if id == "" {
			id = reqid.New()
		}
		w.Header().Set(reqid.Header, id)
		next.ServeHTTP(w, r.WithContext(reqid.WithContext(r.Context(), id)))
	})
}
