package storage

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/scheduler"
)

const (
	sweepEvery   = 5 * time.Minute
	sweepJitter  = 30 * time.Second
	sweepTimeout = 2 * time.Minute
	sweepBatch   = 100
	sweepJobName = "storage-sweep"
)

// Sweep deletes, in the org on ctx, up to 100 deleting or expired pending files; abandoned hears of each expired upload first.
func (s *Service) Sweep(ctx context.Context, abandoned Abandoned) (int, error) {
	if !s.Enabled() {
		return 0, nil
	}
	files, err := s.store.Sweepable(ctx, s.now(), sweepBatch)
	if err != nil {
		return 0, err
	}
	var errs []error
	n := 0
	for _, f := range files {
		claimed, err := s.claim(ctx, f.ID, abandoned)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if claimed == nil {
			continue
		}
		if err := s.driver.Delete(ctx, claimed.ObjectKey); err != nil {
			errs = append(errs, err)
			continue
		}
		if err := s.store.DeleteDeleting(ctx, claimed.ID); err != nil {
			errs = append(errs, err)
			continue
		}
		n++
	}
	return n, errors.Join(errs...)
}

// SECURITY: re-checks the file under its row lock before deleting, so a file completed after the listing is never swept.
func (s *Service) claim(ctx context.Context, id uuid.UUID, abandoned Abandoned) (*File, error) {
	var claimed *File
	err := s.uow.RunInTx(ctx, func(ctx context.Context) error {
		f, err := s.store.LockSweepable(ctx, id, s.now())
		if err != nil || f == nil {
			return err
		}
		if f.Status == StatusPending {
			if abandoned != nil {
				if err := abandoned.UploadAbandoned(ctx, f.ID); err != nil {
					return err
				}
			}
			if err := s.store.MarkDeleting(ctx, f.ID); err != nil {
				return err
			}
		}
		claimed = f
		return nil
	})
	return claimed, err
}

// Jobs returns the per-tenant storage-sweep job; a disabled Service has none.
func (s *Service) Jobs(abandoned Abandoned) []scheduler.Job {
	if !s.Enabled() {
		return nil
	}
	return []scheduler.Job{{
		Name: sweepJobName, Scope: scheduler.ScopeTenant, Schedule: scheduler.MustEveryInterval(sweepEvery, sweepJitter),
		Timeout: sweepTimeout, Singleton: true,
		Run: func(ctx context.Context) error {
			_, err := s.Sweep(ctx, abandoned)
			return err
		},
	}}
}
