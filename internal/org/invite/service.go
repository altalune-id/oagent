package invite

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/legal"
	"altalune.id/oagent/internal/org/invite/views"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/user"
	"altalune.id/oagent/mailer"
)

// Deps are what the invite service needs.
type Deps struct {
	Store      Store
	Transactor db.Transactor
	Members    Members
	Mail       Mail
	Users      Users
	Terms      Terms
	Events     Events
	Orgs       Orgs
	People     People
	From       string
	BaseURL    string
	Unexpected apperror.UnexpectedFunc
}

// Service is the invite application service.
type Service struct {
	store      Store
	uow        db.Transactor
	members    Members
	mail       Mail
	users      Users
	terms      Terms
	events     Events
	orgs       Orgs
	people     People
	from       string
	baseURL    string
	unexpected apperror.UnexpectedFunc
	now        func() time.Time
	afterLive  func()
}

// NewService builds a Service from d.
func NewService(d Deps) *Service {
	return &Service{
		store: d.Store, uow: d.Transactor, members: d.Members, mail: d.Mail, users: d.Users, terms: d.Terms, events: d.Events,
		orgs: d.Orgs, people: d.People, from: d.From, baseURL: strings.TrimSuffix(d.BaseURL, "/"), unexpected: d.Unexpected, now: time.Now,
	}
}

// WithClock sets the clock the service reads.
func (s *Service) WithClock(now func() time.Time) *Service {
	s.now = now
	return s
}

// Create invites email to orgID with role on behalf of r and queues the email; a live invite for that email is replaced.
func (s *Service) Create(ctx context.Context, r member.Requester, orgID uuid.UUID, email string, role member.Role) (*Created, error) {
	email, err := user.NormalizeEmail(email)
	if err != nil {
		return nil, err
	}
	if !role.IsValid() {
		return nil, &member.InvalidRoleError{Role: string(role)}
	}
	var out *Created
	err = s.uow.RunInTx(ctx, func(ctx context.Context) error {
		a, err := s.actor(ctx, r, orgID, role)
		if err != nil {
			return err
		}
		inv, err := s.claim(ctx, a, orgID, email)
		if err != nil {
			return err
		}
		inv.Role = role
		out, err = s.issue(ctx, inv)
		return err
	})
	if err != nil {
		return nil, s.errOrReport(ctx, "invite: create", err)
	}
	return out, nil
}

// Resend gives a pending or expired invite a new link and expiry, and emails it again.
func (s *Service) Resend(ctx context.Context, r member.Requester, orgID, inviteID uuid.UUID) (*Created, error) {
	var out *Created
	err := s.uow.RunInTx(ctx, func(ctx context.Context) error {
		inv, err := s.locked(ctx, r, orgID, inviteID, false)
		if err != nil {
			return err
		}
		out, err = s.issue(ctx, inv)
		return err
	})
	if err != nil {
		return nil, s.errOrReport(ctx, "invite: resend", err)
	}
	return out, nil
}

// Revoke cancels a pending or expired invite; the row is kept.
func (s *Service) Revoke(ctx context.Context, r member.Requester, orgID, inviteID uuid.UUID) error {
	return s.revoke(ctx, r, orgID, inviteID, false)
}

// DiscardDraft cancels a draft.
func (s *Service) DiscardDraft(ctx context.Context, r member.Requester, orgID, inviteID uuid.UUID) error {
	return s.revoke(ctx, r, orgID, inviteID, true)
}

// ListPending returns orgID's pending and expired invites, without drafts, oldest first.
func (s *Service) ListPending(ctx context.Context, orgID uuid.UUID) ([]*Invite, error) {
	return s.list(ctx, orgID, func(i *Invite) bool { return !i.Draft })
}

// ListDrafts returns orgID's drafts that can still be confirmed, oldest first.
func (s *Service) ListDrafts(ctx context.Context, orgID uuid.UUID) ([]*Invite, error) {
	now := s.now()
	return s.list(ctx, orgID, func(i *Invite) bool { return i.Draft && !i.expired(now) })
}

