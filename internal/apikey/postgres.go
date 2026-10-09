package apikey

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/schema/entity"
)

type postgresStore struct {
	pool       db.Pool
	keys       *entity.APIKeys
	grants     *entity.APIKeyProjects
	grantsExpr postgres.Expression
}

// NewStore builds the Postgres store over pool.
func NewStore(pool db.Pool, schema, tablePrefix string) *postgresStore {
	if schema == "" {
		schema = "public"
	}
	return &postgresStore{
		pool:   pool,
		keys:   entity.NewAPIKeys(schema, tablePrefix),
		grants: entity.NewAPIKeyProjects(schema, tablePrefix),
		// NOTE: Raw because go-jet has no builder for a correlated array_agg subquery in a projection.
		grantsExpr: postgres.Raw("COALESCE((SELECT array_agg(g.project_id ORDER BY g.created_at, g.project_id) FROM " +
			schema + "." + tablePrefix + "api_key_projects g WHERE g.org_id = api_keys.org_id AND g.key_id = api_keys.id), '{}')::text"),
	}
}

func (s *postgresStore) reader(ctx context.Context) qrm.DB {
	if tx, ok := db.CurrentTx(ctx); ok {
		return tx
	}
	return s.pool.R
}

func (s *postgresStore) writer(ctx context.Context) qrm.DB {
	if tx, ok := db.CurrentTx(ctx); ok {
		return tx
	}
	return s.pool.W
}

func (s *postgresStore) selectColumns() postgres.ProjectionList {
	return postgres.ProjectionList{
		s.keys.ID, s.keys.OrgID, s.keys.ProjectID, s.keys.Kind, s.keys.AllProjects,
		s.grantsExpr.AS("api_keys.project_ids"),
		s.keys.Name, s.keys.SecretHash, s.keys.SecretHint,
		postgres.CAST(s.keys.Scopes).AS_TEXT().AS("api_keys.scopes"),
		postgres.CAST(s.keys.ResourceIDs).AS_TEXT().AS("api_keys.resource_ids"),
		s.keys.CreatedBy, s.keys.CreatedAt, s.keys.ExpiresAt, s.keys.RevokedAt, s.keys.LastUsedAt,
	}
}

// apiKeyRow is the jet scan target for a selected api_keys projection (project_ids comes from the grant subquery).
type apiKeyRow struct {
	ID          uuid.UUID  `alias:"api_keys.id"`
	OrgID       uuid.UUID  `alias:"api_keys.org_id"`
	ProjectID   *uuid.UUID `alias:"api_keys.project_id"`
	Kind        string     `alias:"api_keys.kind"`
	AllProjects bool       `alias:"api_keys.all_projects"`
	ProjectIDs  string     `alias:"api_keys.project_ids"`
	Name        string     `alias:"api_keys.name"`
	SecretHash  []byte     `alias:"api_keys.secret_hash"`
	SecretHint  string     `alias:"api_keys.secret_hint"`
	Scopes      string     `alias:"api_keys.scopes"`
	ResourceIDs string     `alias:"api_keys.resource_ids"`
	CreatedBy   *uuid.UUID `alias:"api_keys.created_by"`
	CreatedAt   time.Time  `alias:"api_keys.created_at"`
	ExpiresAt   *time.Time `alias:"api_keys.expires_at"`
	RevokedAt   *time.Time `alias:"api_keys.revoked_at"`
	LastUsedAt  *time.Time `alias:"api_keys.last_used_at"`
}

func (r *apiKeyRow) toKey() (*APIKey, error) {
	scopes, err := parsePgTextArray(r.Scopes)
	if err != nil {
		return nil, fmt.Errorf("apikey.postgres: parse scopes: %w", err)
	}
	resourceIDs, err := parsePgUUIDArray(r.ResourceIDs)
	if err != nil {
		return nil, fmt.Errorf("apikey.postgres: parse resource_ids: %w", err)
	}
	projectIDs, err := parsePgUUIDArray(r.ProjectIDs)
	if err != nil {
		return nil, fmt.Errorf("apikey.postgres: parse project_ids: %w", err)
	}
	var hash [32]byte
	copy(hash[:], r.SecretHash)
	k := &APIKey{
		ID:          r.ID,
		OrgID:       r.OrgID,
		Kind:        Kind(r.Kind),
		AllProjects: r.AllProjects,
		ProjectIDs:  projectIDs,
		Name:        r.Name,
		SecretHash:  hash,
		SecretHint:  r.SecretHint,
		Scopes:      scopes,
		ResourceIDs: resourceIDs,
		CreatedAt:   r.CreatedAt.UTC(),
	}
	if r.ProjectID != nil {
		k.ProjectID = *r.ProjectID
	}
	if r.CreatedBy != nil {
		k.CreatedBy = *r.CreatedBy
	}
	if r.ExpiresAt != nil {
		t := r.ExpiresAt.UTC()
		k.ExpiresAt = &t
	}
	if r.RevokedAt != nil {
		t := r.RevokedAt.UTC()
		k.RevokedAt = &t
	}
	if r.LastUsedAt != nil {
		t := r.LastUsedAt.UTC()
		k.LastUsedAt = &t
	}
	return k, nil
}

func parsePgUUIDArray(raw string) ([]uuid.UUID, error) {
	strs, err := parsePgTextArray(raw)
	if err != nil {
		return nil, err
	}
	out := make([]uuid.UUID, 0, len(strs))
	for _, s := range strs {
		id, perr := uuid.Parse(s)
		if perr != nil {
			return nil, perr
		}
		out = append(out, id)
	}
	return out, nil
}

func pgNullableTimeExpr(t *time.Time) postgres.TimestampzExpression {
	if t == nil {
		return entity.NullTimestampz()
	}
	return postgres.TimestampzT(t.UTC())
}

// NOTE: the cast to text[] is explicit because Postgres has no assignment cast from a text parameter to an array type.
func pgTextArrayExpr(elems []string) postgres.StringExpression {
	return postgres.StringExp(postgres.CAST(postgres.String(pgTextArrayLiteral(elems))).AS("text[]"))
}

func pgUUIDArrayExpr(ids []uuid.UUID) postgres.StringExpression {
	return postgres.StringExp(postgres.CAST(postgres.String(pgUUIDArrayLiteral(ids))).AS("uuid[]"))
}

func pgTextArrayLiteral(elems []string) string {
	if len(elems) == 0 {
		return "{}"
	}
	esc := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	parts := make([]string, len(elems))
	for i, e := range elems {
		parts[i] = `"` + esc.Replace(e) + `"`
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func pgUUIDArrayLiteral(ids []uuid.UUID) string {
	elems := make([]string, len(ids))
	for i, id := range ids {
		elems[i] = id.String()
	}
	return pgTextArrayLiteral(elems)
}

func parsePgTextArray(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return []string{}, nil
	}
	if len(s) < 2 || s[0] != '{' || s[len(s)-1] != '}' {
		return nil, fmt.Errorf("malformed array literal %q", s)
	}
	body := s[1 : len(s)-1]
	if body == "" {
		return []string{}, nil
	}
	var out []string
	var cur strings.Builder
	inQuotes, escaped := false, false
	for _, r := range body {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case r == '"':
			inQuotes = !inQuotes
		case r == ',' && !inQuotes:
			out = append(out, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	out = append(out, cur.String())
	return out, nil
}
