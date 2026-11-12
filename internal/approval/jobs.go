package approval

import (
	"context"
	"errors"
	"time"

	"altalune.id/oagent/scheduler"
)

const (
	expireJobName      = "approval-expire"
	expireBatch        = 100
	defaultExpireEvery = 15 * time.Second
)

// Jobs returns the per-tenant approval-expire job, which expires overdue approvals and kicks each thread so its run resumes.
func (s *Service) Jobs(every time.Duration) []scheduler.Job {
	if every <= 0 {
		every = defaultExpireEvery
	}
	return []scheduler.Job{{
		Name: expireJobName, Scope: scheduler.ScopeTenant, Schedule: scheduler.MustEveryInterval(every, every/5), Timeout: every,
		Run: s.expireDue,
	}}
}

func (s *Service) expireDue(ctx context.Context) error {
	ids, err := s.store.DueExpiries(ctx, expireBatch)
	if err != nil {
		return s.errOrReport(ctx, "approval: due expiries", err)
	}
	var errs []error
	for _, id := range ids {
		a, err := s.expire(ctx, id)
		if err != nil || a == nil || s.kicker == nil {
			errs = append(errs, err)
			continue
		}
		errs = append(errs, s.kicker.Kick(ctx, a.OrgID, a.ThreadID))
	}
	return errors.Join(errs...)
}
