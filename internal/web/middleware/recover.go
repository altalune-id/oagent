package middleware

import (
	"fmt"
	"log/slog"
	"net/http"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/reqid"
)

// Recoverer turns a handler panic into a 500, reporting it through unexpected when set.
func Recoverer(log *slog.Logger, unexpected apperror.UnexpectedFunc, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				if unexpected != nil {
					_ = unexpected(r.Context(), "web: panic", fmt.Errorf("panic: %v", rec))
				} else if log != nil {
					log.ErrorContext(r.Context(), "web: panic recovered",
						slog.Any("panic", rec), slog.String("request_id", reqid.FromContext(r.Context())))
				}
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
