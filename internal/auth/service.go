package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/user"
	"altalune.id/oagent/password"
	"altalune.id/oagent/scope"
)

// Service signs people in, checks basic credentials and rebuilds authority.
type Service struct {
	deps        Deps
	verify      func(hash, plain string) error
	verifyDummy func(plain string)
	now         func() time.Time
}

// NewService builds a Service over deps.
func NewService(deps Deps) *Service {
	return &Service{
		deps:        deps,
		verify:      password.Verify,
		verifyDummy: password.VerifyDummy,
		now:         func() time.Time { return time.Now().UTC() },
	}
}

// LoginLocal checks an email and password and returns the signed-in Principal. Every failure is an *InvalidCredentialsError.
func (s *Service) LoginLocal(ctx context.Context, c Credentials) (session.Principal, error) {
	if strings.TrimSpace(c.Email) == "" || c.Password == "" {
		return session.Principal{}, &InvalidCredentialsError{}
	}
	email, err := user.NormalizeEmail(c.Email)
	if err != nil {
		s.verifyDummy(c.Password)
		return session.Principal{}, &InvalidCredentialsError{}
	}
	u, err := s.deps.Users.ByEmail(ctx, email)
	switch {
	case user.IsNotFoundError(err):
		s.verifyDummy(c.Password)
		return session.Principal{}, &InvalidCredentialsError{}
	case err != nil:
		return session.Principal{}, err
	case !u.HasPassword():
		s.verifyDummy(c.Password)
		return session.Principal{}, &InvalidCredentialsError{}
	}
	if err := s.verify(u.PasswordHash, c.Password); err != nil {
		return session.Principal{}, &InvalidCredentialsError{}
	}
	return s.Refresh(ctx, principalOf(u, s.now()))
}

// Basic checks the shared basic-mode credentials and returns the operator Principal.
func (s *Service) Basic(ctx context.Context, username, pass string) (session.Principal, error) {
	want := s.deps.Basic
	if want.User == "" || want.Password == "" {
		return session.Principal{}, &InvalidCredentialsError{}
	}
	userOK := equalHashed(username, want.User)
	passOK := equalHashed(pass, want.Password)
	if userOK&passOK != 1 {
		return session.Principal{}, &InvalidCredentialsError{}
	}
	return s.Refresh(ctx, session.Principal{Source: session.SourceBasic, Name: want.User, IssuedAt: s.now()})
}

// Refresh rebuilds p's Scopes, Operator flag and active org and project from the database; API key Principals are returned as-is.
func (s *Service) Refresh(ctx context.Context, p session.Principal) (session.Principal, error) {
	switch {
	case p.Source == session.SourceAPIKey:
		return p, nil
	case p.IsPerson():
		return s.refreshPerson(ctx, withoutAuthority(p))
	case p.Source == session.SourceBasic:
		return s.refreshBasic(ctx, withoutAuthority(p))
	}
	return withoutAuthority(p), nil
}

// SetPassword replaces the password of the user with email and signs out every session of theirs.
func (s *Service) SetPassword(ctx context.Context, email, plain string) error {
	u, err := s.deps.Users.FindByEmail(ctx, email)
	if err != nil {
		return err
	}
	if err := s.deps.Passwords.SetPassword(ctx, u.ID, plain); err != nil {
		return err
	}
	return s.deps.Sessions.DeleteByUser(ctx, u.ID)
}

// UpdateProfile sets the person's name and returns them.
func (s *Service) UpdateProfile(ctx context.Context, userID uuid.UUID, name string) (*user.User, error) {
	return s.deps.Profiles.SetName(ctx, userID, name)
}

