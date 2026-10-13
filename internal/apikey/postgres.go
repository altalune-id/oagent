package apikey

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/schema/entity"
)

type postgresStore struct {
	pool       db.Pool
	keys       *entity.APIKeys
	grants     *entity.APIKeyProjects
	grantsExpr postgres.Expression
	hintIndex  string
}

// NewStore builds the Postgres store over pool.
func NewStore(pool db.Pool, schema, tablePrefix string) *postgresStore {
	if schema == "" {
		schema = "public"
	}
	return &postgresStore{
		pool:      pool,
		keys:      entity.NewAPIKeys(schema, tablePrefix),
		grants:    entity.NewAPIKeyProjects(schema, tablePrefix),
		hintIndex: tablePrefix + "api_keys_live_hint_key",
		// NOTE: Raw because go-jet has no builder for a correlated array_agg subquery in a projection.
		grantsExpr: postgres.Raw("COALESCE((SELECT array_agg(g.project_id ORDER BY g.created_at, g.project_id) FROM " +
			schema + "." + tablePrefix + "api_key_projects g WHERE g.org_id = api_keys.org_id AND g.key_id = api_keys.id), '{}')::text"),
	}
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
