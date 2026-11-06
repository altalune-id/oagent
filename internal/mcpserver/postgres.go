package mcpserver

import (
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/schema/entity"
)

type postgresStore struct {
	pool    db.Pool
	servers *entity.MCPServers
	resolve string
}

// NewStore builds the Postgres store over pool.
func NewStore(pool db.Pool, schema, tablePrefix string) *postgresStore {
	if schema == "" {
		schema = "public"
	}
	return &postgresStore{
		pool:    pool,
		servers: entity.NewMCPServers(schema, tablePrefix),
		// NOTE: RawStatement because go-jet has no builder for a set-returning function in FROM position.
		resolve: `SELECT x.id AS "mcp_servers.id", x.org_id AS "mcp_servers.org_id", x.project_id AS "mcp_servers.project_id" FROM ` +
			schema + "." + tablePrefix + "resolve_mcp_server_by_public_id(#publicID) x",
	}
}

type serverRow struct {
	ID         uuid.UUID `alias:"mcp_servers.id"`
	PublicID   string    `alias:"mcp_servers.public_id"`
	OrgID      uuid.UUID `alias:"mcp_servers.org_id"`
	ProjectID  uuid.UUID `alias:"mcp_servers.project_id"`
	Name       string    `alias:"mcp_servers.name"`
	URL        string    `alias:"mcp_servers.url"`
	AuthKind   string    `alias:"mcp_servers.auth_kind"`
	AuthHeader *string   `alias:"mcp_servers.auth_header"`
	AuthSealed []byte    `alias:"mcp_servers.auth_sealed"`
	TimeoutMS  int32     `alias:"mcp_servers.timeout_ms"`
	Trusted    bool      `alias:"mcp_servers.trusted"`
	CreatedAt  time.Time `alias:"mcp_servers.created_at"`
	UpdatedAt  time.Time `alias:"mcp_servers.updated_at"`
}

func (r *serverRow) toServer() *Server {
	s := &Server{
		ID:         r.ID,
		PublicID:   r.PublicID,
		OrgID:      r.OrgID,
		ProjectID:  r.ProjectID,
		Name:       r.Name,
		URL:        r.URL,
		AuthKind:   AuthKind(r.AuthKind),
		AuthSealed: r.AuthSealed,
		TimeoutMS:  r.TimeoutMS,
		Trusted:    r.Trusted,
		CreatedAt:  r.CreatedAt.UTC(),
		UpdatedAt:  r.UpdatedAt.UTC(),
	}
	if r.AuthHeader != nil {
		s.AuthHeader = *r.AuthHeader
	}
	return s
}

func toServers(rows []serverRow) []*Server {
	out := make([]*Server, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toServer())
	}
	return out
}
