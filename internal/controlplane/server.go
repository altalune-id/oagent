// Package controlplane wires the Connect-RPC handlers into an http.Handler.
package controlplane

import (
	"context"
	"fmt"
	"net/http"

	"connectrpc.com/connect"

	"altalune.id/auth-server/apperror"
	orgv1connect "altalune.id/auth-server/gen/go/org/v1/orgv1connect"
	"altalune.id/auth-server/internal/org"
)

// Options configures the optional parts of the controlplane surface.
type Options struct {
	OpenAPIEnabled   bool
	OpenAPIBasicAuth *BasicAuth
}

// Server holds the wired Connect handlers.
type Server struct {
	Orgs *org.Service

	OrgSvc *org.OrgService

	OpenAPIEnabled   bool
	OpenAPIBasicAuth *BasicAuth

	unexpected apperror.UnexpectedFunc
}

// New builds a Server.
func New(orgs *org.Service, unexpected apperror.UnexpectedFunc, opts Options) *Server {
	return &Server{
		Orgs:             orgs,
		OrgSvc:           org.NewOrgService(orgs),
		OpenAPIEnabled:   opts.OpenAPIEnabled,
		OpenAPIBasicAuth: opts.OpenAPIBasicAuth,
		unexpected:       unexpected,
	}
}

// Handler mounts the Connect handlers and OpenAPI endpoints under basePath+"/api".
func (s *Server) Handler(basePath string) http.Handler {
	inner := http.NewServeMux()
	orgPath, orgHandler := orgv1connect.NewOrgServiceHandler(s.OrgSvc, s.handlerOptions()...)
	inner.Handle(orgPath, orgHandler)

	if s.OpenAPIEnabled {
		yamlBody, jsonBody := openAPI()
		if len(yamlBody) > 0 {
			guard := openAPIGuard(s.OpenAPIBasicAuth)
			inner.Handle("/openapi.yaml", s.recoverHTTP(guard(openAPIHandler(yamlBody, "application/yaml"))))
			inner.Handle("/openapi.json", s.recoverHTTP(guard(openAPIHandler(jsonBody, "application/json"))))
			inner.Handle("/docs", s.recoverHTTP(guard(docsHandler(basePath+"/api/openapi.yaml"))))
		}
	}

	mount := basePath + "/api"
	outer := http.NewServeMux()
	outer.Handle(mount+"/", http.StripPrefix(mount, inner))
	return outer
}

func (s *Server) handlerOptions() []connect.HandlerOption {
	ics := []connect.Interceptor{
		RequestID(),
	}
	if otel, err := OTel(nil, nil); err == nil && otel != nil {
		ics = append(ics, otel)
	}
	ics = append(ics, Wrap(s.unexpected))
	return []connect.HandlerOption{
		connect.WithRecover(s.recoverPanic),
		connect.WithInterceptors(ics...),
	}
}

func (s *Server) recoverPanic(ctx context.Context, _ connect.Spec, _ http.Header, p any) error {
	return Translate(ctx, fmt.Errorf("panic: %v", p), s.unexpected)
}

func (s *Server) recoverHTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			p := recover()
			if p == nil {
				return
			}
			err := fmt.Errorf("panic: %v", p)
			if s.unexpected != nil {
				_ = s.unexpected(r.Context(), "api: unexpected", err)
			}
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}()
		next.ServeHTTP(w, r)
	})
}
