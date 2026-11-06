//go:build integration

package schema_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/testutil/pgtest"
)

func TestEndUserIdentities_ChecksRefuseBadRows(t *testing.T) {
	db, _ := pgtest.Migrated(t)
	s := seedTwoOrgs(t, db)
	endUser := uuid.New()
	_, err := db.ExecContext(t.Context(), `INSERT INTO end_users (id, public_id, org_id, project_id, created_at, updated_at) VALUES ($1, $2, $3, $4, now(), now())`,
		endUser, "eu-checks", s.orgA, s.prjA)
	require.NoError(t, err)
	insert := func(kind, value string) error {
		_, err := db.ExecContext(t.Context(), `INSERT INTO end_user_identities (id, org_id, project_id, end_user_id, kind, value, created_at) VALUES ($1, $2, $3, $4, $5, $6, now())`,
			uuid.New(), s.orgA, s.prjA, endUser, kind, value)
		return err
	}
	for name, row := range map[string]struct{ kind, value, constraint string }{
		"unknown kind":   {"email", "a@b.c", "end_user_identities_kind_check"},
		"empty value":    {"api_user", "", "end_user_identities_value_length"},
		"257-char value": {"api_user", strings.Repeat("v", 257), "end_user_identities_value_length"},
	} {
		t.Run(name, func(t *testing.T) {
			err := insert(row.kind, row.value)
			pgErr, ok := errors.AsType[*pgconn.PgError](err)
			require.True(t, ok, "got %v", err)
			assert.Equal(t, "23514", pgErr.Code)
			assert.Equal(t, row.constraint, pgErr.ConstraintName)
		})
	}
	for _, kind := range []string{"api_user", "wa_phone", "wa_lid", "widget_anon", "widget_sub"} {
		require.NoError(t, insert(kind, strings.Repeat("v", 256)), kind)
	}
	assert.Equal(t, "23505", pgCode(insert("api_user", strings.Repeat("v", 256))), "one identity per project, kind and value")
}

func TestEndUserIdentities_EndUserOfAnotherProjectRefused(t *testing.T) {
	db, _ := pgtest.Migrated(t)
	s := seedTwoOrgs(t, db)
	endUser := uuid.New()
	_, err := db.ExecContext(t.Context(), `INSERT INTO end_users (id, public_id, org_id, project_id, created_at, updated_at) VALUES ($1, $2, $3, $4, now(), now())`,
		endUser, "eu-fk", s.orgA, s.prjA)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `INSERT INTO end_user_identities (id, org_id, project_id, end_user_id, kind, value, created_at) VALUES ($1, $2, $3, $4, 'api_user', 'x', now())`,
		uuid.New(), s.orgB, s.prjB, endUser)
	assert.Equal(t, "23503", pgCode(err), "an identity always belongs to its end user's org and project")

	_, err = db.ExecContext(t.Context(), `INSERT INTO end_user_identities (id, org_id, project_id, end_user_id, kind, value, created_at) VALUES ($1, $2, $3, $4, 'api_user', 'y', now())`,
		uuid.New(), s.orgA, s.prjA, endUser)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `DELETE FROM end_users WHERE id = $1`, endUser)
	require.NoError(t, err)
	var n int
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT count(*) FROM end_user_identities WHERE end_user_id = $1`, endUser).Scan(&n))
	assert.Zero(t, n, "identities go with their end user")
}
