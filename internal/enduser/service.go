package enduser

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
)

var (
	errRaced        = errors.New("enduser: identity claimed by a concurrent upsert")
	errNestedUpsert = errors.New("enduser: Upsert retries in its own transaction and cannot join the caller's")
)

// Service is the end user application service.
type Service struct {
	store      Store
	uow        db.Transactor
	unexpected apperror.UnexpectedFunc
	log        *slog.Logger
	hints      KeyHints
}

// WithKeyHints labels an end user that is an API key by the key's hint.
func (s *Service) WithKeyHints(h KeyHints) *Service {
	s.hints = h
	return s
}

// NewService builds a Service over store.
func NewService(store Store, uow db.Transactor, unexpected apperror.UnexpectedFunc) *Service {
	return &Service{store: store, uow: uow, unexpected: unexpected, log: slog.Default()}
}

// WithLogger logs identity conflicts to l.
func (s *Service) WithLogger(l *slog.Logger) *Service {
	s.log = l
	return s
}

// Get loads an end user of the org on ctx.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*EndUser, error) {
	e, err := s.store.ByID(ctx, id)
	if err != nil {
		return nil, s.errOrReport(ctx, "enduser: get", err)
	}
	return e, nil
}

// Lookup returns the end user of projectID known by id, or a *NotFoundError; it never creates one.
func (s *Service) Lookup(ctx context.Context, projectID uuid.UUID, id Identity) (*EndUser, error) {
	if err := id.Validate(); err != nil {
		return nil, err
	}
	owners, err := s.store.Owners(ctx, projectID, []Identity{id})
	if err != nil {
		return nil, s.errOrReport(ctx, "enduser: lookup", err)
	}
	owner, ok := owners[id]
	if !ok {
		return nil, &NotFoundError{ID: string(id.Kind)}
	}
	return s.Get(ctx, owner)
}

// Upsert resolves ids to one end user of projectID, creating it and linking the unknown identities; call it outside a transaction.
func (s *Service) Upsert(ctx context.Context, projectID uuid.UUID, ids ...Identity) (*EndUser, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	if _, inTx := db.CurrentTx(ctx); inTx {
		return nil, errNestedUpsert
	}
	if len(ids) == 0 {
		return nil, &InvalidIdentityError{Reason: IdentityValue}
	}
	for _, id := range ids {
		if err := id.Validate(); err != nil {
			return nil, err
		}
	}
	ids = unique(ids)
	var e *EndUser
	for range 2 {
		err = s.uow.RunInTx(ctx, func(ctx context.Context) error {
			e, err = s.upsert(ctx, tc.OrgID, projectID, ids)
			return err
		})
		if !errors.Is(err, errRaced) {
			break
		}
	}
	if err != nil {
		return nil, s.errOrReport(ctx, "enduser: upsert", err)
	}
	return e, nil
}

func (s *Service) upsert(ctx context.Context, orgID, projectID uuid.UUID, ids []Identity) (*EndUser, error) {
	owners, err := s.store.Owners(ctx, projectID, ids)
	if err != nil {
		return nil, err
	}
	var owner uuid.UUID
	conflict := false
	for _, id := range ids {
		o, ok := owners[id]
		if !ok {
			continue
		}
		if owner == uuid.Nil {
			owner = o
		}
		conflict = conflict || o != owner
	}
	if conflict {
		kinds := make([]string, 0, len(ids))
		for _, id := range ids {
			if _, ok := owners[id]; ok {
				kinds = append(kinds, string(id.Kind))
			}
		}
		s.log.WarnContext(ctx, "enduser: identities point at different end users; using the first", "kinds", kinds)
	}
	e, err := s.endUser(ctx, orgID, projectID, owner)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		if _, ok := owners[id]; ok {
			continue
		}
		added, err := s.store.AddIdentity(ctx, e, id)
		if err != nil {
			return nil, err
		}
		if !added {
			return nil, errRaced
		}
	}
	return e, nil
}

func (s *Service) endUser(ctx context.Context, orgID, projectID, owner uuid.UUID) (*EndUser, error) {
	if owner != uuid.Nil {
		return s.store.ByID(ctx, owner)
	}
	e := New(orgID, projectID, time.Now())
	if err := s.store.Save(ctx, e); err != nil {
		return nil, err
	}
	return e, nil
}

func unique(ids []Identity) []Identity {
	seen := make(map[Identity]bool, len(ids))
	out := make([]Identity, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func (s *Service) errOrReport(ctx context.Context, message string, err error) error {
	if IsExpectedError(err) {
		return err
	}
	return s.unexpected(ctx, message, err)
}

// Label names an end user by its first identity's kind and a masked tail; an end user that is gone has no label.
func (s *Service) Label(ctx context.Context, id uuid.UUID) (string, error) {
	ids, err := s.store.IdentitiesOf(ctx, id)
	if err != nil {
		return "", s.errOrReport(ctx, "enduser: label", err)
	}
	if len(ids) == 0 {
		return "", nil
	}
	return s.label(ctx, ids[0])
}

// SECURITY: a key's end user shows the key's public hint, never its id.
func (s *Service) label(ctx context.Context, id Identity) (string, error) {
	raw, isKey := strings.CutPrefix(id.Value, keyActorPrefix)
	if s.hints == nil || id.Kind != KindAPIUser || !isKey {
		return Label(id), nil
	}
	keyID, ok := parseKeyID(raw)
	if !ok {
		return Label(id), nil
	}
	hint, err := s.hints.KeyHint(ctx, keyID)
	if err != nil {
		return "", s.errOrReport(ctx, "enduser: key hint", err)
	}
	if hint == "" {
		return keyLabel, nil
	}
	return keyLabel + " …" + hint, nil
}

func parseKeyID(raw string) (uuid.UUID, bool) {
	id, err := uuid.Parse(raw)
	return id, err == nil
}

const (
	keyActorPrefix = "key:"
	keyLabel       = "api_key"
)
