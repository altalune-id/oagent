package member

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/org/member/views"
	"altalune.id/oagent/mailer"
	"altalune.id/oagent/scope"
)

var errNoDirectory = errors.New("member: no directory wired")

// Service is the membership application service.
type Service struct {
	store      Store
	uow        db.Transactor
	unexpected apperror.UnexpectedFunc
	effects    *Effects
	directory  Directory
	afterLock  func()
	opSupport  bool
}

// NewService builds a Service over store; role changes and removals run as units of work on uow.
func NewService(store Store, uow db.Transactor, unexpected apperror.UnexpectedFunc) *Service {
	return &Service{store: store, uow: uow, unexpected: unexpected}
}

// WithEffects sets what removals and role changes do besides the membership row; without it they only change the row.
func (s *Service) WithEffects(e Effects) *Service {
	s.effects = &e
	return s
}

// WithDirectory sets how AddByEmail finds people and orgs.
func (s *Service) WithDirectory(d Directory) *Service {
	s.directory = d
	return s
}

// WithOperatorSupport sets whether staff (an operator who is a person) may manage, as an admin, the members of orgs they do not belong to.
func (s *Service) WithOperatorSupport(on bool) *Service {
	s.opSupport = on
	return s
}

// Add makes userID a member of orgID with role.
func (s *Service) Add(ctx context.Context, orgID, userID uuid.UUID, role Role) (*Membership, error) {
	m, err := New(orgID, userID, role)
	if err != nil {
		return nil, err
	}
	if err := s.store.Add(ctx, m); err != nil {
		return nil, s.errOrReport(ctx, "member: add", err)
	}
	return m, nil
}

// Find returns the membership of userID in orgID.
func (s *Service) Find(ctx context.Context, orgID, userID uuid.UUID) (*Membership, error) {
	m, err := s.store.Find(ctx, orgID, userID)
	if err != nil {
		return nil, s.errOrReport(ctx, "member: find", err)
	}
	return m, nil
}

// ListByUser returns userID's memberships, oldest first.
func (s *Service) ListByUser(ctx context.Context, userID uuid.UUID) ([]*Membership, error) {
	ms, err := s.store.ListByUser(ctx, userID)
	if err != nil {
		return nil, s.errOrReport(ctx, "member: list by user", err)
	}
	return ms, nil
}

// CountOwners returns how many owners orgID has.
func (s *Service) CountOwners(ctx context.Context, orgID uuid.UUID) (int, error) {
	n, err := s.store.CountOwners(ctx, orgID)
	if err != nil {
		return 0, s.errOrReport(ctx, "member: count owners", err)
	}
	return n, nil
}

// List returns orgID's members with their email and name, owners first, then admins, then members.
func (s *Service) List(ctx context.Context, orgID uuid.UUID) ([]Member, error) {
	ms, err := s.store.ListByOrg(ctx, orgID)
	if err != nil {
		return nil, s.errOrReport(ctx, "member: list", err)
	}
	return ms, nil
}

// SetRole changes userID's role in orgID to the role to, on behalf of r; the org always keeps an owner.
func (s *Service) SetRole(ctx context.Context, r Requester, orgID, userID uuid.UUID, to Role) (*Membership, error) {
	if !to.IsValid() {
		return nil, &InvalidRoleError{Role: string(to)}
	}
	reason := LastOwnerDemote
	if to == RoleOwner {
		reason = ""
	}
	var out *Membership
	err := s.guarded(ctx, orgID, userID, reason, s.authorizer(r, orgID, to),
		func(ctx context.Context, m *Membership) error {
			from := m.Role
			if err := s.store.UpdateRole(ctx, orgID, userID, to); err != nil {
				return err
			}
			m.Role = to
			out = m
			if from == to {
				return nil
			}
			return s.roleChanged(ctx, orgID, userID, to, !scope.Covers(BuiltinPermissions(to), BuiltinPermissions(from)))
		})
	if err != nil {
		return nil, s.errOrReport(ctx, "member: set role", err)
	}
	return out, nil
}

