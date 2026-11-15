package webtool

import (
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/schema/entity"
)

type postgresStore struct {
	pool      db.Pool
	providers *entity.WebSearchProviders
	secrets   *entity.HTTPSecrets
	resolve   string
}

// NewStore builds the Postgres store over pool.
func NewStore(pool db.Pool, schema, tablePrefix string) *postgresStore {
	if schema == "" {
		schema = "public"
	}
	return &postgresStore{
		pool:      pool,
		providers: entity.NewWebSearchProviders(schema, tablePrefix),
		secrets:   entity.NewHTTPSecrets(schema, tablePrefix),
		// NOTE: RawStatement because go-jet has no builder for a set-returning function in FROM position.
		resolve: `SELECT x.id AS "http_secrets.id", x.org_id AS "http_secrets.org_id", x.project_id AS "http_secrets.project_id" FROM ` +
			schema + "." + tablePrefix + "resolve_http_secret_by_public_id(#publicID) x",
	}
}

type providerRow struct {
	ProjectID          uuid.UUID `alias:"web_search_providers.project_id"`
	OrgID              uuid.UUID `alias:"web_search_providers.org_id"`
	Kind               string    `alias:"web_search_providers.kind"`
	APIKeySealed       []byte    `alias:"web_search_providers.api_key_sealed"`
	PricePerCallMicros int64     `alias:"web_search_providers.price_per_call_micros"`
	CreatedAt          time.Time `alias:"web_search_providers.created_at"`
	UpdatedAt          time.Time `alias:"web_search_providers.updated_at"`
}

func (r *providerRow) toProvider() *Provider {
	return &Provider{
		OrgID: r.OrgID, ProjectID: r.ProjectID, Kind: Kind(r.Kind), APIKeySealed: r.APIKeySealed,
		PricePerCallMicros: r.PricePerCallMicros, CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
}

type secretRow struct {
	ID          uuid.UUID `alias:"http_secrets.id"`
	PublicID    string    `alias:"http_secrets.public_id"`
	OrgID       uuid.UUID `alias:"http_secrets.org_id"`
	ProjectID   uuid.UUID `alias:"http_secrets.project_id"`
	Host        string    `alias:"http_secrets.host"`
	Header      string    `alias:"http_secrets.header"`
	ValueSealed []byte    `alias:"http_secrets.value_sealed"`
	CreatedAt   time.Time `alias:"http_secrets.created_at"`
	UpdatedAt   time.Time `alias:"http_secrets.updated_at"`
}

func (r *secretRow) toSecret() *HTTPSecret {
	return &HTTPSecret{
		ID: r.ID, PublicID: r.PublicID, OrgID: r.OrgID, ProjectID: r.ProjectID, Host: r.Host, Header: r.Header,
		ValueSealed: r.ValueSealed, CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
}

func toSecrets(rows []secretRow) []*HTTPSecret {
	out := make([]*HTTPSecret, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toSecret())
	}
	return out
}
