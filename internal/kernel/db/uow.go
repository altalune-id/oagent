package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/go-jet/jet/v2/qrm"
)

type txCtxKey struct{}

type unitOfWork struct {
	tx *sql.Tx
}

// Transactor runs a use case as one unit of work.
type Transactor interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type poolTransactor struct {
	pool Pool
}

// NewTransactor returns a Transactor that begins its transactions on p.W.
func NewTransactor(p Pool) Transactor {
	return poolTransactor{pool: p}
}

func (t poolTransactor) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return RunInTx(ctx, t.pool, fn)
}

// RunInTx runs fn in the transaction on ctx, or begins one on pool.W, exposes it via ctx, and commits when fn returns nil.
func RunInTx(ctx context.Context, pool Pool, fn func(ctx context.Context) error) error {
	if _, ok := unitOfWorkFromContext(ctx); ok {
		return fn(ctx)
	}
	return own(ctx, pool, func(tx *sql.Tx) error {
		return fn(contextWithUnitOfWork(ctx, &unitOfWork{tx: tx}))
	})
}

// InTx runs fn in the transaction on ctx, or in its own that it commits or rolls back.
func InTx(ctx context.Context, pool Pool, fn func(tx *sql.Tx) error) error {
	if tx, ok := CurrentTx(ctx); ok {
		return fn(tx)
	}
	return own(ctx, pool, fn)
}

// Reader returns the transaction on ctx, else pool.R.
func Reader(ctx context.Context, pool Pool) qrm.DB {
	if tx, ok := CurrentTx(ctx); ok {
		return tx
	}
	return pool.R
}

// Writer returns the transaction on ctx, else pool.W.
func Writer(ctx context.Context, pool Pool) qrm.DB {
	if tx, ok := CurrentTx(ctx); ok {
		return tx
	}
	return pool.W
}

func own(ctx context.Context, pool Pool, fn func(tx *sql.Tx) error) error {
	if pool.W == nil {
		return errors.New("db: pool.W is nil")
	}
	tx, err := pool.W.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("db: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("db: commit: %w", err)
	}
	return nil
}

// CurrentTx returns the transaction bound to ctx, or (nil, false) if none is active.
func CurrentTx(ctx context.Context) (*sql.Tx, bool) {
	uow, ok := unitOfWorkFromContext(ctx)
	if !ok {
		return nil, false
	}
	return uow.tx, true
}

// ContextWithTx returns a context carrying tx as the active unit of work.
func ContextWithTx(ctx context.Context, tx *sql.Tx) context.Context {
	return contextWithUnitOfWork(ctx, &unitOfWork{tx: tx})
}

func contextWithUnitOfWork(ctx context.Context, uow *unitOfWork) context.Context {
	return context.WithValue(ctx, txCtxKey{}, uow)
}

func unitOfWorkFromContext(ctx context.Context) (*unitOfWork, bool) {
	uow, ok := ctx.Value(txCtxKey{}).(*unitOfWork)
	return uow, ok
}
