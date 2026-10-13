package fakes

import "context"

// Transactor runs each unit of work by calling fn directly, for services over fake stores.
type Transactor struct{}

// RunInTx calls fn with ctx.
func (Transactor) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}
