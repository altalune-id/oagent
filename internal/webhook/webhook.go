// Package webhook is the outbound webhooks bounded context: tenant endpoints and their delivery attempts.
package webhook

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/nanoid"
)

// MaxEndpointsPerProject bounds the fan-out of one event.
const MaxEndpointsPerProject = 10

// MaxURLLen bounds an endpoint URL, in bytes.
const MaxURLLen = 2048

// MaxDescriptionRunes bounds an endpoint description.
const MaxDescriptionRunes = 200

// Delivery header names a receiver reads.
const (
	HeaderEventID    = "X-Oagent-Event-Id"
	HeaderEventType  = "X-Oagent-Event-Type"
	HeaderDeliveryID = "X-Oagent-Delivery-Id"
	HeaderTimestamp  = "X-Oagent-Timestamp"
	HeaderSignature  = "X-Oagent-Signature"
)

// Endpoint is the aggregate root: one tenant URL subscribed to a set of event types.
type Endpoint struct {
	ID, OrgID, ProjectID uuid.UUID
	PublicID             string
	URL, Description     string
	EventTypes           []events.Type
	Secrets              SealedSecrets
	Active               bool
	CreatedAt, UpdatedAt time.Time
}

// SealedSecrets holds the signing secrets as sealed bytes; Secondary is nil outside a rotation.
type SealedSecrets struct{ Primary, Secondary []byte }

// Attempt records one delivery attempt of an outbox entry to an endpoint.
type Attempt struct {
	ID, OrgID, ProjectID, EndpointID, DeliveryID, EventID uuid.UUID
	EventType                                             events.Type
	Attempt                                               int
	StatusCode                                            int
	Error                                                 string
	ResponseBody                                          string
	ResponseTruncated                                     bool
	ResponseHeaders                                       []Header
	Duration                                              time.Duration
	CreatedAt                                             time.Time
}

// Header is one HTTP header name and value sent with a delivery.
type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ValidateOption relaxes an endpoint validation rule.
type ValidateOption func(*validateOpts)

type validateOpts struct{ allowInsecureURL bool }

// AllowInsecureURL permits http:// endpoint URLs; self-hosted dev only.
func AllowInsecureURL() ValidateOption { return func(o *validateOpts) { o.allowInsecureURL = true } }