// Draft records an assistant's suggestion to invite email as a member: no token and no email until a person confirms it.
func (s *Service) Draft(ctx context.Context, r member.Requester, orgID uuid.UUID, email string) (*Invite, error) {
	email, err := user.NormalizeEmail(email)
	if err != nil {
		return nil, err
	}
	var out *Invite
	err = s.uow.RunInTx(ctx, func(ctx context.Context) error {
		a, err := s.actor(ctx, r, orgID, member.RoleMember)
		if err != nil {
			return err
		}
		inv, err := s.claim(ctx, a, orgID, email)
		if err != nil {
			return err
		}
		if inv.TokenHash != "" && !inv.expired(s.now()) {
			return &PendingError{}
		}
		inv.draft(s.now())
		out = inv
		return s.save(ctx, inv)
	})
	if err != nil {
		return nil, s.errOrReport(ctx, "invite: draft", err)
	}
	return out, nil
}

// ConfirmDraft turns a draft into an invite with a link and queues the email; a person must call it.
func (s *Service) ConfirmDraft(ctx context.Context, r member.Requester, orgID, inviteID uuid.UUID) (*Created, error) {
	var out *Created
	err := s.uow.RunInTx(ctx, func(ctx context.Context) error {
		inv, err := s.locked(ctx, r, orgID, inviteID, true)
		if err != nil {
			return err
		}
		if inv.expired(s.now()) {
			return &DraftExpiredError{}
		}
		out, err = s.issue(ctx, inv)
		return err
	})
	if err != nil {
		return nil, s.errOrReport(ctx, "invite: confirm draft", err)
	}
	return out, nil
}

// Lookup returns the usable invite the raw token from a link names.
func (s *Service) Lookup(ctx context.Context, rawToken string) (*Invite, error) {
	if rawToken == "" {
		return nil, &NotUsableError{Reason: ReasonNotFound}
	}
	inv, err := s.store.ByTokenHash(ctx, hashToken(rawToken))
	if IsNotFoundError(err) {
		return nil, &NotUsableError{Reason: ReasonNotFound}
	}
	if err != nil {
		return nil, s.errOrReport(ctx, "invite: lookup", err)
	}
	if err := inv.Usable(s.now()); err != nil {
		return nil, err
	}
	return inv, nil
}

// ByPublicID returns the invite with publicID in any state.
func (s *Service) ByPublicID(ctx context.Context, publicID string) (*Invite, error) {
	inv, err := s.store.ByPublicID(ctx, publicID)
	if err != nil {
		return nil, s.errOrReport(ctx, "invite: by public id", err)
	}
	return inv, nil
}

// Accept uses up inviteID for userID and adds them to the org; an existing member keeps their role.
func (s *Service) Accept(ctx context.Context, inviteID, userID uuid.UUID) (Outcome, error) {
	var out Outcome
	err := s.uow.RunInTx(ctx, func(ctx context.Context) error {
		var err error
		out, err = s.accept(ctx, inviteID, userID)
		return err
	})
	if err != nil {
		return 0, s.errOrReport(ctx, "invite: accept", err)
	}
	return out, nil
}

// Join creates the account the invite inviteID names, records the terms when acceptTerms, and accepts the invite, in one transaction.
func (s *Service) Join(ctx context.Context, inviteID uuid.UUID, name, password string, acceptTerms bool) (uuid.UUID, error) {
	var userID uuid.UUID
	err := s.uow.RunInTx(ctx, func(ctx context.Context) error {
		inv, err := s.store.ByID(ctx, inviteID)
		if IsNotFoundError(err) {
			return &NotUsableError{Reason: ReasonNotFound}
		}
		if err != nil {
			return err
		}
		if err := inv.Usable(s.now()); err != nil {
			return err
		}
		// SECURITY: never a second user for an email.
		if _, err := s.users.ByEmail(ctx, inv.Email); !user.IsNotFoundError(err) {
			if err == nil {
				return &user.AlreadyExistsError{Email: inv.Email}
			}
			return err
		}
		u, err := s.users.Create(ctx, user.CreateRequest{Email: inv.Email, Name: name, Password: password})
		if err != nil {
			return err
		}
		if acceptTerms {
			if _, err := s.terms.Accept(ctx, u.ID, legal.SourceJoin); err != nil {
				return err
			}
		}
		if _, err := s.accept(ctx, inv.ID, u.ID); err != nil {
			return err
		}
		userID = u.ID
		return nil
	})
	if err != nil {
		return uuid.Nil, s.errOrReport(ctx, "invite: join", err)
	}
	return userID, nil
}

