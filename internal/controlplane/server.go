// Package controlplane wires the Connect-RPC handlers into an http.Handler.
package controlplane

import (
	"context"
	"fmt"
	"net/http"

	"connectrpc.com/connect"

	"altalune.id/oagent/apperror"
	agentv1connect "altalune.id/oagent/gen/go/agent/v1/agentv1connect"
	apikeyv1connect "altalune.id/oagent/gen/go/apikey/v1/apikeyv1connect"
	authv1connect "altalune.id/oagent/gen/go/auth/v1/authv1connect"
	invitev1connect "altalune.id/oagent/gen/go/invite/v1/invitev1connect"
	memberv1connect "altalune.id/oagent/gen/go/member/v1/memberv1connect"
	orgv1connect "altalune.id/oagent/gen/go/org/v1/orgv1connect"
	projectv1connect "altalune.id/oagent/gen/go/project/v1/projectv1connect"
	webhookv1connect "altalune.id/oagent/gen/go/webhook/v1/webhookv1connect"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/apikey"
	"altalune.id/oagent/internal/auth"
	"altalune.id/oagent/internal/kernel/authn"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/org/invite"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/user"
	"altalune.id/oagent/internal/webhook"
)

// Options configures the optional parts of the controlplane surface.
type Options struct {
	OpenAPIEnabled   bool
	OpenAPIBasicAuth *BasicAuth
	Authn            authn.InterceptorOptions
	// Validator checks every request after authn; nil uses protovalidate's shared one.
	Validator *Validator
}

// Services are the domain services the controlplane exposes.
type Services struct {
	Orgs     *org.Service
	Projects *project.Service
	Agents   *agent.Service
	APIKeys  *apikey.Service
	Webhooks *webhook.Service
	Users    *user.Service
	Members  *member.Service
	Invites  *invite.Service
	Auth     *auth.Service
	Refs     Refs
}

// Refs turns the public ids every surface carries into ids and back, for every domain's handler.
type Refs interface {
	invite.OrgIDs
	project.OrgRefs
	agent.Refs
	apikey.Refs
	webhook.Refs
	member.Refs
	auth.PublicIDs
}

// Server holds the wired Connect handlers.
type Server struct {
	Orgs     *org.Service
	Projects *project.Service
	Agents   *agent.Service
	APIKeys  *apikey.Service
	Webhooks *webhook.Service

	OrgSvc     *org.OrgService
	ProjectSvc *project.ProjectService
	AgentSvc   *agent.AgentService
	APIKeySvc  *apikey.APIKeyService
	WebhookSvc *webhook.WebhookControlPlane
	AuthSvc    *auth.AuthService
	MemberSvc  *member.MemberService
	InviteSvc  *invite.InviteService

	OpenAPIEnabled   bool
	OpenAPIBasicAuth *BasicAuth
	authn            authn.InterceptorOptions
	validator        *Validator

	unexpected apperror.UnexpectedFunc
}

// New builds a Server.
func New(svcs Services, unexpected apperror.UnexpectedFunc, opts Options) *Server {
	s := &Server{
		Orgs:             svcs.Orgs,
		Projects:         svcs.Projects,
		Agents:           svcs.Agents,
		APIKeys:          svcs.APIKeys,
		Webhooks:         svcs.Webhooks,
		OrgSvc:           org.NewControlPlane(svcs.Orgs),
		OpenAPIEnabled:   opts.OpenAPIEnabled,
		OpenAPIBasicAuth: opts.OpenAPIBasicAuth,
		authn:            opts.Authn,
		validator:        opts.Validator,
		unexpected:       unexpected,
	}
	if svcs.Projects != nil {
		s.ProjectSvc = project.NewControlPlane(svcs.Projects, svcs.Refs)
	}
	if svcs.Agents != nil {
		s.AgentSvc = agent.NewControlPlane(svcs.Agents, svcs.Refs)
	}
	if svcs.APIKeys != nil {
		s.APIKeySvc = apikey.NewControlPlane(svcs.APIKeys, svcs.Refs)
	}
	if svcs.Webhooks != nil {
		s.WebhookSvc = webhook.NewControlPlane(svcs.Webhooks, svcs.Refs)
	}
	if svcs.Members != nil {
		s.MemberSvc = member.NewControlPlane(svcs.Members, svcs.Refs)
	}
	if svcs.Invites != nil {
		s.InviteSvc = invite.NewControlPlane(svcs.Invites, svcs.Refs)
	}
	if svcs.Orgs != nil && svcs.Projects != nil && svcs.Users != nil {
		s.AuthSvc = auth.NewControlPlane(svcs.Orgs, svcs.Projects, svcs.Users).WithProfiles(svcs.Auth)
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
	if s.APIKeySvc != nil {
		keyPath, keyHandler := apikeyv1connect.NewAPIKeyServiceHandler(s.APIKeySvc, s.handlerOptions()...)
		inner.Handle(keyPath, keyHandler)
	}
	if s.WebhookSvc != nil {
		hookPath, hookHandler := webhookv1connect.NewWebhookServiceHandler(s.WebhookSvc, s.handlerOptions()...)
		inner.Handle(hookPath, hookHandler)
	}
	if s.MemberSvc != nil {
		memberPath, memberHandler := memberv1connect.NewMemberServiceHandler(s.MemberSvc, s.handlerOptions()...)
		inner.Handle(memberPath, memberHandler)
	}
	if s.InviteSvc != nil {
		invitePath, inviteHandler := invitev1connect.NewInviteServiceHandler(s.InviteSvc, s.handlerOptions()...)
		inner.Handle(invitePath, inviteHandler)
	}

	if s.AuthSvc != nil {
		authPath, authHandler := authv1connect.NewAuthServiceHandler(s.AuthSvc, s.handlerOptions()...)
		inner.Handle(authPath, authHandler)
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
	ics = append(ics, Wrap(s.unexpected), authn.Interceptor(s.authn), tenantFromPrincipal(), s.validator.interceptor())
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
