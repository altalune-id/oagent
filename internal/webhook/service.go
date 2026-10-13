package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/kernel/outbox"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/nanoid"
	"altalune.id/oagent/sealer"
)

// Envelope is the JSON body every delivery of one event carries; it is the public wire contract.
type Envelope struct {
	ID         string         `json:"id"`
	Type       events.Type    `json:"type"`
	APIVersion string         `json:"api_version"`
	CreatedAt  time.Time      `json:"created_at"`
	Tenant     EnvelopeTenant `json:"tenant"`
	Data       any            `json:"data"`
}

// EnvelopeTenant names the org and project an event belongs to, by their public ids and slug.
type EnvelopeTenant struct {
	OrgID       string `json:"org_id"`
	ProjectID   string `json:"project_id"`
	ProjectSlug string `json:"project_slug"`
}

func envelopeType(payload []byte) (events.Type, error) {
	var env struct {
		Type events.Type `json:"type"`
	}
	if err := json.Unmarshal(payload, &env); err != nil {
		return "", fmt.Errorf("decode envelope type: %w", err)
	}
	if env.Type == "" {
		return "", errors.New("decode envelope type: type is empty")
	}
	return env.Type, nil
}

// Service is the webhooks driving port.
type Service struct {
	store      Store
	log        *slog.Logger
	unexpected apperror.UnexpectedFunc
	sealer     sealer.Sealer
	outbox     outbox.Store
	resolver   Resolver

	allowInsecureURL bool
	projects         tenant.ProjectResolver
}

// ServiceOption tunes a Service at construction.
type ServiceOption func(*Service)

// WithAllowInsecureURL lets endpoints use http:// URLs; self-hosted dev only.
func WithAllowInsecureURL() ServiceOption { return func(s *Service) { s.allowInsecureURL = true } }

// WithProjects makes Create refuse a project outside the caller's org.
func WithProjects(r tenant.ProjectResolver) ServiceOption { return func(s *Service) { s.projects = r } }

func (s *Service) validateOpts() []ValidateOption {
	if s.allowInsecureURL {
		return []ValidateOption{AllowInsecureURL()}
	}
	return nil
}

// NewService binds the service to its dependencies.
func NewService(
	store Store, log *slog.Logger, unexpected apperror.UnexpectedFunc,
	sl sealer.Sealer, ob outbox.Store, resolver Resolver, opts ...ServiceOption,
) *Service {
	if log == nil {
		log = slog.Default()
	}
	svc := &Service{
		store:      store,
		log:        log.With("module", "webhook"),
		unexpected: unexpected,
		sealer:     sl,
		outbox:     ob,
		resolver:   resolver,
	}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

// Enqueue fans t out to every active endpoint of the caller's project subscribed to it.
func (s *Service) Enqueue(ctx context.Context, t events.Type, data any) error {
	spec, ok := events.Lookup(t)
	if !ok {
		return &events.UnknownTypeError{Type: t}
	}
	if err := events.CheckPayload(t, data); err != nil {
		return err
	}
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	endpoints, err := s.store.List(ctx, tc.OrgID, tc.ProjectID)
	if err != nil {
		return fmt.Errorf("webhook.Enqueue: %w", err)
	}
	targets := slices.DeleteFunc(endpoints, func(e *Endpoint) bool { return !e.Active || !e.Subscribes(t) })
	if len(targets) == 0 {
		return nil
	}
	if err := s.fanOut(ctx, tc.OrgID, tc.ProjectID, spec, data, targets); err != nil {
		return fmt.Errorf("webhook.Enqueue: %w", err)
	}
	return nil
}

// EnqueueOrg fans an org event out to the subscribed endpoints of every project in orgID, one event per project.
func (s *Service) EnqueueOrg(ctx context.Context, orgID uuid.UUID, t events.Type, data any) error {
	spec, ok := events.Lookup(t)
	if !ok {
		return &events.UnknownTypeError{Type: t}
	}
	if err := events.CheckPayload(t, data); err != nil {
		return err
	}
	endpoints, err := s.store.ListByOrg(ctx, orgID)
	if err != nil {
		return fmt.Errorf("webhook.EnqueueOrg: %w", err)
	}
	var projects []uuid.UUID
	targets := map[uuid.UUID][]*Endpoint{}
	for _, e := range endpoints {
		if !e.Active || !e.Subscribes(t) {
			continue
		}
		if _, seen := targets[e.ProjectID]; !seen {
			projects = append(projects, e.ProjectID)
		}
		targets[e.ProjectID] = append(targets[e.ProjectID], e)
	}
	for _, projectID := range projects {
		scoped := tenant.Into(ctx, tenant.Context{OrgID: orgID, ProjectID: projectID})
		if err := s.fanOut(scoped, orgID, projectID, spec, data, targets[projectID]); err != nil {
			return fmt.Errorf("webhook.EnqueueOrg: %w", err)
		}
	}
	return nil
}

// Create persists a new endpoint in the caller's project and returns its signing secret, shown only this once.
func (s *Service) Create(ctx context.Context, rawURL, description string, types []events.Type) (*Endpoint, string, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, "", err
	}
	if err := tenant.RequireProjectInOrg(ctx, s.projects, tc.OrgID, tc.ProjectID); err != nil {
		if tenant.IsProjectOutsideOrgError(err) {
			return nil, "", err
		}
		return nil, "", s.unexpected(ctx, "webhook.Create: project org", err, "org_id", tc.OrgID, "project_id", tc.ProjectID)
	}
	e, err := New(tc.OrgID, tc.ProjectID, rawURL, description, types, s.validateOpts()...)
	if err != nil {
		return nil, "", err
	}
	existing, err := s.store.List(ctx, tc.OrgID, tc.ProjectID)
	if err != nil {
		return nil, "", s.unexpected(ctx, "webhook.Create: list", err, "org_id", tc.OrgID, "project_id", tc.ProjectID)
	}
	if len(existing) >= MaxEndpointsPerProject {
		return nil, "", &EndpointLimitError{Limit: MaxEndpointsPerProject}
	}
	secret, err := newSecret()
	if err != nil {
		return nil, "", s.unexpected(ctx, "webhook.Create: new secret", err, "endpoint_id", e.ID)
	}
	if e.Secrets.Primary, err = sealSecret(s.sealer, e.ID, slotPrimary, secret); err != nil {
		return nil, "", s.sealFailure(ctx, "webhook.Create: seal", err, e.ID)
	}
	if err := s.save(ctx, "webhook.Create", e); err != nil {
		return nil, "", err
	}
	return e, secret, nil
}

