package channel

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/scheduler"
	"altalune.id/oagent/sealer"
)

// Service manages channel connections: their settings, sealed secrets and events.
type Service struct {
	store        Store
	uow          db.Transactor
	unexpected   apperror.UnexpectedFunc
	sealer       sealer.Sealer
	publisher    Publisher
	refs         Refs
	allowPrivate bool
	log          *slog.Logger
}

// NewService builds a Service over store that names projects and agents in events through refs; it seals nothing until WithSealer.
func NewService(store Store, uow db.Transactor, refs Refs, unexpected apperror.UnexpectedFunc) *Service {
	if refs == nil {
		panic("channel: NewService needs Refs")
	}
	return &Service{store: store, uow: uow, refs: refs, unexpected: unexpected, sealer: sealer.Disabled(), log: slog.Default()}
}

// WithSealer seals and opens secrets with sl.
func (s *Service) WithSealer(sl sealer.Sealer) *Service {
	s.sealer = sl
	return s
}

// WithPublisher emits every write's event through p, inside the write's unit of work.
func (s *Service) WithPublisher(p Publisher) *Service {
	s.publisher = p
	return s
}

// WithAllowPrivateHosts lets base URLs use http, for an operator who runs OpenWA on a private network.
func (s *Service) WithAllowPrivateHosts(allow bool) *Service {
	s.allowPrivate = allow
	return s
}

// WithLogger logs through l.
func (s *Service) WithLogger(l *slog.Logger) *Service {
	s.log = l
	return s
}

// Create stores agentID's connection with its device key sealed and emits its event; SetSecret adds the signing secret.
func (s *Service) Create(ctx context.Context, projectID, agentID uuid.UUID, in Input, deviceKey string) (*Connection, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	deviceKey = strings.TrimSpace(deviceKey)
	if deviceKey == "" {
		return nil, &SecretRequiredError{}
	}
	c, err := New(tc.OrgID, projectID, agentID, in, time.Now(), AllowPrivateHosts(s.allowPrivate))
	if err != nil {
		return nil, err
	}
	if c.DeviceKeySealed, err = seal(s.sealer, c, slotDeviceKey, deviceKey); err != nil {
		return nil, s.errOrReport(ctx, "channel: seal", &SecretsUnavailableError{Cause: err})
	}
	err = s.uow.RunInTx(ctx, func(ctx context.Context) error {
		if err := s.store.Insert(ctx, c); err != nil {
			return err
		}
		return s.publish(ctx, events.ChannelConnectionCreated, c)
	})
	if err != nil {
		return nil, s.errOrReport(ctx, "channel: create", err)
	}
	return c, nil
}

// Get loads a connection of the bound org.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Connection, error) {
	c, err := s.store.ByID(ctx, id)
	return c, s.errOrReport(ctx, "channel: get", err)
}

// List lists a project's connections.
func (s *Service) List(ctx context.Context, projectID uuid.UUID) ([]*Connection, error) {
	out, err := s.store.List(ctx, projectID)
	return out, s.errOrReport(ctx, "channel: list", err)
}

// Update replaces a connection's settings; a nil deviceKey keeps the stored key, which a new base URL or device refuses.
func (s *Service) Update(ctx context.Context, id uuid.UUID, in Input, deviceKey *string) (*Connection, error) {
	return s.change(ctx, id, func(c *Connection) error {
		oldBase, oldDevice := c.BaseURL, c.DeviceID
		if err := c.Apply(in, time.Now(), AllowPrivateHosts(s.allowPrivate)); err != nil {
			return err
		}
		// SECURITY: the stored key never goes to a new address; whoever moves the connection supplies the key for it.
		if deviceKey == nil && (c.BaseURL != oldBase || c.DeviceID != oldDevice) {
			return &SecretRequiredError{}
		}
		if deviceKey == nil {
			return nil
		}
		key := strings.TrimSpace(*deviceKey)
		if key == "" {
			return &SecretRequiredError{}
		}
		sealed, err := seal(s.sealer, c, slotDeviceKey, key)
		if err != nil {
			return &SecretsUnavailableError{Cause: err}
		}
		c.DeviceKeySealed = sealed
		return nil
	})
}

// SetSecret makes newSecret the primary signing secret; the old one stays as secondary unless dropPrevious.
func (s *Service) SetSecret(ctx context.Context, id uuid.UUID, newSecret string, dropPrevious bool) (*Connection, error) {
	newSecret = strings.TrimSpace(newSecret)
	if newSecret == "" {
		return nil, &SecretRequiredError{}
	}
	return s.change(ctx, id, func(c *Connection) error {
		c.SigningSecondarySealed = nil
		if len(c.SigningPrimarySealed) > 0 && !dropPrevious {
			old, err := open(s.sealer, c, slotPrimary, c.SigningPrimarySealed)
			if err != nil {
				return err
			}
			if c.SigningSecondarySealed, err = seal(s.sealer, c, slotSecondary, old); err != nil {
				return &SecretsUnavailableError{Cause: err}
			}
		}
		var err error
		if c.SigningPrimarySealed, err = seal(s.sealer, c, slotPrimary, newSecret); err != nil {
			return &SecretsUnavailableError{Cause: err}
		}
		c.UpdatedAt = time.Now().UTC()
		return nil
	})
}

// NOTE: one unit of work, so the change and its event commit together or not at all.
func (s *Service) change(ctx context.Context, id uuid.UUID, edit func(*Connection) error) (*Connection, error) {
	var out *Connection
	err := s.uow.RunInTx(ctx, func(ctx context.Context) error {
		c, err := s.store.ByID(ctx, id)
		if err != nil {
			return err
		}
		if err := edit(c); err != nil {
			return err
		}
		if err := s.store.Update(ctx, c); err != nil {
			return err
		}
		out = c
		return s.publish(ctx, events.ChannelConnectionUpdated, c)
	})
	if err != nil {
		return nil, s.errOrReport(ctx, "channel: update", err)
	}
	return out, nil
}

