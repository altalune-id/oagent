package modelprovider

import (
	"context"
	"maps"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/sealer"
)

// Service is the model provider application service: the driving port surfaces call.
type Service struct {
	store            Store
	uow              db.Transactor
	unexpected       apperror.UnexpectedFunc
	sealer           sealer.Sealer
	usage            Usage
	projects         tenant.ProjectResolver
	allowInsecureURL bool
}

// Resolved is what a run needs to call one model of a provider.
type Resolved struct {
	Kind            Kind
	BaseURL, APIKey string
	Model           Model
}

// NewService builds a Service over store; without WithSealer every key operation answers SecretsUnavailableError.
func NewService(store Store, uow db.Transactor, unexpected apperror.UnexpectedFunc) *Service {
	return &Service{store: store, uow: uow, unexpected: unexpected, sealer: sealer.Disabled()}
}

// WithSealer seals and opens API keys with sl.
func (s *Service) WithSealer(sl sealer.Sealer) *Service {
	s.sealer = sl
	return s
}

// WithUsage makes Update and Delete refuse a change that breaks a published agent version; nil skips the check.
func (s *Service) WithUsage(u Usage) *Service {
	s.usage = u
	return s
}

// WithProjects makes Create refuse a project outside the org on ctx; nil skips the check.
func (s *Service) WithProjects(r tenant.ProjectResolver) *Service {
	s.projects = r
	return s
}

// WithAllowInsecureURL lets providers use http:// base URLs; self-hosted dev only.
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

// Create validates and stores a provider in the org on ctx, sealing apiKey.
func (s *Service) Create(ctx context.Context, projectID uuid.UUID, in Input, apiKey string) (*Provider, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	if err := tenant.RequireProjectInOrg(ctx, s.projects, tc.OrgID, projectID); err != nil {
		if tenant.IsProjectOutsideOrgError(err) {
			return nil, err
		}
		return nil, s.errOrReport(ctx, "modelprovider: project org", err)
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" || strings.ContainsFunc(apiKey, unicode.IsControl) {
		return nil, &SecretRequiredError{}
	}
	p, err := New(tc.OrgID, projectID, in, time.Now(), s.validateOpts()...)
	if err != nil {
		return nil, err
	}
	if err := s.seal(p, apiKey); err != nil {
		return nil, s.errOrReport(ctx, "modelprovider: create seal", err)
	}
	if err := s.store.Save(ctx, p); err != nil {
		return nil, s.errOrReport(ctx, "modelprovider: create", err)
	}
	return p, nil
}

// Update replaces a provider's editable fields; a nil apiKey keeps the sealed key, which a base URL change refuses.
func (s *Service) Update(ctx context.Context, id uuid.UUID, in Input, apiKey *string) (*Provider, error) {
	if apiKey != nil {
		trimmed := strings.TrimSpace(*apiKey)
		if trimmed == "" || strings.ContainsFunc(trimmed, unicode.IsControl) {
			return nil, &SecretRequiredError{}
		}
		apiKey = &trimmed
	}
	var p *Provider
	err := s.uow.RunInTx(ctx, func(ctx context.Context) error {
		var err error
		if p, err = s.store.LockForUpdate(ctx, id); err != nil {
			return err
		}
		urlChanged, err := p.Update(in, time.Now(), s.validateOpts()...)
		if err != nil {
			return err
		}
		if urlChanged && apiKey == nil {
			return &SecretRequiredError{}
		}
		if err := s.checkModelsInUse(ctx, p); err != nil {
			return err
		}
		if apiKey != nil {
			if err := s.seal(p, *apiKey); err != nil {
				return err
			}
		}
		return s.store.Save(ctx, p)
	})
	if err != nil {
		return nil, s.errOrReport(ctx, "modelprovider: update", err)
	}
	return p, nil
}

func (s *Service) checkModelsInUse(ctx context.Context, p *Provider) error {
	if s.usage == nil {
		return nil
	}
	use, err := s.usage.ProviderUse(ctx, p.ID)
	if err != nil {
		return err
	}
	for _, id := range slices.Sorted(maps.Keys(use.Models)) {
		m, ok := p.Model(id)
		if !ok {
			return &InUseError{Reason: InUseModelRemoved, Model: id}
		}
		if use.Models[id] && m.Prices == nil {
			return &InUseError{Reason: InUsePriceRemoved, Model: id}
		}
	}
	return nil
}

// Delete removes a provider no published agent version uses.
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	err := s.uow.RunInTx(ctx, func(ctx context.Context) error {
		if _, err := s.store.LockForUpdate(ctx, id); err != nil {
			return err
		}
		if s.usage != nil {
			use, err := s.usage.ProviderUse(ctx, id)
			if err != nil {
				return err
			}
			if use.InUse {
				return &InUseError{Reason: InUseDelete}
			}
		}
		return s.store.Delete(ctx, id)
	})
	if err != nil {
		return s.errOrReport(ctx, "modelprovider: delete", err)
	}
	return nil
}

// Get loads a provider by id.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Provider, error) {
	p, err := s.store.ByID(ctx, id)
	if err != nil {
		return nil, s.errOrReport(ctx, "modelprovider: get", err)
	}
	return p, nil
}

// List returns a project's providers.
func (s *Service) List(ctx context.Context, projectID uuid.UUID) ([]*Provider, error) {
	ps, err := s.store.List(ctx, projectID)
	if err != nil {
		return nil, s.errOrReport(ctx, "modelprovider: list", err)
	}
	return ps, nil
}

// Resolve names publicID's provider and its org, in any org; the caller checks reach before it reads the provider.
func (s *Service) Resolve(ctx context.Context, publicID string) (Ref, error) {
	r, err := s.store.Resolve(ctx, publicID)
	if err != nil {
		return Ref{}, s.errOrReport(ctx, "modelprovider: resolve", err)
	}
	return r, nil
}

// ResolveModel returns the base URL, opened API key and model a run calls. SECURITY: never log the key or return it to a client.
func (s *Service) ResolveModel(ctx context.Context, id uuid.UUID, model string) (Resolved, error) {
	p, err := s.store.ByID(ctx, id)
	if err != nil {
		return Resolved{}, s.errOrReport(ctx, "modelprovider: resolve model", err)
	}
	m, ok := p.Model(model)
	if !ok {
		return Resolved{}, &UnknownModelError{Model: model}
	}
	key, err := openKey(s.sealer, p.OrgID, p.ID, p.APIKeySealed)
	if err != nil {
		return Resolved{}, s.errOrReport(ctx, "modelprovider: resolve model open", secretsErr(err))
	}
	return Resolved{Kind: p.Kind, BaseURL: p.BaseURL, APIKey: key, Model: m}, nil
}

func (s *Service) seal(p *Provider, apiKey string) error {
	sealed, err := sealKey(s.sealer, p.OrgID, p.ID, apiKey)
	if err != nil {
		return secretsErr(err)
	}
	p.APIKeySealed = sealed
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