// SECURITY: MarkAccepted's conditional UPDATE is the only gate on use; never check, then write.
func (s *Service) accept(ctx context.Context, inviteID, userID uuid.UUID) (Outcome, error) {
	email, name, err := s.people.Contact(ctx, userID)
	if err != nil {
		return 0, err
	}
	cur, err := s.store.ByID(ctx, inviteID)
	if IsNotFoundError(err) || (err == nil && cur.Draft) {
		return 0, &NotUsableError{Reason: ReasonNotFound}
	}
	if err != nil {
		return 0, err
	}
	if !strings.EqualFold(email, cur.Email) {
		return 0, &EmailMismatchError{}
	}
	inv, err := s.store.MarkAccepted(ctx, inviteID, userID, s.now())
	if IsNotFoundError(err) {
		return 0, s.whyNotUsable(ctx, inviteID)
	}
	if err != nil {
		return 0, err
	}
	added, err := s.members.AddFromInvite(ctx, inv.OrgID, userID, inv.Role)
	if err != nil {
		return 0, err
	}
	if !added {
		return AlreadyMember, nil
	}
	userPublicID, err := s.people.PublicID(ctx, userID)
	if err != nil {
		return 0, err
	}
	err = s.emit(ctx, inv.OrgID, events.MemberAdded, events.MemberAddedV1{
		UserID: userPublicID, Email: email, Name: name, Role: string(inv.Role), InviteID: inv.PublicID,
	})
	if err != nil {
		return 0, err
	}
	return Joined, nil
}

func (s *Service) whyNotUsable(ctx context.Context, inviteID uuid.UUID) error {
	inv, err := s.store.ByID(ctx, inviteID)
	if err != nil {
		return err
	}
	if err := inv.Usable(s.now()); err != nil {
		return err
	}
	return &NotUsableError{Reason: ReasonNotFound}
}

func (s *Service) actor(ctx context.Context, r member.Requester, orgID uuid.UUID, role member.Role) (member.Actor, error) {
	a, err := s.members.ActorFor(ctx, r, orgID)
	if err != nil {
		return member.Actor{}, err
	}
	return a, Authorize(a, role)
}

func (s *Service) claim(ctx context.Context, a member.Actor, orgID uuid.UUID, email string) (*Invite, error) {
	if err := s.store.LockEmail(ctx, orgID, email); err != nil {
		return nil, err
	}
	inv, err := s.store.Live(ctx, orgID, email)
	if s.afterLive != nil {
		s.afterLive()
	}
	switch {
	case IsNotFoundError(err):
		return newInvite(orgID, email, member.RoleMember, a.UserID, s.now())
	case err != nil:
		return nil, err
	}
	if err := Authorize(a, inv.Role); err != nil {
		return nil, err
	}
	inv.InvitedBy = a.UserID
	return inv, nil
}

func (s *Service) locked(ctx context.Context, r member.Requester, orgID, inviteID uuid.UUID, wantDraft bool) (*Invite, error) {
	a, err := s.members.ActorFor(ctx, r, orgID)
	if err != nil {
		return nil, err
	}
	inv, err := s.store.Lock(ctx, inviteID)
	if err != nil {
		return nil, err
	}
	if inv.OrgID != orgID || !inv.live() || inv.Draft != wantDraft {
		return nil, &NotFoundError{}
	}
	if err := Authorize(a, inv.Role); err != nil {
		return nil, err
	}
	return inv, nil
}