// Delete removes a connection and its deliveries, detaches its threads, and emits its deleted event.
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	err := s.uow.RunInTx(ctx, func(ctx context.Context) error {
		c, err := s.store.ByID(ctx, id)
		if err != nil {
			return err
		}
		if err := s.store.Delete(ctx, id); err != nil {
			return err
		}
		return s.publish(ctx, events.ChannelConnectionDeleted, c)
	})
	return s.errOrReport(ctx, "channel: delete", err)
}

// Resolve finds a connection by public id with no tenant bound, then loads it inside its own tenant.
func (s *Service) Resolve(ctx context.Context, publicID string) (*Connection, error) {
	ref, err := s.store.Resolve(ctx, publicID)
	if err != nil {
		return nil, s.errOrReport(ctx, "channel: resolve", err)
	}
	ctx = tenant.Into(ctx, tenant.Context{OrgID: ref.OrgID, ProjectID: ref.ProjectID})
	return s.Get(ctx, ref.ID)
}

// Open loads an active connection of the bound org with its opened secrets; a disabled one is a *NotFoundError.
func (s *Service) Open(ctx context.Context, id uuid.UUID) (*Connection, Secrets, error) {
	c, err := s.Get(ctx, id)
	if err != nil {
		return nil, Secrets{}, err
	}
	if c.Status != StatusActive {
		return nil, Secrets{}, &NotFoundError{}
	}
	secrets, err := s.OpenSecrets(ctx, c)
	return c, secrets, err
}

// Record stores a verified provider outcome as the connection's last result; the same result within a minute changes nothing.
func (s *Service) Record(ctx context.Context, id uuid.UUID, result string) error {
	return s.store.RecordResult(ctx, id, result, time.Now())
}

// OpenSecrets opens a connection's signing secrets, newest first, and its device key.
func (s *Service) OpenSecrets(ctx context.Context, c *Connection) (Secrets, error) {
	var out Secrets
	var err error
	if len(c.SigningPrimarySealed) > 0 {
		primary, perr := open(s.sealer, c, slotPrimary, c.SigningPrimarySealed)
		if perr != nil {
			return Secrets{}, s.errOrReport(ctx, "channel: open secrets", perr)
		}
		out.Signing = []string{primary}
	}
	if len(c.SigningSecondarySealed) > 0 {
		secondary, err := open(s.sealer, c, slotSecondary, c.SigningSecondarySealed)
		if err != nil {
			return Secrets{}, s.errOrReport(ctx, "channel: open secrets", err)
		}
		out.Signing = append(out.Signing, secondary)
	}
	if out.DeviceKey, err = s.deviceKey(ctx, c); err != nil {
		return Secrets{}, s.errOrReport(ctx, "channel: open secrets", err)
	}
	return out, nil
}

// SECURITY: the only place a connection's device key is opened; every provider call asks here, each time.
func (s *Service) deviceKey(_ context.Context, c *Connection) (string, error) {
	return open(s.sealer, c, slotDeviceKey, c.DeviceKeySealed)
}

func (s *Service) publish(ctx context.Context, t events.Type, c *Connection) error {
	if s.publisher == nil {
		return nil
	}
	ctx = tenant.Into(ctx, tenant.Context{OrgID: c.OrgID, ProjectID: c.ProjectID})
	payload := events.ChannelConnectionV1{ID: c.PublicID, Provider: c.Provider, Status: c.ReportedStatus()}
	var err error
	if payload.ProjectID, err = s.refs.ProjectPublicID(ctx, c.ProjectID); err != nil {
		return fmt.Errorf("channel: publish %s: %w", t, err)
	}
	if payload.AgentID, _, err = s.refs.ResourcePublicID(ctx, c.OrgID, c.AgentID); err != nil {
		return fmt.Errorf("channel: publish %s: %w", t, err)
	}
	var data any
	switch t {
	case events.ChannelConnectionCreated:
		data = events.ChannelConnectionCreatedV1(payload)
	case events.ChannelConnectionDeleted:
		data = events.ChannelConnectionDeletedV1(payload)
	default:
		data = events.ChannelConnectionUpdatedV1(payload)
	}
	if err := s.publisher.Enqueue(ctx, t, data); err != nil {
		return fmt.Errorf("channel: publish %s: %w", t, err)
	}
	return nil
}

func (s *Service) errOrReport(ctx context.Context, message string, err error) error {
	if err == nil || IsNotFoundError(err) || IsInvalidError(err) || IsSecretRequiredError(err) {
		return err
	}
	if IsSecretsUnavailableError(err) || IsProviderUnavailableError(err) {
		s.log.ErrorContext(ctx, message, "err", err.Error())
		return err
	}
	if s.unexpected == nil {
		return err
	}
	return s.unexpected(ctx, message, err)
}

const (
	pruneJobName  = "ingest-prune"
	pruneEvery    = time.Hour
	deliveryKeep  = 48 * time.Hour
	pruneDeadline = time.Minute
)

// SchedulerJobs returns the per-tenant job ingest-prune, which drops delivery ids older than 48 h every hour.
func (s *Service) SchedulerJobs() []scheduler.Job {
	return []scheduler.Job{{
		Name: pruneJobName, Scope: scheduler.ScopeTenant,
		Schedule: scheduler.MustEveryInterval(pruneEvery, pruneEvery/10), Timeout: pruneDeadline,
		Run: func(ctx context.Context) error {
			return s.store.PruneDeliveries(ctx, time.Now().Add(-deliveryKeep))
		},
	}}
}
