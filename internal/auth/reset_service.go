package auth

import (
	"context"
	"strings"
	"time"

	"altalune.id/oagent/internal/auth/views"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/user"
	"altalune.id/oagent/mailer"
	"altalune.id/oagent/password"
)

const mailTime = "2 January 2006, 15:04 UTC"

const resetResendAfter = 2 * time.Minute

// ResetDeps are the ports the password reset flow uses.
type ResetDeps struct {
	Store      ResetStore
	Transactor db.Transactor
	Users      ResetUsers
	Passwords  PasswordSetter
	Sessions   UserSessions
	Mail       MailQueue
	From       string
	BaseURL    string
}

// Resets runs password reset by email.
type Resets struct {
	d   ResetDeps
	now func() time.Time
}

// NewResets builds the reset flow over d.
func NewResets(d ResetDeps) *Resets {
	return &Resets{d: d, now: func() time.Time { return time.Now().UTC() }}
}

// WithClock sets the clock that issues and checks links.
func (s *Resets) WithClock(now func() time.Time) *Resets {
	s.now = now
	return s
}

// Request emails a link to a person with a password and no unused link under 2 minutes old. SECURITY: all else gets the same nil.
func (s *Resets) Request(ctx context.Context, raw string) error {
	email, err := user.NormalizeEmail(raw)
	if err != nil {
		s.decoy(ctx)
		return nil //nolint:nilerr // SECURITY: an address that cannot exist gets the same answer as any other.
	}
	u, err := s.d.Users.ByEmail(ctx, email)
	switch {
	case user.IsNotFoundError(err):
		s.decoy(ctx)
		return nil
	case err != nil:
		return err
	case !u.HasPassword():
		s.decoy(ctx)
		return nil
	}
	r, msg, err := s.prepare(ctx, u)
	if err != nil {
		return err
	}
	return s.d.Transactor.RunInTx(ctx, func(ctx context.Context) error {
		recent, err := s.d.Store.HasRecentUnused(ctx, u.ID, s.now().Add(-resetResendAfter))
		if err != nil || recent {
			return err
		}
		if err := s.d.Store.DeleteUnused(ctx, u.ID); err != nil {
			return err
		}
		if err := s.d.Store.Insert(ctx, r); err != nil {
			return err
		}
		_, err = s.d.Mail.Enqueue(ctx, msg)
		return err
	})
}

// Lookup returns the usable reset the raw token from a link names.
func (s *Resets) Lookup(ctx context.Context, rawToken string) (*Reset, error) {
	if rawToken == "" {
		return nil, &ResetNotUsableError{Reason: ResetNotFound}
	}
	r, err := s.d.Store.ByTokenHash(ctx, hashResetToken(rawToken))
	if err != nil {
		return nil, err
	}
	if err := r.Usable(s.now()); err != nil {
		return nil, err
	}
	return r, nil
}

// ByPublicID returns the usable reset with publicID.
func (s *Resets) ByPublicID(ctx context.Context, publicID string) (*Reset, error) {
	r, err := s.d.Store.ByPublicID(ctx, publicID)
	if err != nil {
		return nil, err
	}
	if err := r.Usable(s.now()); err != nil {
		return nil, err
	}
	return r, nil
}

// Person returns the email and name of the person r is for, the new password's hints.
func (s *Resets) Person(ctx context.Context, r *Reset) (email, name string, err error) {
	u, err := s.d.Users.ByID(ctx, r.UserID)
	if err != nil {
		return "", "", err
	}
	return u.Email, u.Name, nil
}

// Complete uses the reset publicID to set a new password, ends every session of that person and tells them, in one unit of work.
func (s *Resets) Complete(ctx context.Context, publicID, plain string) error {
	r, err := s.ByPublicID(ctx, publicID)
	if err != nil {
		return err
	}
	email, name, err := s.Person(ctx, r)
	if err != nil {
		return err
	}
	if err := password.Check(plain, password.Hints{Email: email, Name: name}); err != nil {
		return err
	}
	return s.d.Transactor.RunInTx(ctx, func(ctx context.Context) error {
		r, err := s.d.Store.MarkUsed(ctx, publicID, s.now())
		if IsResetNotUsableError(err) {
			return s.whyNotUsable(ctx, publicID)
		}
		if err != nil {
			return err
		}
		if err := s.d.Passwords.SetPassword(ctx, r.UserID, plain); err != nil {
			return err
		}
		if err := s.d.Sessions.DeleteByUser(ctx, r.UserID); err != nil {
			return err
		}
		u, err := s.d.Users.ByID(ctx, r.UserID)
		if err != nil {
			return err
		}
		return passwordChanged(ctx, s.d.Mail, s.d.From, s.d.BaseURL, u)
	})
}

func (s *Resets) whyNotUsable(ctx context.Context, publicID string) error {
	r, err := s.d.Store.ByPublicID(ctx, publicID)
	if err != nil {
		return err
	}
	if err := r.Usable(s.now()); err != nil {
		return err
	}
	return &ResetNotUsableError{Reason: ResetUsed}
}

func (s *Resets) prepare(ctx context.Context, u *user.User) (*Reset, mailer.Message, error) {
	r, token, err := newReset(u.ID, s.now())
	if err != nil {
		return nil, mailer.Message{}, err
	}
	link := s.d.BaseURL + "/reset-password/" + token
	expires := r.ExpiresAt.Format(mailTime)
	var html strings.Builder
	if err := views.ResetHTML(link, expires).Render(ctx, &html); err != nil {
		return nil, mailer.Message{}, err
	}
	msg := mailer.Message{
		To: u.Email, From: s.d.From, Subject: views.ResetSubject(),
		TextBody: views.ResetText(link, expires), HTMLBody: html.String(),
	}
	return r, msg, nil
}

// NOTE: an unknown email still builds a link and renders the email, so it costs about the same as a known one.
func (s *Resets) decoy(ctx context.Context) {
	_, _, _ = s.prepare(ctx, &user.User{Email: "nobody@example.invalid"})
}

func passwordChanged(ctx context.Context, mail MailQueue, from, baseURL string, u *user.User) error {
	signIn := baseURL + "/login"
	var html strings.Builder
	if err := views.ChangedHTML(signIn).Render(ctx, &html); err != nil {
		return err
	}
	_, err := mail.Enqueue(ctx, mailer.Message{
		To: u.Email, From: from, Subject: views.ChangedSubject(),
		TextBody: views.ChangedText(signIn), HTMLBody: html.String(),
	})
	return err
}