// Update replaces the editable fields of the identified endpoint.
func (s *Service) Update(
	ctx context.Context, id uuid.UUID, rawURL, description string, types []events.Type, active bool,
) (*Endpoint, error) {
	e, err := s.load(ctx, "webhook.Update", id)
	if err != nil {
		return nil, err
	}
	if err := e.Update(rawURL, description, types, active, s.validateOpts()...); err != nil {
		return nil, err
	}
	if err := s.save(ctx, "webhook.Update", e); err != nil {
		return nil, err
	}
	return e, nil
}

// Delete removes the identified endpoint; its pending deliveries settle as no-ops.
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	if _, err := s.load(ctx, "webhook.Delete", id); err != nil {
		return err
	}
	if err := s.store.Delete(ctx, id); err != nil {
		if IsNotFoundError(err) {
			return err
		}
		return s.unexpected(ctx, "webhook.Delete: delete", err, "endpoint_id", id)
	}
	return nil
}

// ByID returns the identified endpoint when it belongs to the caller's tenant scope.
func (s *Service) ByID(ctx context.Context, id uuid.UUID) (*Endpoint, error) {
	return s.load(ctx, "webhook.ByID", id)
}

// List returns the endpoints of the caller's project, newest first.
func (s *Service) List(ctx context.Context) ([]*Endpoint, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	out, err := s.store.List(ctx, tc.OrgID, tc.ProjectID)
	if err != nil {
		return nil, s.unexpected(ctx, "webhook.List: list", err, "org_id", tc.OrgID, "project_id", tc.ProjectID)
	}
	return out, nil
}

// Rotate issues a new primary secret, keeping the old one as the secondary when it can still be opened.
func (s *Service) Rotate(ctx context.Context, id uuid.UUID) (string, error) {
	e, err := s.load(ctx, "webhook.Rotate", id)
	if err != nil {
		return "", err
	}
	secret, err := newSecret()
	if err != nil {
		return "", s.unexpected(ctx, "webhook.Rotate: new secret", err, "endpoint_id", id)
	}
	primary, err := sealSecret(s.sealer, e.ID, slotPrimary, secret)
	if err != nil {
		return "", s.sealFailure(ctx, "webhook.Rotate: seal primary", err, id)
	}
	var secondary []byte
	if old, ok := s.openPrimary(ctx, e); ok {
		if secondary, err = sealSecret(s.sealer, e.ID, slotSecondary, old); err != nil {
			return "", s.sealFailure(ctx, "webhook.Rotate: seal secondary", err, id)
		}
	}
	if err := s.saveSecrets(ctx, "webhook.Rotate", e, SealedSecrets{Primary: primary, Secondary: secondary}); err != nil {
		return "", err
	}
	return secret, nil
}

// RetireSecondary drops the secondary secret, ending a rotation.
func (s *Service) RetireSecondary(ctx context.Context, id uuid.UUID) error {
	e, err := s.load(ctx, "webhook.RetireSecondary", id)
	if err != nil {
		return err
	}
	if e.Secrets.Secondary == nil {
		return nil
	}
	return s.saveSecrets(ctx, "webhook.RetireSecondary", e, SealedSecrets{Primary: e.Secrets.Primary})
}

