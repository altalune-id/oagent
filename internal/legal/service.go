package legal

import (
	"context"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/nanoid"
)

const publicIDLen = 21

// Service serves the legal documents and records acceptance of the terms.
type Service struct {
	users    TermsAcceptor
	store    Store
	uow      db.Transactor
	external *time.Time
	terms    *Snapshot
	privacy  *Snapshot
}

// NewService builds a Service; users may be nil when acceptance is never asked for.
func NewService(users TermsAcceptor) *Service {
	return &Service{users: users}
}

// WithStore keeps snapshots of the built-in documents and the acceptance history in store.
func (s *Service) WithStore(store Store, uow db.Transactor) *Service {
	s.store, s.uow = store, uow
	return s
}

// WithExternalTerms says the terms live at an external URL, last changed at updated; a zero updated never asks again.
func (s *Service) WithExternalTerms(updated time.Time) *Service {
	s.external = &updated
	return s
}

// Publish stores the built-in documents' current versions; it fails when a stored version's text differs.
func (s *Service) Publish(ctx context.Context) error {
	if s.store == nil || s.external != nil {
		return nil
	}
	m, err := docs()
	if err != nil {
		return err
	}
	published := map[string]*Snapshot{}
	for _, slug := range []string{TermsSlug, PrivacySlug} {
		d := m[slug]
		got, err := s.store.Publish(ctx, Snapshot{
			ID: uuid.New(), Slug: slug, Version: d.Version, Title: d.Title, Body: d.Source, SHA256: d.SHA256,
		})
		if err != nil {
			return err
		}
		if got.SHA256 != d.SHA256 {
			return &VersionChangedError{Slug: slug, Version: d.Version}
		}
		published[slug] = &got
	}
	s.terms, s.privacy = published[TermsSlug], published[PrivacySlug]
	return nil
}

// TermsUpdatedAt is when the terms or the privacy policy last changed.
func (s *Service) TermsUpdatedAt() time.Time {
	if s.external != nil {
		return *s.external
	}
	if s.terms != nil {
		return latestOf(s.terms.PublishedAt, s.privacy.PublishedAt)
	}
	m, err := docs()
	if err != nil {
		return time.Time{}
	}
	return latest(m)
}

// NeedsAcceptance reports whether a person who accepted the terms at acceptedAt must accept them again.
func (s *Service) NeedsAcceptance(acceptedAt time.Time) bool {
	return needsAcceptance(acceptedAt, s.TermsUpdatedAt())
}

func needsAcceptance(acceptedAt, termsUpdated time.Time) bool {
	return acceptedAt.IsZero() || acceptedAt.Before(termsUpdated)
}

// Document returns the document for slug.
func (s *Service) Document(slug string) (*Document, error) {
	return BySlug(slug)
}

// CanAccept reports whether the service can record acceptance.
func (s *Service) CanAccept() bool { return s.users != nil }

// Accept records that userID accepted the current terms at source and returns when; it joins the transaction on ctx.
func (s *Service) Accept(ctx context.Context, userID uuid.UUID, source Source) (time.Time, error) {
	var at time.Time
	err := s.inTx(ctx, func(ctx context.Context) error {
		u, err := s.users.AcceptTerms(ctx, userID)
		if err != nil {
			return err
		}
		at = *u.TermsAcceptedAt
		if s.terms == nil {
			return nil
		}
		publicID, err := nanoid.New(publicIDLen)
		if err != nil {
			return err
		}
		return s.store.RecordAcceptance(ctx, Acceptance{
			ID: uuid.New(), PublicID: publicID, UserID: userID,
			TermsDocumentID: s.terms.ID, PrivacyDocumentID: s.privacy.ID, Source: source, AcceptedAt: at,
		})
	})
	if err != nil {
		return time.Time{}, err
	}
	return at, nil
}

func (s *Service) inTx(ctx context.Context, fn func(context.Context) error) error {
	if s.store == nil {
		return fn(ctx)
	}
	return s.uow.RunInTx(ctx, fn)
}

func latestOf(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}
