package webtool

import (
	"context"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/sealer"
)

// Service is the web tool settings application service: the driving port surfaces and runs call.
type Service struct {
	store      Store
	unexpected apperror.UnexpectedFunc
	sealer     sealer.Sealer
	users      Users
}

// OpenProvider is what a run needs to search: the kind, the opened key and the price per call.
type OpenProvider struct {
	Kind               Kind
	APIKey             string
	PricePerCallMicros int64
}

// OpenSecret is one opened HTTP secret for a run. SECURITY: Value is a secret; never log it.
type OpenSecret struct {
	Host, Header, Value string
}

// NewService builds a Service over store; without WithSealer every key operation answers SecretsUnavailableError.
func NewService(store Store, unexpected apperror.UnexpectedFunc) *Service {
	return &Service{store: store, unexpected: unexpected, sealer: sealer.Disabled()}
}

// WithSealer seals and opens keys and secret values with sl.
func (s *Service) WithSealer(sl sealer.Sealer) *Service {
	s.sealer = sl
	return s
}

// WithUsers names the agents that search with a project's provider through u; without it the list is empty.
func (s *Service) WithUsers(u Users) *Service {
	s.users = u
	return s
}

// SearchUsers names the agents of a project whose latest published version has web search on.
func (s *Service) SearchUsers(ctx context.Context, projectID uuid.UUID) ([]UsedBy, error) {
	if s.users == nil {
		return nil, nil
	}
	used, err := s.users.WebSearchUsers(ctx, projectID)
	if err != nil {
		return nil, s.errOrReport(ctx, "webtool: search users", err)
	}
	return used, nil
}

// Provider loads a project's search provider of the org on ctx; nil when it has none.
func (s *Service) Provider(ctx context.Context, projectID uuid.UUID) (*Provider, error) {
	p, err := s.store.Provider(ctx, projectID)
	if IsNotFoundError(err) {
		return nil, nil
	}
	if err != nil {
		return nil, s.errOrReport(ctx, "webtool: provider", err)
	}
	return p, nil
}

