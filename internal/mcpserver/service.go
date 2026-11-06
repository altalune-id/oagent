package mcpserver

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/sealer"
)

// Service is the MCP server application service: the driving port surfaces call.
type Service struct {
	store            Store
	uow              db.Transactor
	unexpected       apperror.UnexpectedFunc
	sealer           sealer.Sealer
	usage            Usage
	projects         tenant.ProjectResolver
	prober           prober
	allowInsecureURL bool
}

// NewService builds a Service over store; it seals nothing until WithSealer, and its probe refuses private hosts.
func NewService(store Store, uow db.Transactor, unexpected apperror.UnexpectedFunc) *Service {
	return &Service{store: store, uow: uow, unexpected: unexpected, sealer: sealer.Disabled(), prober: newProber()}
}

// WithSealer seals and opens credentials with sl.
func (s *Service) WithSealer(sl sealer.Sealer) *Service {
	s.sealer = sl
	return s
}

// WithUsage makes Delete refuse a server a published agent version uses; nil skips the check.
func (s *Service) WithUsage(u Usage) *Service {
	s.usage = u
	return s
}

// WithProjects makes Create refuse a project outside the org on ctx; nil skips the check.
func (s *Service) WithProjects(r tenant.ProjectResolver) *Service {
	s.projects = r
	return s
}

// WithAllowInsecureURL lets servers use http:// URLs; self-hosted dev only.
func (s *Service) WithAllowInsecureURL() *Service {
	s.allowInsecureURL = true
	return s
}

func (s *Service) validateOpts() []ValidateOption {
	if s.allowInsecureURL {
		return []ValidateOption{AllowInsecureURL()}
	}
	return nil
}

// Create validates and stores a server in the org on ctx, sealing authValue unless the server needs no auth.
func (s *Service) Create(ctx context.Context, projectID uuid.UUID, in Input, authValue string) (*Server, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	if err := tenant.RequireProjectInOrg(ctx, s.projects, tc.OrgID, projectID); err != nil {
		if tenant.IsProjectOutsideOrgError(err) {
			return nil, err
		}
		return nil, s.errOrReport(ctx, "mcpserver: project org", err)
	}
	srv, err := New(tc.OrgID, projectID, in, time.Now(), s.validateOpts()...)
	if err != nil {
		return nil, err
	}
	if srv.AuthKind != AuthNone {
		value, err := cleanAuthValue(authValue)
		if err != nil {
			return nil, err
		}
		if err := s.seal(srv, value); err != nil {
			return nil, s.errOrReport(ctx, "mcpserver: create seal", err)
		}
	}
	if err := s.store.Save(ctx, srv); err != nil {
		return nil, s.errOrReport(ctx, "mcpserver: create", err)
	}
	return srv, nil
}

// Update replaces a server's editable fields but its name; a nil authValue keeps the credential, which a URL or auth kind change refuses.
func (s *Service) Update(ctx context.Context, id uuid.UUID, in Input, authValue *string) (*Server, error) {
	var value *string
	if authValue != nil {
		v, err := cleanAuthValue(*authValue)
		if err != nil {
			return nil, err
		}
		value = &v
	}
	var srv *Server
	err := s.uow.RunInTx(ctx, func(ctx context.Context) error {
		var err error
		if srv, err = s.store.LockForUpdate(ctx, id); err != nil {
			return err
		}
		oldKind := srv.AuthKind
		urlChanged, err := srv.Update(in, time.Now(), s.validateOpts()...)
		if err != nil {
			return err
		}
		if srv.AuthKind != AuthNone {
			if value == nil && (urlChanged || srv.AuthKind != oldKind) {
				return &SecretRequiredError{Reason: SecretValueRequired}
			}
			if value != nil {
				if err := s.seal(srv, *value); err != nil {
					return err
				}
			}
		}
		return s.store.Save(ctx, srv)
	})
	if err != nil {
		return nil, s.errOrReport(ctx, "mcpserver: update", err)
	}
	return srv, nil
}

// Delete removes a server no published agent version uses.
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	err := s.uow.RunInTx(ctx, func(ctx context.Context) error {
		if _, err := s.store.LockForUpdate(ctx, id); err != nil {
			return err
		}
		if s.usage != nil {
			inUse, err := s.usage.ServerInUse(ctx, id)
			if err != nil {
				return err
			}
			if inUse {
				return &InUseError{}
			}
		}
		return s.store.Delete(ctx, id)
	})
	if err != nil {
		return s.errOrReport(ctx, "mcpserver: delete", err)
	}
	return nil
}

// Get loads a server by id.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Server, error) {
	srv, err := s.store.ByID(ctx, id)
	if err != nil {
		return nil, s.errOrReport(ctx, "mcpserver: get", err)
	}
	return srv, nil
}

// List returns a project's servers.
func (s *Service) List(ctx context.Context, projectID uuid.UUID) ([]*Server, error) {
	srvs, err := s.store.List(ctx, projectID)
	if err != nil {
		return nil, s.errOrReport(ctx, "mcpserver: list", err)
	}
	return srvs, nil
}

// Resolve names publicID's server and its org, in any org; the caller checks reach before it reads the server.
func (s *Service) Resolve(ctx context.Context, publicID string) (Ref, error) {
	r, err := s.store.Resolve(ctx, publicID)
	if err != nil {
		return Ref{}, s.errOrReport(ctx, "mcpserver: resolve", err)
	}
	return r, nil
}

// SECURITY: a CR, LF or other control character in a header value would let a tenant inject headers into oagent's requests.
func cleanAuthValue(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" || strings.ContainsFunc(v, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return "", &SecretRequiredError{Reason: SecretValueRequired}
	}
	return v, nil
}

func (s *Service) seal(srv *Server, value string) error {
	sealed, err := sealAuth(s.sealer, srv.OrgID, srv.ID, value)
	if err != nil {
		return secretsErr(err)
	}
	srv.AuthSealed = sealed
	return nil
}

func secretsErr(err error) error {
	if sealer.IsUnavailableError(err) {
		return &SecretsUnavailableError{Cause: err}
	}
	return err
}

func (s *Service) errOrReport(ctx context.Context, message string, err error) error {
	if IsExpectedError(err) {
		return err
	}
	return s.unexpected(ctx, message, err)
}