// Remove takes userID out of orgID on behalf of r; the org always keeps an owner.
func (s *Service) Remove(ctx context.Context, r Requester, orgID, userID uuid.UUID) error {
	err := s.guarded(ctx, orgID, userID, LastOwnerRemove, s.authorizer(r, orgID, ""),
		func(ctx context.Context, _ *Membership) error {
			if err := s.store.Delete(ctx, orgID, userID); err != nil {
				return err
			}
			return s.removed(ctx, orgID, userID)
		})
	if err != nil {
		return s.errOrReport(ctx, "member: remove", err)
	}
	return nil
}

// Leave takes userID out of orgID at their own request, unless they are its last owner.
func (s *Service) Leave(ctx context.Context, orgID, userID uuid.UUID) error {
	err := s.guarded(ctx, orgID, userID, LastOwnerLeave, nil,
		func(ctx context.Context, _ *Membership) error {
			if err := s.store.Delete(ctx, orgID, userID); err != nil {
				return err
			}
			return s.revokeKeys(ctx, orgID, userID)
		})
	if err != nil {
		return s.errOrReport(ctx, "member: leave", err)
	}
	return nil
}

// AddExisting makes the existing user userID a member of orgID with role, on behalf of r; an existing membership is never changed.
func (s *Service) AddExisting(ctx context.Context, r Requester, orgID, userID uuid.UUID, role Role) (*Membership, error) {
	if !role.IsValid() {
		return nil, &InvalidRoleError{Role: string(role)}
	}
	m, err := New(orgID, userID, role)
	if err != nil {
		return nil, err
	}
	authorize := s.authorizer(r, orgID, role)
	err = s.uow.RunInTx(ctx, func(ctx context.Context) error {
		if err := authorize(ctx, nil, &Membership{OrgID: orgID, UserID: userID}); err != nil {
			return err
		}
		return s.store.Add(ctx, m)
	})
	if err != nil {
		return nil, s.errOrReport(ctx, "member: add existing", err)
	}
	return m, nil
}

// AddFromInvite makes userID a member of orgID with role unless they already belong to it; it joins the transaction on ctx.
func (s *Service) AddFromInvite(ctx context.Context, orgID, userID uuid.UUID, role Role) (added bool, err error) {
	m, err := New(orgID, userID, role)
	if err != nil {
		return false, err
	}
	added, err = s.store.AddIfAbsent(ctx, m)
	if err != nil {
		return false, s.errOrReport(ctx, "member: add from invite", err)
	}
	return added, nil
}

// ActorFor resolves r for orgID with the rules SetRole and Remove use; it joins the transaction on ctx, where the membership row is locked.
func (s *Service) ActorFor(ctx context.Context, r Requester, orgID uuid.UUID) (Actor, error) {
	a, err := s.actor(ctx, r, orgID, nil)
	if err != nil {
		return Actor{}, s.errOrReport(ctx, "member: actor", err)
	}
	return a, nil
}

// AddByEmail makes the existing user with email a member of the org orgPublicID with role, on behalf of r.
func (s *Service) AddByEmail(ctx context.Context, r Requester, email, orgPublicID string, role Role) (*Member, error) {
	if s.directory == nil {
		return nil, s.unexpected(ctx, "member: add by email", errNoDirectory)
	}
	userID, err := s.directory.UserIDByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	userPublicID, err := s.directory.UserPublicID(ctx, userID)
	if err != nil {
		return nil, s.errOrReport(ctx, "member: add by email", err)
	}
	orgID, err := s.directory.OrgIDByPublicID(ctx, orgPublicID)
	if err != nil {
		return nil, err
	}
	m, err := s.AddExisting(ctx, r, orgID, userID, role)
	if err != nil {
		return nil, err
	}
	return &Member{Membership: *m, UserPublicID: userPublicID, Email: email}, nil
}

func (s *Service) guarded(
	ctx context.Context, orgID, userID uuid.UUID, reason LastOwnerReason,
	authorize func(ctx context.Context, owners []uuid.UUID, m *Membership) error, apply func(context.Context, *Membership) error,
) error {
	return s.uow.RunInTx(ctx, func(ctx context.Context) error {
		owners, err := s.store.LockOwners(ctx, orgID)
		if err != nil {
			return err
		}
		if s.afterLock != nil {
			s.afterLock()
		}
		m, err := s.store.Find(ctx, orgID, userID)
		if err != nil {
			return err
		}
		if authorize != nil {
			if err := authorize(ctx, owners, m); err != nil {
				return err
			}
		}
		if reason != "" && m.Role == RoleOwner && len(owners) <= 1 {
			return &LastOwnerError{Reason: reason}
		}
		return apply(ctx, m)
	})
}

