package install

import (
	"cmp"
	"context"
	"strings"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/legal"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/user"
)

const (
	lockName       = "oagent.install"
	defaultOrgName = "Default"
)

// Service installs the instance and reports whether it still needs installing.
type Service struct {
	uow      db.Transactor
	users    UserCreator
	orgs     SystemOrgs
	members  Owners
	projects DefaultProjects
	terms    Terms
	invites  OwnerInvites
}

// NewService builds a Service over the domain services it writes through.
func NewService(uow db.Transactor, users UserCreator, orgs SystemOrgs, members Owners, projects DefaultProjects) *Service {
	return &Service{uow: uow, users: users, orgs: orgs, members: members, projects: projects}
}

// WithTerms records the owner's acceptance of the terms through t.
func (s *Service) WithTerms(t Terms) *Service {
	s.terms = t
	return s
}

// WithoutPasswords makes install invite the first owner by email through invites instead of creating them with a password.
func (s *Service) WithoutPasswords(invites OwnerInvites) *Service {
	s.invites = invites
	return s
}

// Passwords reports whether install creates the first owner with a password, or invites them by email.
func (s *Service) Passwords() bool { return s.invites == nil }

// Required reports whether the instance still needs installing: no system org, or in local mode no owner of it nor a pending owner invite.
func (s *Service) Required(ctx context.Context, basic bool) (bool, error) {
	sys, err := s.orgs.System(ctx)
	if org.IsNotFoundError(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if basic {
		return false, nil
	}
	return s.ownerless(ctx, sys.ID)
}

func (s *Service) ownerless(ctx context.Context, orgID uuid.UUID) (bool, error) {
	owners, err := s.members.CountOwners(ctx, orgID)
	switch {
	case err != nil || owners > 0:
		return false, err
	case s.Passwords():
		return true, nil
	}
	pending, err := s.invites.PendingOwner(ctx, orgID)
	return !pending, err
}

// Install sets up the first owner (an invite with passwords off), the system org (claimed or created) and its default project.
func (s *Service) Install(ctx context.Context, r Request) (*user.User, error) {
	var created *user.User
	err := s.uow.RunInTx(ctx, func(ctx context.Context) error {
		if err := db.LockInTx(ctx, lockName); err != nil {
			return err
		}
		sys, err := s.ownerlessSystemOrg(ctx)
		if err != nil {
			return err
		}
		if !s.Passwords() {
			if sys, err = s.systemOrg(ctx, sys, r.OrgName, uuid.Nil); err != nil {
				return err
			}
			if _, err := s.invites.InviteOwner(ctx, sys.ID, r.Email); err != nil {
				return err
			}
			return s.ensureDefaultProject(ctx, sys.ID)
		}
		u, err := s.users.Create(ctx, user.CreateRequest{Email: r.Email, Name: r.Name, Password: r.Password})
		if err != nil {
			return err
		}
		if r.AcceptTerms {
			if _, err := s.terms.Accept(ctx, u.ID, legal.SourceInstall); err != nil {
				return err
			}
		}
		if sys, err = s.systemOrg(ctx, sys, r.OrgName, u.ID); err != nil {
			return err
		}
		if _, err := s.members.Add(ctx, sys.ID, u.ID, member.RoleOwner); err != nil {
			return err
		}
		if err := s.ensureDefaultProject(ctx, sys.ID); err != nil {
			return err
		}
		created = u
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// EnsureBasicSystemOrg creates the ownerless system org and its default project when missing.
func (s *Service) EnsureBasicSystemOrg(ctx context.Context) error {
	return s.uow.RunInTx(ctx, func(ctx context.Context) error {
		if err := db.LockInTx(ctx, lockName); err != nil {
			return err
		}
		sys, err := s.orgs.System(ctx)
		if org.IsNotFoundError(err) {
			sys, err = s.orgs.CreateSystem(ctx, org.CreateRequest{Name: defaultOrgName})
		}
		if err != nil {
			return err
		}
		return s.ensureDefaultProject(ctx, sys.ID)
	})
}

func (s *Service) ownerlessSystemOrg(ctx context.Context) (*org.Org, error) {
	sys, err := s.orgs.System(ctx)
	if org.IsNotFoundError(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ownerless, err := s.ownerless(ctx, sys.ID)
	switch {
	case err != nil:
		return nil, err
	case !ownerless:
		return nil, &AlreadyInstalledError{}
	}
	return sys, nil
}

func (s *Service) systemOrg(ctx context.Context, sys *org.Org, name string, createdBy uuid.UUID) (*org.Org, error) {
	if sys != nil {
		return sys, nil
	}
	return s.orgs.CreateSystem(ctx, org.CreateRequest{Name: cmp.Or(strings.TrimSpace(name), defaultOrgName), CreatedBy: createdBy})
}

func (s *Service) ensureDefaultProject(ctx context.Context, orgID uuid.UUID) error {
	_, err := s.projects.BySlug(ctx, orgID, project.DefaultSlug)
	if err == nil || !project.IsNotFoundError(err) {
		return err
	}
	_, err = s.projects.Create(ctx, project.CreateRequest{OrgID: orgID, Slug: project.DefaultSlug, Name: project.DefaultName})
	return err
}