func (s *Service) revoke(ctx context.Context, r member.Requester, orgID, inviteID uuid.UUID, draft bool) error {
	err := s.uow.RunInTx(ctx, func(ctx context.Context) error {
		if _, err := s.locked(ctx, r, orgID, inviteID, draft); err != nil {
			return err
		}
		return s.store.Revoke(ctx, inviteID, s.now())
	})
	if err != nil {
		return s.errOrReport(ctx, "invite: revoke", err)
	}
	return nil
}

func (s *Service) issue(ctx context.Context, inv *Invite) (*Created, error) {
	raw, err := inv.issue(s.now())
	if err != nil {
		return nil, err
	}
	if err := s.save(ctx, inv); err != nil {
		return nil, err
	}
	link := s.baseURL + "/invite/" + raw
	if err := s.send(ctx, inv, link); err != nil {
		return nil, err
	}
	var inviter string
	if inv.InvitedBy != uuid.Nil {
		if inviter, err = s.people.PublicID(ctx, inv.InvitedBy); err != nil {
			return nil, err
		}
	}
	err = s.emit(ctx, inv.OrgID, events.InviteCreated, events.InviteCreatedV1{
		ID: inv.PublicID, Email: inv.Email, Role: string(inv.Role), InvitedBy: inviter, ExpiresAt: inv.ExpiresAt,
	})
	if err != nil {
		return nil, err
	}
	return &Created{Invite: inv, Link: link}, nil
}

func (s *Service) emit(ctx context.Context, orgID uuid.UUID, t events.Type, data any) error {
	if s.events == nil {
		return nil
	}
	return s.events.EnqueueOrg(ctx, orgID, t, data)
}

func (s *Service) save(ctx context.Context, inv *Invite) error {
	if !inv.unsaved {
		return s.store.Update(ctx, inv)
	}
	if err := s.store.Insert(ctx, inv); err != nil {
		return err
	}
	inv.unsaved = false
	return nil
}

func (s *Service) send(ctx context.Context, inv *Invite, link string) error {
	org, err := s.orgs.Name(ctx, inv.OrgID)
	if err != nil {
		return err
	}
	org = strings.Join(strings.Fields(org), " ")
	inviter := s.inviterName(ctx, inv.InvitedBy)
	role := inv.Role.Label()
	expires := inv.ExpiresAt.UTC().Format("2 January 2006, 15:04 UTC")
	var html strings.Builder
	if err := views.InviteHTML(org, inviter, role, link, expires).Render(ctx, &html); err != nil {
		return err
	}
	_, err = s.mail.Enqueue(ctx, mailer.Message{
		To: inv.Email, From: s.from, Subject: views.InviteSubject(inviter, org),
		TextBody: views.InviteText(org, inviter, role, link, expires), HTMLBody: html.String(),
	})
	return err
}

func (s *Service) inviterName(ctx context.Context, userID uuid.UUID) string {
	if userID == uuid.Nil {
		return "Someone"
	}
	email, name, err := s.people.Contact(ctx, userID)
	switch {
	case err != nil:
		return "Someone"
	case strings.TrimSpace(name) != "":
		return strings.Join(strings.Fields(name), " ")
	}
	return email
}

func (s *Service) list(ctx context.Context, orgID uuid.UUID, keep func(*Invite) bool) ([]*Invite, error) {
	all, err := s.store.ListLive(ctx, orgID)
	if err != nil {
		return nil, s.errOrReport(ctx, "invite: list", err)
	}
	out := make([]*Invite, 0, len(all))
	for _, i := range all {
		if keep(i) {
			out = append(out, i)
		}
	}
	return out, nil
}

func (s *Service) errOrReport(ctx context.Context, message string, err error) error {
	if IsExpectedError(err) {
		return err
	}
	return s.unexpected(ctx, message, err)
}