// SendTest queues one webhook.ping to the identified endpoint, whatever it subscribes to.
func (s *Service) SendTest(ctx context.Context, id uuid.UUID) error {
	e, err := s.load(ctx, "webhook.SendTest", id)
	if err != nil {
		return err
	}
	if !e.Active {
		return &EndpointInactiveError{ID: e.PublicID}
	}
	spec, ok := events.Lookup(events.WebhookPing)
	if !ok {
		err := &events.UnknownTypeError{Type: events.WebhookPing}
		return s.unexpected(ctx, "webhook.SendTest: lookup", err, "endpoint_id", id)
	}
	if err := s.fanOut(ctx, e.OrgID, e.ProjectID, spec, events.WebhookPingV1{EndpointID: e.PublicID}, []*Endpoint{e}); err != nil {
		return s.unexpected(ctx, "webhook.SendTest: enqueue", err, "endpoint_id", id)
	}
	return nil
}

// MaxDeliveriesListed caps one Deliveries call.
const MaxDeliveriesListed = 50

// Delivery is one outbox delivery to an endpoint, with the event type read from its envelope.
type Delivery struct {
	outbox.Entry
	EventType events.Type
}

// Deliveries returns up to limit deliveries of the identified endpoint, newest first.
func (s *Service) Deliveries(ctx context.Context, id uuid.UUID, limit int) ([]Delivery, error) {
	if _, err := s.load(ctx, "webhook.Deliveries", id); err != nil {
		return nil, err
	}
	entries, err := s.outbox.ListByTarget(ctx, id.String(), min(limit, MaxDeliveriesListed))
	if err != nil {
		return nil, s.unexpected(ctx, "webhook.Deliveries: list", err, "endpoint_id", id)
	}
	out := make([]Delivery, len(entries))
	for i, e := range entries {
		out[i] = s.toDelivery(ctx, id, e)
	}
	return out, nil
}

// Attempts returns the recorded attempts of one delivery to the identified endpoint, newest first.
func (s *Service) Attempts(ctx context.Context, id, deliveryID uuid.UUID) ([]Attempt, error) {
	if _, err := s.load(ctx, "webhook.Attempts", id); err != nil {
		return nil, err
	}
	out, err := s.store.ListAttempts(ctx, id, deliveryID)
	if err != nil {
		return nil, s.unexpected(ctx, "webhook.Attempts: list", err, "endpoint_id", id, "delivery_id", deliveryID)
	}
	return out, nil
}

func (s *Service) toDelivery(ctx context.Context, endpointID uuid.UUID, e outbox.Entry) Delivery {
	typ, err := envelopeType(e.Payload)
	if err != nil {
		s.log.WarnContext(ctx, "webhook: delivery envelope has no readable type",
			"endpoint_id", endpointID, "delivery_id", e.ID, "err", err)
		return Delivery{Entry: e}
	}
	return Delivery{Entry: e, EventType: typ}
}

// IDByPublicID returns the id of the caller's project endpoint with publicID, or a *NotFoundError.
func (s *Service) IDByPublicID(ctx context.Context, publicID string) (uuid.UUID, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	e, err := s.store.ByPublicID(ctx, publicID)
	if err != nil {
		if IsNotFoundError(err) {
			return uuid.Nil, err
		}
		return uuid.Nil, s.unexpected(ctx, "webhook.IDByPublicID", err)
	}
	if e.OrgID != tc.OrgID || e.ProjectID != tc.ProjectID {
		return uuid.Nil, &NotFoundError{ID: publicID}
	}
	return e.ID, nil
}

// DeliveryIDByPublicID returns the id of the endpoint's delivery with publicID, or a *DeliveryNotFoundError.
func (s *Service) DeliveryIDByPublicID(ctx context.Context, id uuid.UUID, publicID string) (uuid.UUID, error) {
	if _, err := s.load(ctx, "webhook.DeliveryIDByPublicID", id); err != nil {
		return uuid.Nil, err
	}
	deliveryID, err := s.outbox.IDByPublicID(ctx, publicID, id.String())
	switch {
	case err == nil:
		return deliveryID, nil
	case outbox.IsNotFoundError(err):
		return uuid.Nil, &DeliveryNotFoundError{ID: publicID}
	}
	return uuid.Nil, s.unexpected(ctx, "webhook.DeliveryIDByPublicID", err, "endpoint_id", id)
}