// SetProvider sets a project's search provider; a nil apiKey keeps the stored key, which a new provider or a kind change refuses.
func (s *Service) SetProvider(ctx context.Context, projectID uuid.UUID, kind Kind, apiKey *string, price int64) (*Provider, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	if !kind.Available() {
		return nil, &ProviderUnavailableError{Kind: kind}
	}
	key, err := cleanKey(apiKey)
	if err != nil {
		return nil, err
	}
	old, err := s.Provider(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if key == "" && (old == nil || old.Kind != kind) {
		return nil, &SecretRequiredError{}
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	// NOTE: read and save are not one locked unit of work, so two racing edits are last-write-wins.
	p := &Provider{OrgID: tc.OrgID, ProjectID: projectID, Kind: kind, PricePerCallMicros: price, CreatedAt: now, UpdatedAt: now}
	if old != nil {
		p.APIKeySealed, p.CreatedAt = old.APIKeySealed, old.CreatedAt
	}
	if key != "" {
		if p.APIKeySealed, err = sealSearchKey(s.sealer, tc.OrgID, projectID, key); err != nil {
			return nil, s.errOrReport(ctx, "webtool: seal search key", err)
		}
	}
	if err := s.store.SaveProvider(ctx, p); err != nil {
		return nil, s.errOrReport(ctx, "webtool: set provider", err)
	}
	return p, nil
}

// DeleteProvider removes a project's search provider; a project with none is not an error.
func (s *Service) DeleteProvider(ctx context.Context, projectID uuid.UUID) error {
	if err := s.store.DeleteProvider(ctx, projectID); err != nil {
		return s.errOrReport(ctx, "webtool: delete provider", err)
	}
	return nil
}

// Secrets lists a project's HTTP secrets by host, then header.
func (s *Service) Secrets(ctx context.Context, projectID uuid.UUID) ([]*HTTPSecret, error) {
	list, err := s.store.Secrets(ctx, projectID)
	if err != nil {
		return nil, s.errOrReport(ctx, "webtool: secrets", err)
	}
	return list, nil
}

// SetSecret stores value for host and header in a project, replacing a stored one, up to MaxHTTPSecrets.
func (s *Service) SetSecret(ctx context.Context, projectID uuid.UUID, host, header, value string) (*HTTPSecret, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	sec, err := NewHTTPSecret(tc.OrgID, projectID, host, header, time.Now())
	if err != nil {
		return nil, err
	}
	if value == "" || strings.ContainsFunc(value, unicode.IsControl) {
		return nil, apperror.Invalid("value", apperror.FieldInvalid)
	}
	// NOTE: the cap is soft; two concurrent new keys at 49 can both land.
	total, exists, err := s.store.SecretCount(ctx, projectID, sec.Host, sec.Header)
	if err != nil {
		return nil, s.errOrReport(ctx, "webtool: count secrets", err)
	}
	if !exists && total >= MaxHTTPSecrets {
		return nil, &SecretLimitError{}
	}
	if sec.ValueSealed, err = sealSecret(s.sealer, tc.OrgID, projectID, sec.Host, sec.Header, value); err != nil {
		return nil, s.errOrReport(ctx, "webtool: seal secret", err)
	}
	saved, err := s.store.SaveSecret(ctx, sec)
	if err != nil {
		return nil, s.errOrReport(ctx, "webtool: set secret", err)
	}
	return saved, nil
}

// ResolveSecret names a secret's row and owners from its public id; an unknown one is a *NotFoundError.
func (s *Service) ResolveSecret(ctx context.Context, publicID string) (Ref, error) {
	r, err := s.store.ResolveSecret(ctx, publicID)
	if err != nil {
		return Ref{}, s.errOrReport(ctx, "webtool: resolve secret", err)
	}
	return r, nil
}

// DeleteSecret removes a secret of the org on ctx by id.
func (s *Service) DeleteSecret(ctx context.Context, id uuid.UUID) error {
	if err := s.store.DeleteSecret(ctx, id); err != nil {
		return s.errOrReport(ctx, "webtool: delete secret", err)
	}
	return nil
}

// OpenSearchKey opens a project's search key for a run; no provider, or one whose kind is no longer offered, is a *NotFoundError.
func (s *Service) OpenSearchKey(ctx context.Context, projectID uuid.UUID) (OpenProvider, error) {
	p, err := s.store.Provider(ctx, projectID)
	if err != nil {
		return OpenProvider{}, s.errOrReport(ctx, "webtool: open search key", err)
	}
	if !p.Kind.Available() {
		return OpenProvider{}, &NotFoundError{}
	}
	key, err := openSearchKey(s.sealer, p.OrgID, p.ProjectID, p.APIKeySealed)
	if err != nil && !IsSecretsUnavailableError(err) {
		err = &SearchKeyUnsealableError{Cause: err}
	}
	if err != nil {
		return OpenProvider{}, err
	}
	return OpenProvider{Kind: p.Kind, APIKey: key, PricePerCallMicros: p.PricePerCallMicros}, nil
}

// OpenSecrets opens a project's HTTP secrets for hosts only; a value that will not open is skipped and counted.
func (s *Service) OpenSecrets(ctx context.Context, projectID uuid.UUID, hosts []string) (open []OpenSecret, unsealable int, err error) {
	list, err := s.store.SecretsForHosts(ctx, projectID, hosts)
	if err != nil {
		return nil, 0, s.errOrReport(ctx, "webtool: open secrets", err)
	}
	for _, sec := range list {
		v, err := openSecret(s.sealer, sec.OrgID, sec.ProjectID, sec.Host, sec.Header, sec.ValueSealed)
		if err != nil {
			unsealable++
			continue
		}
		open = append(open, OpenSecret{Host: sec.Host, Header: sec.Header, Value: v})
	}
	return open, unsealable, nil
}

func cleanKey(apiKey *string) (string, error) {
	if apiKey == nil {
		return "", nil
	}
	key := strings.TrimSpace(*apiKey)
	if key == "" || strings.ContainsFunc(key, unicode.IsControl) {
		return "", &SecretRequiredError{}
	}
	return key, nil
}

func (s *Service) errOrReport(ctx context.Context, message string, err error) error {
	if IsExpectedError(err) || s.unexpected == nil {
		return err
	}
	return s.unexpected(ctx, message, err)
}
