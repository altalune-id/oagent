// Package controlplane wires the Connect-RPC handlers into an http.Handler.
package controlplane

import (
	"context"
	"fmt"
	"net/http"

	"connectrpc.com/connect"

	"altalune.id/oagent/apperror"
	agentv1connect "altalune.id/oagent/gen/go/agent/v1/agentv1connect"
	orgv1connect "altalune.id/oagent/gen/go/org/v1/orgv1connect"
	projectv1connect "altalune.id/oagent/gen/go/project/v1/projectv1connect"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/project"
)

// Options configures the optional parts of the controlplane surface.
type Options struct {
	OpenAPIEnabled   bool
	OpenAPIBasicAuth *BasicAuth
}

// Services are the domain services the controlplane exposes.
type Services struct {
	Orgs     *org.Service
	Projects *project.Service
	Agents   *agent.Service
}

// Server holds the wired Connect handlers.
type Server struct {
	Orgs     *org.Service
	Projects *project.Service
	Agents   *agent.Service

	OrgSvc     *org.OrgService
	ProjectSvc *project.ProjectService
	AgentSvc   *agent.AgentService

	OpenAPIEnabled   bool
	OpenAPIBasicAuth *BasicAuth

	unexpected apperror.UnexpectedFunc
}

// New builds a Server.
func New(svcs Services, unexpected apperror.UnexpectedFunc, opts Options) *Server {
	s := &Server{
		Orgs:             svcs.Orgs,
		Projects:         svcs.Projects,
		Agents:           svcs.Agents,
		OrgSvc:           org.NewControlPlane(svcs.Orgs),
		OpenAPIEnabled:   opts.OpenAPIEnabled,
		OpenAPIBasicAuth: opts.OpenAPIBasicAuth,
		unexpected:       unexpected,
	}
	if svcs.Projects != nil {
		s.ProjectSvc = project.NewControlPlane(svcs.Projects)
	}
	if svcs.Agents != nil {
		s.AgentSvc = agent.NewControlPlane(svcs.Agents)
	}
	return s
}

// Handler mounts the Connect handlers and OpenAPI endpoints under basePath+"/api".
func (s *Server) Handler(basePath string) http.Handler {
	inner := http.NewServeMux()
	orgPath, orgHandler := orgv1connect.NewOrgServiceHandler(s.OrgSvc, s.handlerOptions()...)
	inner.Handle(orgPath, orgHandler)
	if s.ProjectSvc != nil {
		projectPath, projectHandler := projectv1connect.NewProjectServiceHandler(s.ProjectSvc, s.handlerOptions()...)
		inner.Handle(projectPath, projectHandler)
	}
	if s.AgentSvc != nil {
		agentPath, agentHandler := agentv1connect.NewAgentServiceHandler(s.AgentSvc, s.handlerOptions()...)
		inner.Handle(agentPath, agentHandler)
	}

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
