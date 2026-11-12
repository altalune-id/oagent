package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/go-jet/jet/v2/qrm"
)

const setLocal = `SELECT set_config($1, $2, true)`

// Setting is one transaction-local planner setting a ReadTuned query applies.
type Setting struct {
	Name, Value string
}

// ErrSettingRefused rejects a setting outside the allowlist or a value it does not take.
var ErrSettingRefused = errors.New("db: setting refused")

// ErrTunedInUnitOfWork rejects ReadTuned inside a unit of work, where its settings would last past the read.
var ErrTunedInUnitOfWork = errors.New("db: ReadTuned never runs inside a unit of work")

// ReadTuned runs fn in its own read-only transaction on pool.R with the tenant scope of ctx and the allowlisted tune settings.
func ReadTuned(ctx context.Context, pool Pool, tune []Setting, fn func(q qrm.DB) error) error {
	for _, s := range tune {
		if !allowed(s) {
			return fmt.Errorf("%w: %s", ErrSettingRefused, s.Name)
		}
	}
	if _, ok := unitOfWorkFromContext(ctx); ok {
		return ErrTunedInUnitOfWork
	}
	return own(ctx, pool.R, &sql.TxOptions{ReadOnly: true}, func(uow *unitOfWork) error {
		for _, s := range tune {
			if _, err := uow.tx.ExecContext(ctx, setLocal, s.Name, s.Value); err != nil {
				return fmt.Errorf("db: set %s: %w", s.Name, err)
			}
		}
		return fn(uow.tx)
	})
}

func allowed(s Setting) bool {
	switch s.Name {
	case "hnsw.iterative_scan":
		return slices.Contains([]string{"relaxed_order", "strict_order", "off"}, s.Value)
	case "hnsw.max_scan_tuples":
		n, err := strconv.Atoi(s.Value)
		return err == nil && n > 0 && strconv.Itoa(n) == s.Value
	}
	return false
}
