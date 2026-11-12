// Package modelprovider models the language model providers a project's agents call.
package modelprovider

import (
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/nanoid"
	"altalune.id/oagent/weburl"
)

// MaxBaseURLLen bounds a provider base URL, in bytes.
const MaxBaseURLLen = weburl.MaxLen

// MaxModels bounds the models one provider lists.
const MaxModels = 64

// Kind names the wire protocol a provider speaks.
type Kind string

// The provider kinds.
const (
	KindOpenAICompatible Kind = "openai_compatible"
	KindAnthropic        Kind = "anthropic"
)

// Kinds returns every provider kind.
func Kinds() []Kind { return []Kind{KindOpenAICompatible, KindAnthropic} }

// Prices are micro-USD per 1M tokens, per token class.
type Prices struct {
	InputMicros      int64 `json:"input_micros"`
	OutputMicros     int64 `json:"output_micros"`
	CacheReadMicros  int64 `json:"cache_read_micros"`
	CacheWriteMicros int64 `json:"cache_write_micros"`
}

// Model is one model a provider serves; nil Prices means unpriced, and Embedding marks a model that turns text into vectors.
type Model struct {
	ID        string  `json:"id"`
	Vision    bool    `json:"vision,omitempty"`
	Embedding bool    `json:"embedding,omitempty"`
	Prices    *Prices `json:"prices,omitempty"`
}

// Input is the editable part of a provider.
type Input struct {
	Name    string
	Kind    Kind
	BaseURL string
	Models  []Model
}

// Provider is the aggregate root: one model endpoint and its key, scoped to a project.
type Provider struct {
	ID, OrgID, ProjectID uuid.UUID
	PublicID, Name       string
	Kind                 Kind
	BaseURL              string
	APIKeySealed         []byte
	Models               []Model
	CreatedAt, UpdatedAt time.Time
}

// Ref names a provider and the tenant it belongs to, before any row is read.
type Ref struct {
	ID        uuid.UUID
	OrgID     uuid.UUID
	ProjectID uuid.UUID
}

// ValidateOption relaxes a provider validation rule.
type ValidateOption func(*validateOpts)

type validateOpts struct{ allowInsecureURL bool }

// AllowInsecureURL permits http:// base URLs; self-hosted dev only.
func AllowInsecureURL() ValidateOption { return func(o *validateOpts) { o.allowInsecureURL = true } }

func resolveValidateOpts(opts []ValidateOption) validateOpts {
	var o validateOpts
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// New enforces creation invariants and returns a provider without a sealed key.
func New(orgID, projectID uuid.UUID, in Input, now time.Time, opts ...ValidateOption) (*Provider, error) {
	baseURL, models, err := validate(in, resolveValidateOpts(opts))
	if err != nil {
		return nil, err
	}
	publicID, err := nanoid.New(21)
	if err != nil {
		return nil, err
	}
	now = now.UTC()
	return &Provider{
		ID:        uuid.Must(uuid.NewV7()),
		PublicID:  publicID,
		OrgID:     orgID,
		ProjectID: projectID,
		Name:      strings.TrimSpace(in.Name),
		Kind:      in.Kind,
		BaseURL:   baseURL,
		Models:    models,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// Update re-validates and replaces the editable fields, leaving the sealed key and kind alone, and reports whether the base URL changed.
func (p *Provider) Update(in Input, now time.Time, opts ...ValidateOption) (urlChanged bool, err error) {
	if in.Kind != p.Kind {
		return false, &KindChangedError{}
	}
	baseURL, models, err := validate(in, resolveValidateOpts(opts))
	if err != nil {
		return false, err
	}
	urlChanged = baseURL != p.BaseURL
	p.Name = strings.TrimSpace(in.Name)
	p.BaseURL = baseURL
	p.Models = models
	p.UpdatedAt = now.UTC()
	return urlChanged, nil
}

// Model returns the provider's model with the given id.
func (p *Provider) Model(id string) (Model, bool) {
	i := slices.IndexFunc(p.Models, func(m Model) bool { return m.ID == id })
	if i < 0 {
		return Model{}, false
	}
	return p.Models[i], true
}

func validate(in Input, o validateOpts) (string, []Model, error) {
	baseURL, err := validateBaseURL(in.BaseURL, o.allowInsecureURL)
	if err != nil {
		return "", nil, err
	}
	models, err := validateModels(in.Models)
	if err != nil {
		return "", nil, err
	}
	return baseURL, models, nil
}

func validateBaseURL(rawURL string, allowInsecureURL bool) (string, error) {
	u, reason := weburl.Check(rawURL, allowInsecureURL)
	if reason != "" {
		return "", &InvalidBaseURLError{Reason: URLReason(reason)}
	}
	return u, nil
}

func validateModels(models []Model) ([]Model, error) {
	if len(models) == 0 {
		return nil, &InvalidModelsError{Reason: ModelsRequired}
	}
	if len(models) > MaxModels {
		return nil, &InvalidModelsError{Reason: ModelsTooMany}
	}
	for i, m := range models {
		if slices.ContainsFunc(models[:i], func(prev Model) bool { return prev.ID == m.ID }) {
			return nil, &InvalidModelsError{Reason: ModelsDuplicate, Model: m.ID}
		}
		if m.Prices != nil && (m.Prices.InputMicros < 0 || m.Prices.OutputMicros < 0 ||
			m.Prices.CacheReadMicros < 0 || m.Prices.CacheWriteMicros < 0) {
			return nil, &InvalidModelsError{Reason: ModelsPrice, Model: m.ID}
		}
	}
	return cloneModels(models), nil
}

func cloneModels(models []Model) []Model {
	out := make([]Model, len(models))
	for i, m := range models {
		out[i] = m
		if m.Prices != nil {
			prices := *m.Prices
			out[i].Prices = &prices
		}
	}
	return out
}
