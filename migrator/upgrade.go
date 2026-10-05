package migrator

import (
	"context"
	"database/sql"
)

// Upgrade is an external migration suite run after goose.
type Upgrade struct {
	Name string
	Run  func(ctx context.Context, db *sql.DB) error
}

// WithUpgrade registers an external migration suite to run after goose, in call order.
func (m *Migrator) WithUpgrade(name string, run func(ctx context.Context, db *sql.DB) error) *Migrator {
	m.upgrades = append(m.upgrades, Upgrade{Name: name, Run: run})
	return m
}
