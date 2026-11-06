package fakes

import (
	"context"

	"altalune.id/oagent/internal/kernel/db"
)

type txKey struct{}

// InTx reports whether ctx is inside a unit of work, run by a fake transactor or by Postgres.
func InTx(ctx context.Context) bool {
	if in, _ := ctx.Value(txKey{}).(bool); in {
		return true
	}
	_, ok := db.CurrentTx(ctx)
	return ok
}

// Transactor runs each unit of work by calling fn directly, for services over fake stores.
type Transactor struct{}

// RunInTx calls fn with ctx marked as inside a unit of work.
func (Transactor) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(context.WithValue(ctx, txKey{}, true))
}

// CountingTransactor is a Transactor that counts the units of work it ran.
type CountingTransactor struct {
	Calls int
}

// RunInTx calls fn with ctx marked as inside a unit of work.
func (t *CountingTransactor) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	t.Calls++
	return Transactor{}.RunInTx(ctx, fn)
}
