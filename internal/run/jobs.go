package run

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/scheduler"
)

const (
	reconcileBatch    = 100
	defaultSweepEvery = 15 * time.Second
)

// Jobs returns the per-tenant jobs run-sweep (reclaims expired leases) and run-reconcile (kicks what a kick missed), every 15 s by default.
func Jobs(store Store, k Kicker, every time.Duration) []scheduler.Job {
	if every <= 0 {
		every = defaultSweepEvery
	}
	schedule := scheduler.MustEveryInterval(every, every/5)
	return []scheduler.Job{
		{
			Name: "run-sweep", Scope: scheduler.ScopeTenant, Schedule: schedule, Timeout: every,
			Run: func(ctx context.Context) error {
				ids, err := store.Expired(ctx, time.Now())
				if err != nil {
					return err
				}
				var errs []error
				for _, id := range ids {
					ok, err := store.Reclaim(ctx, id)
					if err == nil && ok {
						err = kick(ctx, k, id)
					}
					errs = append(errs, err)
				}
				return errors.Join(errs...)
			},
		},
		{
			Name: "run-reconcile", Scope: scheduler.ScopeTenant, Schedule: schedule, Timeout: every,
			Run: func(ctx context.Context) error {
				ids, err := store.PendingUnleased(ctx, time.Now(), reconcileBatch)
				if err != nil {
					return err
				}
				var errs []error
				for _, id := range ids {
					errs = append(errs, kick(ctx, k, id))
				}
				return errors.Join(errs...)
			},
		},
	}
}

func kick(ctx context.Context, k Kicker, threadID uuid.UUID) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	return k.Kick(ctx, Kick{OrgID: tc.OrgID, ThreadID: threadID})
}
