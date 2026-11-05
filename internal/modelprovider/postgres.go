package modelprovider

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/schema/entity"
)

type postgresStore struct {
	pool      db.Pool
	providers *entity.ModelProviders
	resolve   string
}

// NewStore builds the Postgres store over pool.
func NewStore(pool db.Pool, schema, tablePrefix string) *postgresStore {
	if schema == "" {
		schema = "public"
	}
	return &postgresStore{
		pool:      pool,
		providers: entity.NewModelProviders(schema, tablePrefix),
		// NOTE: RawStatement because go-jet has no builder for a set-returning function in FROM position.
		resolve: `SELECT x.id AS "model_providers.id", x.org_id AS "model_providers.org_id", x.project_id AS "model_providers.project_id" FROM ` +
			schema + "." + tablePrefix + "resolve_model_provider_by_public_id(#publicID) x",
	}
}

type providerRow struct {
	ID           uuid.UUID `alias:"model_providers.id"`
	PublicID     string    `alias:"model_providers.public_id"`
	OrgID        uuid.UUID `alias:"model_providers.org_id"`
	ProjectID    uuid.UUID `alias:"model_providers.project_id"`
	Name         string    `alias:"model_providers.name"`
	Kind         string    `alias:"model_providers.kind"`
	BaseURL      string    `alias:"model_providers.base_url"`
	APIKeySealed []byte    `alias:"model_providers.api_key_sealed"`
	Models       string    `alias:"model_providers.models"`
	CreatedAt    time.Time `alias:"model_providers.created_at"`
	UpdatedAt    time.Time `alias:"model_providers.updated_at"`
}

func (r *providerRow) toProvider() (*Provider, error) {
	var models []Model
	if err := json.Unmarshal([]byte(r.Models), &models); err != nil {
		return nil, fmt.Errorf("modelprovider: models: %w", err)
	}
	return &Provider{
		ID:           r.ID,
		PublicID:     r.PublicID,
		OrgID:        r.OrgID,
		ProjectID:    r.ProjectID,
		Name:         r.Name,
		Kind:         Kind(r.Kind),
		BaseURL:      r.BaseURL,
		APIKeySealed: r.APIKeySealed,
		Models:       models,
		CreatedAt:    r.CreatedAt.UTC(),
		UpdatedAt:    r.UpdatedAt.UTC(),
	}, nil
}

func toProviders(rows []providerRow) ([]*Provider, error) {
	out := make([]*Provider, 0, len(rows))
	for i := range rows {
		p, err := rows[i].toProvider()
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}
