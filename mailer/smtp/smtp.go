// Package smtp is the mail driver that sends through an SMTP server with STARTTLS, implicit TLS or no TLS.
package smtp

import (
	"bytes"
	"cmp"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	netsmtp "net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"altalune.id/oagent/mailer"
	"altalune.id/oagent/nanoid"
)

// The transport security modes.
const (
	TLSStartTLS = "starttls"
	TLSImplicit = "implicit"
	TLSNone     = "none"
)

// Defaults applied when Config leaves a field zero.
const (
	DefaultPort    = 587
	DefaultTimeout = 10 * time.Second
)

const messageIDLen = 24

var _ mailer.Mailer = (*Sender)(nil)

// Config configures the driver; TLS is starttls, implicit or none.
type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	TLS      string
	Timeout  time.Duration
}

// Sender sends each Message as one SMTP transaction bounded by a timeout.
type Sender struct {
	from    string
	cfg     Config
	rootCAs *x509.CertPool
}

// New builds a Sender, applying the default port, TLS mode and timeout.
func New(from string, cfg Config) (*Sender, error) {
	if from == "" {
		return nil, &mailer.MissingConfigError{Field: "from"}
	}
	if cfg.Host == "" {
		return nil, &mailer.MissingConfigError{Field: "smtp.host"}
	}
	cfg.Port = cmp.Or(cfg.Port, DefaultPort)
	if cfg.Port < 1 || cfg.Port > 65535 {
		return nil, &mailer.InvalidConfigError{Field: "smtp.port", Value: strconv.Itoa(cfg.Port), Reason: "must be 1-65535"}
	}
	cfg.TLS = cmp.Or(cfg.TLS, TLSStartTLS)
	switch cfg.TLS {
	case TLSStartTLS, TLSImplicit, TLSNone:
	default:
		return nil, &mailer.InvalidConfigError{Field: "smtp.tls", Value: cfg.TLS, Reason: "must be starttls, implicit or none"}
	}
	if cfg.Timeout < 0 {
		return nil, &mailer.InvalidConfigError{Field: "smtp.timeout", Value: cfg.Timeout.String(), Reason: "must not be negative"}
	}
	cfg.Timeout = cmp.Or(cfg.Timeout, DefaultTimeout)
	return &Sender{from: from, cfg: cfg}, nil
}

// Send delivers m as one SMTP transaction; ctx and the configured timeout bound the whole dialogue.
func (s *Sender) Send(ctx context.Context, m mailer.Message) error {
	m.From = cmp.Or(m.From, s.from)
	if err := m.Validate(); err != nil {
		return err
	}
	from, err := mail.ParseAddress(m.From)
	if err != nil {
		return &mailer.InvalidAddressError{Field: "From"}
	}
	to, err := mail.ParseAddress(m.To)
	if err != nil {
		return &mailer.InvalidAddressError{Field: "To"}
	}
	raw, err := buildMIME(from, to, m, time.Now())
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	defer cancel()
	conn, err := s.dial(ctx)
	if err != nil {
		return s.fail(ctx, "dial", err)
	}
	defer func() { _ = conn.Close() }()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Unix(1, 0)) })
	defer stop()

	return s.deliver(ctx, conn, from.Address, to.Address, raw)
}