func resolveValidateOpts(opts []ValidateOption) validateOpts {
	var o validateOpts
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// New enforces creation invariants and returns an active endpoint without secrets.
func New(orgID, projectID uuid.UUID, rawURL, description string, types []events.Type, opts ...ValidateOption) (*Endpoint, error) {
	u, d, ts, err := validate(rawURL, description, types, resolveValidateOpts(opts).allowInsecureURL)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	publicID, err := nanoid.New(21)
	if err != nil {
		return nil, err
	}
	return &Endpoint{
		ID:          uuid.Must(uuid.NewV7()),
		PublicID:    publicID,
		OrgID:       orgID,
		ProjectID:   projectID,
		URL:         u,
		Description: d,
		EventTypes:  ts,
		Active:      true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// Update re-validates and replaces the editable fields, leaving the secrets alone.
func (e *Endpoint) Update(rawURL, description string, types []events.Type, active bool, opts ...ValidateOption) error {
	u, d, ts, err := validate(rawURL, description, types, resolveValidateOpts(opts).allowInsecureURL)
	if err != nil {
		return err
	}
	e.URL = u
	e.Description = d
	e.EventTypes = ts
	e.Active = active
	e.UpdatedAt = time.Now().UTC()
	return nil
}

// SetSecrets replaces the sealed signing secrets.
func (e *Endpoint) SetSecrets(s SealedSecrets) {
	e.Secrets = s
	e.UpdatedAt = time.Now().UTC()
}

// Subscribes reports whether the endpoint is subscribed to t.
func (e *Endpoint) Subscribes(t events.Type) bool {
	return slices.Contains(e.EventTypes, t)
}

func validate(rawURL, description string, types []events.Type, allowInsecureURL bool) (u, d string, ts []events.Type, err error) {
	if u, err = validateURL(rawURL, allowInsecureURL); err != nil {
		return "", "", nil, err
	}
	if d, err = validateDescription(description); err != nil {
		return "", "", nil, err
	}
	if ts, err = validateTypes(types); err != nil {
		return "", "", nil, err
	}
	return u, d, ts, nil
}

func validateURL(rawURL string, allowInsecureURL bool) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", &InvalidURLError{Reason: URLRequired}
	}
	if len(rawURL) > MaxURLLen {
		return "", &InvalidURLError{Reason: URLTooLong}
	}
	if !utf8.ValidString(rawURL) {
		return "", &InvalidURLError{Reason: URLNotUTF8}
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", &InvalidURLError{Reason: URLUnparsable}
	}
	if u.Scheme != "https" && (!allowInsecureURL || u.Scheme != "http") {
		return "", &InvalidURLError{Reason: URLHTTPSRequired}
	}
	if u.User != nil {
		return "", &InvalidURLError{Reason: URLHasCredentials}
	}
	if u.Hostname() == "" {
		return "", &InvalidURLError{Reason: URLNoHost}
	}
	return rawURL, nil
}

func validateDescription(description string) (string, error) {
	description = strings.TrimSpace(description)
	if !utf8.ValidString(description) {
		return "", &InvalidDescriptionError{Reason: DescriptionNotUTF8}
	}
	if utf8.RuneCountInString(description) > MaxDescriptionRunes {
		return "", &InvalidDescriptionError{Reason: DescriptionTooLong}
	}
	return description, nil
}

func validateTypes(types []events.Type) ([]events.Type, error) {
	if len(types) == 0 {
		return nil, &InvalidEventTypesError{Reason: EventTypesRequired}
	}
	for i, t := range types {
		spec, ok := events.Lookup(t)
		if !ok {
			return nil, &InvalidEventTypesError{Event: t, Reason: EventTypeUnknown}
		}
		if !spec.Subscribable {
			return nil, &InvalidEventTypesError{Event: t, Reason: EventTypeNotSubscribable}
		}
		if slices.Contains(types[:i], t) {
			return nil, &InvalidEventTypesError{Event: t, Reason: EventTypeDuplicate}
		}
	}
	return slices.Clone(types), nil
}

type endpointRow struct {
	ID              uuid.UUID `alias:"webhook_endpoints.id"`
	PublicID        string    `alias:"webhook_endpoints.public_id"`
	OrgID           uuid.UUID `alias:"webhook_endpoints.org_id"`
	ProjectID       uuid.UUID `alias:"webhook_endpoints.project_id"`
	URL             string    `alias:"webhook_endpoints.url"`
	Description     string    `alias:"webhook_endpoints.description"`
	EventTypes      string    `alias:"webhook_endpoints.event_types"`
	SecretPrimary   []byte    `alias:"webhook_endpoints.secret_primary"`
	SecretSecondary []byte    `alias:"webhook_endpoints.secret_secondary"`
	Active          bool      `alias:"webhook_endpoints.active"`
	CreatedAt       time.Time `alias:"webhook_endpoints.created_at"`
	UpdatedAt       time.Time `alias:"webhook_endpoints.updated_at"`
}

func (r *endpointRow) toEndpoint() (*Endpoint, error) {
	types, err := unmarshalEventTypes(r.EventTypes)
	if err != nil {
		return nil, fmt.Errorf("webhook.postgres: %w", err)
	}
	return &Endpoint{
		ID:          r.ID,
		PublicID:    r.PublicID,
		OrgID:       r.OrgID,
		ProjectID:   r.ProjectID,
		URL:         r.URL,
		Description: r.Description,
		EventTypes:  types,
		Secrets:     SealedSecrets{Primary: r.SecretPrimary, Secondary: r.SecretSecondary},
		Active:      r.Active,
		CreatedAt:   r.CreatedAt.UTC(),
		UpdatedAt:   r.UpdatedAt.UTC(),
	}, nil
}

type attemptRow struct {
	ID                uuid.UUID `alias:"webhook_deliveries.id"`
	OrgID             uuid.UUID `alias:"webhook_deliveries.org_id"`
	ProjectID         uuid.UUID `alias:"webhook_deliveries.project_id"`
	EndpointID        uuid.UUID `alias:"webhook_deliveries.endpoint_id"`
	DeliveryID        uuid.UUID `alias:"webhook_deliveries.delivery_id"`
	EventID           uuid.UUID `alias:"webhook_deliveries.event_id"`
	EventType         string    `alias:"webhook_deliveries.event_type"`
	Attempt           int       `alias:"webhook_deliveries.attempt"`
	StatusCode        int       `alias:"webhook_deliveries.status_code"`
	Error             string    `alias:"webhook_deliveries.error"`
	ResponseBody      string    `alias:"webhook_deliveries.response_body"`
	ResponseTruncated bool      `alias:"webhook_deliveries.response_truncated"`
	ResponseHeaders   string    `alias:"webhook_deliveries.response_headers"`
	DurationMs        int64     `alias:"webhook_deliveries.duration_ms"`
	CreatedAt         time.Time `alias:"webhook_deliveries.created_at"`
}

func (r *attemptRow) toAttempt() (Attempt, error) {
	headers, err := unmarshalHeaders(r.ResponseHeaders)
	if err != nil {
		return Attempt{}, fmt.Errorf("webhook.postgres: attempt %w", err)
	}
	return Attempt{
		ID:                r.ID,
		OrgID:             r.OrgID,
		ProjectID:         r.ProjectID,
		EndpointID:        r.EndpointID,
		DeliveryID:        r.DeliveryID,
		EventID:           r.EventID,
		EventType:         events.Type(r.EventType),
		Attempt:           r.Attempt,
		StatusCode:        r.StatusCode,
		Error:             r.Error,
		ResponseBody:      r.ResponseBody,
		ResponseTruncated: r.ResponseTruncated,
		ResponseHeaders:   headers,
		Duration:          time.Duration(r.DurationMs) * time.Millisecond,
		CreatedAt:         r.CreatedAt.UTC(),
	}, nil
}