func (s *Service) authorizer(r Requester, orgID uuid.UUID, to Role) func(context.Context, []uuid.UUID, *Membership) error {
	return func(ctx context.Context, owners []uuid.UUID, target *Membership) error {
		a, err := s.actor(ctx, r, orgID, owners)
		if err != nil {
			return err
		}
		return Authorize(a, target, to)
	}
}

// SECURITY: a person acts with their role's permissions, read under the lock; staff act as admin only outside their orgs, with support on.
func (s *Service) actor(ctx context.Context, r Requester, orgID uuid.UUID, owners []uuid.UUID) (Actor, error) {
	if r.UserID == uuid.Nil {
		if r.Operator {
			return Actor{Operator: true}, nil
		}
		return Actor{}, &authz.ForbiddenError{Permission: scope.MemberWrite}
	}
	a := Actor{UserID: r.UserID}
	if slices.Contains(owners, r.UserID) {
		a.Permissions = BuiltinPermissions(RoleOwner)
		return a, nil
	}
	m, err := s.store.LockMember(ctx, orgID, r.UserID)
	switch {
	case err == nil:
		a.Permissions = BuiltinPermissions(m.Role)
	case IsNotFoundError(err):
		if r.Operator && s.opSupport {
			a.Permissions = BuiltinPermissions(RoleAdmin)
		}
	default:
		return Actor{}, err
	}
	return a, nil
}

func (s *Service) removed(ctx context.Context, orgID, userID uuid.UUID) error {
	if s.effects == nil {
		return nil
	}
	if err := s.effects.Sessions.DeleteByUser(ctx, userID); err != nil {
		return fmt.Errorf("member: end sessions: %w", err)
	}
	if err := s.revokeKeys(ctx, orgID, userID); err != nil {
		return err
	}
	return s.notify(ctx, orgID, userID, func(org string) mailer.Message {
		var html strings.Builder
		_ = views.RemovedHTML(org).Render(ctx, &html)
		return mailer.Message{Subject: views.RemovedSubject(org), TextBody: views.RemovedText(org), HTMLBody: html.String()}
	})
}

func (s *Service) roleChanged(ctx context.Context, orgID, userID uuid.UUID, to Role, demoted bool) error {
	if s.effects == nil {
		return nil
	}
	if demoted {
		if err := s.effects.Sessions.DeleteByUser(ctx, userID); err != nil {
			return fmt.Errorf("member: end sessions: %w", err)
		}
	}
	console := strings.TrimSuffix(s.effects.BaseURL, "/") + "/console/"
	role := to.Label()
	return s.notify(ctx, orgID, userID, func(org string) mailer.Message {
		var html strings.Builder
		_ = views.RoleChangedHTML(org, role, console).Render(ctx, &html)
		return mailer.Message{
			Subject: views.RoleChangedSubject(org, role), TextBody: views.RoleChangedText(org, role, console), HTMLBody: html.String(),
		}
	})
}

func (s *Service) revokeKeys(ctx context.Context, orgID, userID uuid.UUID) error {
	if s.effects == nil {
		return nil
	}
	if _, err := s.effects.Keys.RevokePersonalOf(ctx, orgID, userID); err != nil {
		return fmt.Errorf("member: revoke personal keys: %w", err)
	}
	return nil
}

func (s *Service) notify(ctx context.Context, orgID, userID uuid.UUID, build func(org string) mailer.Message) error {
	email, _, err := s.effects.People.Contact(ctx, userID)
	if err != nil {
		return fmt.Errorf("member: notice contact: %w", err)
	}
	name, err := s.effects.Orgs.Name(ctx, orgID)
	if err != nil {
		return fmt.Errorf("member: notice org: %w", err)
	}
	m := build(strings.Join(strings.Fields(name), " "))
	m.To, m.From = email, s.effects.From
	if _, err := s.effects.Mail.Enqueue(ctx, m); err != nil {
		return fmt.Errorf("member: queue notice: %w", err)
	}
	return nil
}

func (s *Service) errOrReport(ctx context.Context, message string, err error) error {
	if IsExpectedError(err) {
		return err
	}
	return s.unexpected(ctx, message, err)
}