// Retry requeues one failed delivery of the identified endpoint.
func (s *Service) Retry(ctx context.Context, id, deliveryID uuid.UUID) error {
	if _, err := s.load(ctx, "webhook.Retry", id); err != nil {
		return err
	}
	err := s.outbox.Requeue(ctx, deliveryID, id.String())
	switch {
	case err == nil:
		return nil
	case outbox.IsNotFailedError(err):
		return &DeliveryNotRetryableError{ID: deliveryID.String()}
	case outbox.IsNotFoundError(err):
		return &DeliveryNotFoundError{ID: deliveryID.String()}
	}
	return s.unexpected(ctx, "webhook.Retry: requeue", err, "endpoint_id", id, "delivery_id", deliveryID)
}

// RetryFailed requeues every failed delivery of the identified endpoint and returns how many.
func (s *Service) RetryFailed(ctx context.Context, id uuid.UUID) (int, error) {
	if _, err := s.load(ctx, "webhook.RetryFailed", id); err != nil {
		return 0, err
	}
	n, err := s.outbox.RequeueFailed(ctx, id.String())
	if err != nil {
		return 0, s.unexpected(ctx, "webhook.RetryFailed: requeue", err, "endpoint_id", id)
	}
	return n, nil
}

func (s *Service) fanOut(ctx context.Context, orgID, projectID uuid.UUID, spec events.Spec, data any, targets []*Endpoint) error {
	slug, projectPublicID, err := s.resolver.ProjectRef(ctx, projectID)
	if err != nil {
		return fmt.Errorf("project ref: %w", err)
	}
	orgPublicID, err := s.resolver.OrgRef(ctx, orgID)
	if err != nil {
		return fmt.Errorf("org ref: %w", err)
	}
	eventPublicID, err := nanoid.New(21)
	if err != nil {
		return fmt.Errorf("event id: %w", err)
	}
	// NOTE: the outbox dedup key stays an internal uuid; only the envelope exposes the public id.
	eventUUID, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("event id: %w", err)
	}
	body, err := json.Marshal(Envelope{
		ID:         eventPublicID,
		Type:       spec.Type,
		APIVersion: spec.APIVersion(),
		CreatedAt:  time.Now().UTC().Truncate(time.Second),
		Tenant:     EnvelopeTenant{OrgID: orgPublicID, ProjectID: projectPublicID, ProjectSlug: slug},
		Data:       data,
	})
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}
	for _, e := range targets {
		entryID, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("entry id: %w", err)
		}
		entryPublicID, err := nanoid.New(21)
		if err != nil {
			return fmt.Errorf("entry id: %w", err)
		}
		if err := s.outbox.Enqueue(ctx, outbox.Entry{
			ID:            entryID,
			PublicID:      entryPublicID,
			EventID:       eventUUID,
			EventPublicID: eventPublicID,
			OrgID:         orgID,
			ProjectID:     projectID,
			Target:        e.ID.String(),
			Payload:       body,
		}); err != nil {
			return fmt.Errorf("outbox enqueue: endpoint_id=%s: %w", e.ID, err)
		}
	}
	return nil
}

// SECURITY: the store filters by org, so the project check here is what keeps a sibling project's endpoint out of reach.
func (s *Service) load(ctx context.Context, method string, id uuid.UUID) (*Endpoint, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	e, err := s.store.ByID(ctx, id)
	if err != nil {
		if IsNotFoundError(err) {
			return nil, err
		}
		return nil, s.unexpected(ctx, method+": byID", err, "endpoint_id", id)
	}
	if e.OrgID != tc.OrgID || e.ProjectID != tc.ProjectID {
		return nil, &NotFoundError{ID: id.String()}
	}
	return e, nil
}

func (s *Service) save(ctx context.Context, method string, e *Endpoint) error {
	if err := s.store.Save(ctx, e); err != nil {
		if IsNotFoundError(err) {
			return err
		}
		return s.unexpected(ctx, method+": save", err, "endpoint_id", e.ID)
	}
	return nil
}

func (s *Service) saveSecrets(ctx context.Context, method string, e *Endpoint, next SealedSecrets) error {
	err := s.store.SaveSecrets(ctx, e.ID, e.Secrets, next)
	if err == nil {
		e.SetSecrets(next)
		return nil
	}
	if IsNotFoundError(err) || IsSecretConflictError(err) {
		return err
	}
	return s.unexpected(ctx, method+": save secrets", err, "endpoint_id", e.ID)
}

func (s *Service) sealFailure(ctx context.Context, msg string, err error, id uuid.UUID) error {
	if sealer.IsUnavailableError(err) {
		return err
	}
	return s.unexpected(ctx, msg, err, "endpoint_id", id)
}

func (s *Service) openPrimary(ctx context.Context, e *Endpoint) (string, bool) {
	old, err := openSecret(s.sealer, e.ID, slotPrimary, e.Secrets.Primary)
	if err != nil {
		s.log.WarnContext(ctx, "webhook: old primary secret cannot be opened, rotating without a secondary",
			"endpoint_id", e.ID, "err", err)
		return "", false
	}
	return old, true
}