// ChangePassword checks current, sets next and ends every session but keepSID, in one unit of work; a wrong current is AUT001.
func (s *Service) ChangePassword(ctx context.Context, userID uuid.UUID, keepSID, current, next string) error {
	u, err := s.deps.Profiles.ByID(ctx, userID)
	if err != nil {
		return err
	}
	if !u.HasPassword() {
		s.verifyDummy(current)
		return &InvalidCredentialsError{}
	}
	if err := s.verify(u.PasswordHash, current); err != nil {
		return &InvalidCredentialsError{}
	}
	return s.deps.Transactor.RunInTx(ctx, func(ctx context.Context) error {
		if err := s.deps.Passwords.SetPassword(ctx, u.ID, next); err != nil {
			return err
		}
		if err := s.deps.Sessions.DeleteByUserExcept(ctx, u.ID, keepSID); err != nil {
			return err
		}
		return passwordChanged(ctx, s.deps.Mail, s.deps.From, s.deps.BaseURL, u)
	})
}

// SignOutEverywhere ends every session of the person.
func (s *Service) SignOutEverywhere(ctx context.Context, userID uuid.UUID) error {
	return s.deps.Sessions.DeleteByUser(ctx, userID)
}

// Logout deletes the session sid.
func (s *Service) Logout(ctx context.Context, sid string) error {
	return s.deps.Sessions.Delete(ctx, sid)
}

// SECURITY: hashing first gives equal-length inputs, so the compare time does not leak the expected length.
func equalHashed(got, want string) int {
	g, w := sha256.Sum256([]byte(got)), sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(g[:], w[:])
}

func principalOf(u *user.User, now time.Time) session.Principal {
	p := session.Principal{
		UserID:    u.ID,
		Email:     u.Email,
		Name:      u.Name,
		AvatarURL: u.AvatarURL,
		Source:    session.SourceLocal,
		Locale:    u.Locale,
		IssuedAt:  now,
	}
	if u.TermsAcceptedAt != nil {
		p.TermsAcceptedAt = *u.TermsAcceptedAt
	}
	return p
}

func (s *Service) refreshPerson(ctx context.Context, p session.Principal) (session.Principal, error) {
	ms, err := s.deps.Members.ListByUser(ctx, p.UserID)
	if err != nil {
		return session.Principal{}, err
	}
	if len(ms) == 0 {
		p.ActiveOrgID, p.ActiveProjectID = uuid.Nil, uuid.Nil
		return p, nil
	}
	active := ms[0]
	if i := slices.IndexFunc(ms, func(m *member.Membership) bool { return m.OrgID == p.ActiveOrgID }); i >= 0 {
		active = ms[i]
	}
	if p.Operator, err = s.managesSystemOrg(ctx, ms); err != nil {
		return session.Principal{}, err
	}
	p.Scopes = member.BuiltinPermissions(active.Role)
	return s.withActiveOrg(ctx, p, active.OrgID)
}

func (s *Service) refreshBasic(ctx context.Context, p session.Principal) (session.Principal, error) {
	sys, err := s.deps.Orgs.System(ctx)
	if err != nil {
		return session.Principal{}, err
	}
	p.Operator, p.Scopes = true, []string{"*"}
	return s.withActiveOrg(ctx, p, sys.ID)
}

// SECURITY: only a person who holds member:write in the system org is an operator; managing any other org is not enough.
func (s *Service) managesSystemOrg(ctx context.Context, ms []*member.Membership) (bool, error) {
	sys, err := s.deps.Orgs.System(ctx)
	if org.IsNotFoundError(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(ms, func(m *member.Membership) bool {
		return m.OrgID == sys.ID && scope.Grants(member.BuiltinPermissions(m.Role), scope.MemberWrite)
	}), nil
}

func (s *Service) withActiveOrg(ctx context.Context, p session.Principal, orgID uuid.UUID) (session.Principal, error) {
	projects, err := s.deps.Projects.List(ctx, orgID)
	if err != nil {
		return session.Principal{}, err
	}
	keep := p.ActiveOrgID == orgID && slices.ContainsFunc(projects, func(pr *project.Project) bool { return pr.ID == p.ActiveProjectID })
	p.ActiveOrgID = orgID
	switch {
	case keep:
	case len(projects) > 0:
		p.ActiveProjectID = projects[0].ID
	default:
		p.ActiveProjectID = uuid.Nil
	}
	return p, nil
}

func withoutAuthority(p session.Principal) session.Principal {
	p.Scopes, p.Operator, p.ProjectIDs, p.AllProjects, p.ResourceIDs = nil, false, nil, false, nil
	return p
}