func (s *Sender) deliver(ctx context.Context, conn net.Conn, from, to string, raw []byte) error {
	c, err := netsmtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		return s.fail(ctx, "greeting", err)
	}
	defer func() { _ = c.Close() }()
	if s.cfg.TLS == TLSStartTLS {
		// SECURITY: never fall back to plain text when the server does not offer STARTTLS.
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return &StartTLSUnsupportedError{Host: s.cfg.Host}
		}
		if err := c.StartTLS(s.tlsConfig()); err != nil {
			return s.fail(ctx, "starttls", err)
		}
	}
	if s.cfg.Username != "" {
		if err := c.Auth(netsmtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)); err != nil {
			return s.fail(ctx, "auth", err)
		}
	}
	if err := c.Mail(from); err != nil {
		return s.fail(ctx, "mail from", err)
	}
	if err := c.Rcpt(to); err != nil {
		return s.fail(ctx, "rcpt to", err)
	}
	w, err := c.Data()
	if err != nil {
		return s.fail(ctx, "data", err)
	}
	if _, err := w.Write(raw); err != nil {
		return s.fail(ctx, "data", err)
	}
	if err := w.Close(); err != nil {
		return s.fail(ctx, "data", err)
	}
	// NOTE: the server accepted the message at the end of DATA; a failed QUIT must not trigger a resend.
	_ = c.Quit()
	return nil
}

func (s *Sender) dial(ctx context.Context) (net.Conn, error) {
	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	d := &net.Dialer{Timeout: s.cfg.Timeout}
	if s.cfg.TLS == TLSImplicit {
		td := &tls.Dialer{NetDialer: d, Config: s.tlsConfig()}
		return td.DialContext(ctx, "tcp", addr)
	}
	return d.DialContext(ctx, "tcp", addr)
}

func (s *Sender) tlsConfig() *tls.Config {
	return &tls.Config{ServerName: s.cfg.Host, RootCAs: s.rootCAs, MinVersion: tls.VersionTLS12}
}

func (s *Sender) fail(ctx context.Context, step string, err error) error {
	if cerr := ctx.Err(); cerr != nil {
		return fmt.Errorf("mailer: smtp %s: %w", step, cerr)
	}
	return fmt.Errorf("mailer: smtp %s: %w", step, err)
}

func buildMIME(from, to *mail.Address, m mailer.Message, now time.Time) ([]byte, error) {
	id, err := nanoid.New(messageIDLen)
	if err != nil {
		return nil, fmt.Errorf("mailer: message id: %w", err)
	}
	domain := from.Address[strings.LastIndexByte(from.Address, '@')+1:]
	var b bytes.Buffer
	fmt.Fprintf(&b, "From: %s\r\n", from.String())
	fmt.Fprintf(&b, "To: %s\r\n", to.String())
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", m.Subject))
	fmt.Fprintf(&b, "Date: %s\r\n", now.Format(time.RFC1123Z))
	fmt.Fprintf(&b, "Message-ID: <%s@%s>\r\n", id, domain)
	b.WriteString("MIME-Version: 1.0\r\n")

	if m.TextBody == "" || m.HTMLBody == "" {
		ct, body := "text/plain; charset=utf-8", m.TextBody
		if m.TextBody == "" {
			ct, body = "text/html; charset=utf-8", m.HTMLBody
		}
		fmt.Fprintf(&b, "Content-Type: %s\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n", ct)
		if err := writeQP(&b, body); err != nil {
			return nil, err
		}
		return b.Bytes(), nil
	}

	mw := multipart.NewWriter(&b)
	fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", mw.Boundary())
	for _, p := range []struct{ ct, body string }{
		{"text/plain; charset=utf-8", m.TextBody},
		{"text/html; charset=utf-8", m.HTMLBody},
	} {
		pw, err := mw.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {p.ct},
			"Content-Transfer-Encoding": {"quoted-printable"},
		})
		if err != nil {
			return nil, fmt.Errorf("mailer: mime part: %w", err)
		}
		if err := writeQP(pw, p.body); err != nil {
			return nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("mailer: mime close: %w", err)
	}
	return b.Bytes(), nil
}

func writeQP(w interface{ Write([]byte) (int, error) }, body string) error {
	qw := quotedprintable.NewWriter(w)
	if _, err := qw.Write([]byte(body)); err != nil {
		return fmt.Errorf("mailer: encode body: %w", err)
	}
	if err := qw.Close(); err != nil {
		return fmt.Errorf("mailer: encode body: %w", err)
	}
	return nil
}
