// Package migrator runs goose migrations with schema, prefix and role templating.
package migrator

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/database"

	"altalune.id/oagent/pg"
)

// Migrator runs migrations from an fs.FS.
type Migrator struct {
	fsys     fs.FS
	cfg      Config
	store    database.Store
	upgrades []Upgrade
}

// New builds a Migrator over fsys.
func New(fsys fs.FS, cfg Config) *Migrator {
	return &Migrator{fsys: fsys, cfg: cfg}
}

// TargetVersion reads VERSION from the migrations FS.
func (m *Migrator) TargetVersion() (int64, error) {
	b, err := fs.ReadFile(m.fsys, "VERSION")
	if err != nil {
		return 0, fmt.Errorf("migrator: read VERSION: %w", err)
	}
	s := strings.TrimSpace(string(b))
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil || v < 0 {
		return 0, fmt.Errorf("migrator: VERSION %q is not a valid version", s)
	}
	return v, nil
}

func (m *Migrator) provider(sqldb *sql.DB) (*goose.Provider, error) {
	cfg := m.cfg.withDefaults()
	tpl := newTemplatedFS(m.fsys, templateVars(cfg))
	store := m.store
	if store == nil {
		table, err := gooseTableName(cfg)
		if err != nil {
			return nil, err
		}
		store, err = database.NewStore(database.DialectPostgres, table)
		if err != nil {
			return nil, fmt.Errorf("migrator: store: %w", err)
		}
	}
	return goose.NewProvider("", sqldb, tpl, goose.WithStore(store))
}

func gooseTableName(cfg Config) (string, error) {
	cfg = cfg.withDefaults()
	table := cfg.TablePrefix + "goose_db_version"
	if err := pg.ValidateIdentifier(cfg.Schema); err != nil {
		return "", fmt.Errorf("migrator: schema: %w", err)
	}
	if err := pg.ValidateIdentifier(table); err != nil {
		return "", fmt.Errorf("migrator: tablePrefix: %w", err)
	}
	return cfg.Schema + "." + table, nil
}

// Up applies pending migrations up to VERSION.
func (m *Migrator) Up(ctx context.Context, sqldb *sql.DB) error {
	target, err := m.TargetVersion()
	if err != nil {
		return err
	}
	p, err := m.provider(sqldb)
	if err != nil {
		return err
	}
	if _, err := p.UpTo(ctx, target); err != nil {
		return fmt.Errorf("migrator: up to %d: %w", target, err)
	}
	for _, u := range m.upgrades {
		if err := u.Run(ctx, sqldb); err != nil {
			return fmt.Errorf("migrator: upgrade %q: %w", u.Name, err)
		}
	}
	return nil
}

// MigrationStatus is one migration's status.
type MigrationStatus struct {
	Version   int64
	Applied   bool
	AppliedAt time.Time
	Source    string
}

// Status reports each migration's applied state.
func (m *Migrator) Status(ctx context.Context, sqldb *sql.DB) ([]MigrationStatus, error) {
	p, err := m.provider(sqldb)
	if err != nil {
		return nil, err
	}
	items, err := p.Status(ctx)
	if err != nil {
		return nil, fmt.Errorf("migrator: status: %w", err)
	}
	rows := make([]MigrationStatus, 0, len(items))
	for _, it := range items {
		row := MigrationStatus{Applied: it.State == goose.StateApplied, AppliedAt: it.AppliedAt}
		if it.Source != nil {
			row.Version = it.Source.Version
			row.Source = it.Source.Path
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// IsUpToDate reports whether every migration up to VERSION is applied.
func (m *Migrator) IsUpToDate(ctx context.Context, sqldb *sql.DB) (bool, error) {
	target, err := m.TargetVersion()
	if err != nil {
		return false, err
	}
	rows, err := m.Status(ctx, sqldb)
	if err != nil {
		return false, err
	}
	return upToDate(target, rows), nil
}

func upToDate(target int64, rows []MigrationStatus) bool {
	for _, r := range rows {
		if r.Version <= target && !r.Applied {
			return false
		}
	}
	return true
}

// DownTo rolls back to version.
func (m *Migrator) DownTo(ctx context.Context, sqldb *sql.DB, version int64) error {
	p, err := m.provider(sqldb)
	if err != nil {
		return err
	}
	if _, err := p.DownTo(ctx, version); err != nil {
		return fmt.Errorf("migrator: down-to %d: %w", version, err)
	}
	return nil
}
