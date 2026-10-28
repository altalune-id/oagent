// Package db is the database feature: the writer/reader pool, the unit of work and the Postgres error checks.
package db

import (
	"context"
	"database/sql"

	"github.com/go-jet/jet/v2/qrm"
)

// Read runs fn on the unit of work in ctx, or in a read-only transaction on pool.R that carries the tenant scope of ctx.
func Read(ctx context.Context, pool Pool, fn func(q qrm.DB) error) error {
	return readOn(ctx, pool.R, fn)
}

// ReadPrimary is Read on pool.W, for reads that must see the latest commit.
func ReadPrimary(ctx context.Context, pool Pool, fn func(q qrm.DB) error) error {
	return readOn(ctx, pool.W, fn)
}

func readOn(ctx context.Context, handle *sql.DB, fn func(q qrm.DB) error) error {
	if uow, ok := unitOfWorkFromContext(ctx); ok {
		if err := uow.join(ctx); err != nil {
			return err
		}
		return fn(uow.tx)
	}
	return own(ctx, handle, &sql.TxOptions{ReadOnly: true}, func(uow *unitOfWork) error { return fn(uow.tx) })
}

// Write runs fn on the unit of work in ctx, or in a transaction on pool.W that carries the tenant scope of ctx.
func Write(ctx context.Context, pool Pool, fn func(q qrm.DB) error) error {
	return InTx(ctx, pool, func(tx *sql.Tx) error { return fn(tx) })
}
