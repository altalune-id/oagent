//go:build integration

package schema_test

import (
	"database/sql"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/testutil/pgtest"
	"altalune.id/oagent/nanoid"
)

type provisioned struct {
	psql, dsn, prefix string
}

func (p provisioned) render(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile("../scripts/db/" + name)
	require.NoError(t, err)
	return strings.NewReplacer("@@PREFIX@@", p.prefix, "@@SCHEMA@@", "public", "@@TABLE_PREFIX@@", "").Replace(string(raw))
}

func (p provisioned) run(t *testing.T, script string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), p.psql, "-qX", "-v", "ON_ERROR_STOP=1", p.dsn)
	cmd.Stdin = strings.NewReader(script)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
}

func (p provisioned) bootstrap(t *testing.T) {
	t.Helper()
	p.run(t, "\\set migrator_password 'migrator-password-0123456789'\n\\set service_password 'service-password-0123456789'\nBEGIN;\n"+
		p.render(t, "bootstrap.template.sql")+"\nCOMMIT;\n")
}

func openWith(t *testing.T, dsn, user, password string) *sql.DB {
	t.Helper()
	cfg, err := pgx.ParseConfig(dsn)
	require.NoError(t, err)
	if user != "" {
		cfg.User, cfg.Password = user, password
	}
	db := stdlib.OpenDB(*cfg)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// SECURITY: a bootstrap re-run must not strip pgvector's operators, or every search fails; verify must accept them in public.
func TestProvision_RerunKeepsTheVectorOperators(t *testing.T) {
	psql, err := exec.LookPath("psql")
	require.NoError(t, err, "the provisioning scripts run through psql")
	h := pgtest.New(t)
	admin := openWith(t, h.DSN, "", "")
	dbName := "prov_" + strings.ToLower(nanoid.MustNew(10))
	dbName = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return '_'
	}, dbName)
	_, err = admin.ExecContext(t.Context(), "CREATE DATABASE "+pgx.Identifier{dbName}.Sanitize())
	require.NoError(t, err)
	prefix := dbName
	t.Cleanup(func() {
		ctx := t.Context()
		_, _ = admin.ExecContext(ctx, "DROP DATABASE IF EXISTS "+pgx.Identifier{dbName}.Sanitize()+" WITH (FORCE)")
		for _, role := range []string{prefix + "_migrator", prefix + "_service", prefix + "_owner"} {
			_, _ = admin.ExecContext(ctx, "DROP ROLE IF EXISTS "+pgx.Identifier{role}.Sanitize())
		}
	})
	cfg, err := pgx.ParseConfig(h.DSN)
	require.NoError(t, err)
	dsn := url.URL{
		Scheme: "postgres", User: url.UserPassword(cfg.User, cfg.Password), Host: net.JoinHostPort(cfg.Host, strconv.Itoa(int(cfg.Port))),
		Path: "/" + dbName, RawQuery: "sslmode=disable",
	}
	p := provisioned{psql: psql, dsn: dsn.String(), prefix: prefix}

	p.bootstrap(t)
	p.bootstrap(t)
	p.run(t, p.render(t, "verify.template.sql"))

	svc := openWith(t, p.dsn, prefix+"_service", "service-password-0123456789")
	var dist float64
	var dims int
	require.NoError(t, svc.QueryRowContext(t.Context(),
		`SELECT '[1,0]'::vector <=> '[0,1]'::vector, vector_dims('[1,2,3]'::vector)`).Scan(&dist, &dims))
	assert.InDelta(t, 1.0, dist, 1e-6)
	assert.Equal(t, 3, dims)

	var public bool
	require.NoError(t, openWith(t, p.dsn, "", "").QueryRowContext(t.Context(),
		`SELECT has_function_privilege('public', 'public.vector_dims(public.vector)'::regprocedure, 'EXECUTE')`).Scan(&public))
	assert.True(t, public, "the extension's own grants survive a re-run")
}
