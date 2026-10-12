package config

import (
	"net/mail"
	"time"

	"altalune.id/oagent/sealer"
)

// MailDriver selects how email is sent.
type MailDriver string

// The mail drivers. Console prints mail to stdout and is refused in production.
const (
	MailDriverConsole MailDriver = "console"
	MailDriverSMTP    MailDriver = "smtp"
	MailDriverResend  MailDriver = "resend"
)

// MailTLS selects the SMTP transport security.
type MailTLS string

// The SMTP TLS modes. None is refused in production.
const (
	MailTLSStartTLS MailTLS = "starttls"
	MailTLSImplicit MailTLS = "implicit"
	MailTLSNone     MailTLS = "none"
)

// MailConfig configures outgoing email.
type MailConfig struct {
	Driver MailDriver       `yaml:"driver" mapstructure:"driver"`
	From   string           `yaml:"from"   mapstructure:"from"`
	SMTP   MailSMTPConfig   `yaml:"smtp"   mapstructure:"smtp"`
	Resend MailResendConfig `yaml:"resend" mapstructure:"resend"`
}

// MailSMTPConfig configures the SMTP driver.
type MailSMTPConfig struct {
	Host     string        `yaml:"host"     mapstructure:"host"`
	Port     int           `yaml:"port"     mapstructure:"port"`
	Username string        `yaml:"username" mapstructure:"username"`
	Password string        `yaml:"password" mapstructure:"password" awareness:"secret"`
	TLS      MailTLS       `yaml:"tls"      mapstructure:"tls"`
	Timeout  time.Duration `yaml:"timeout"  mapstructure:"timeout"`
}

// MailResendConfig configures the Resend driver.
type MailResendConfig struct {
	APIKey string `yaml:"apiKey" mapstructure:"apiKey" awareness:"secret"`
}

func (c *Config) validateMail() error {
	m := c.Mail
	switch m.Driver {
	case MailDriverConsole, MailDriverSMTP, MailDriverResend:
	default:
		return &InvalidSettingError{Key: "mail.driver", Reason: "must be console, smtp or resend"}
	}
	if m.Driver == MailDriverConsole && c.Env.IsProduction() {
		return &InvalidSettingError{Key: "mail.driver", Reason: "console is not allowed in production; use smtp or resend"}
	}
	if m.From != "" {
		if _, err := mail.ParseAddress(m.From); err != nil {
			return &InvalidSettingError{Key: "mail.from", Reason: "must be an email address, optionally with a name: Name <addr@example.com>"}
		}
	}
	if m.Driver == MailDriverConsole {
		return nil
	}
	reason := "mail.driver is " + string(m.Driver)
	if m.From == "" {
		return &MissingSettingError{Keys: "mail.from", Reason: reason}
	}
	if c.HTTP.BaseURL == "" {
		return &MissingSettingError{Keys: "http.baseURL", Reason: reason + "; emails need absolute links"}
	}
	if c.Secrets.EncryptionKey == "" {
		return &MissingSettingError{Keys: "secrets.encryptionKey", Reason: reason + "; queued emails are encrypted; " + generateHint}
	}
	if _, err := sealer.ParseKey(c.Secrets.EncryptionKey); err != nil {
		return &InvalidSettingError{Key: "secrets.encryptionKey", Reason: "must be 32 bytes as hex or base64; " + generateHint}
	}
	if m.Driver == MailDriverResend {
		if m.Resend.APIKey == "" {
			return &MissingSettingError{Keys: "mail.resend.apiKey", Reason: reason}
		}
		return nil
	}
	return c.validateSMTP()
}

// MaxSMTPTimeout bounds one SMTP send, well inside the mail queue's 5-minute claim lease.
const MaxSMTPTimeout = 2 * time.Minute

func (c *Config) validateSMTP() error {
	s := c.Mail.SMTP
	if s.Host == "" {
		return &MissingSettingError{Keys: "mail.smtp.host", Reason: "mail.driver is smtp"}
	}
	if s.Port < 1 || s.Port > 65535 {
		return &InvalidSettingError{Key: "mail.smtp.port", Reason: "must be 1-65535"}
	}
	if s.Timeout <= 0 || s.Timeout > MaxSMTPTimeout {
		return &InvalidSettingError{Key: "mail.smtp.timeout", Reason: "must be between 1s and " + MaxSMTPTimeout.String()}
	}
	switch s.TLS {
	case MailTLSStartTLS, MailTLSImplicit:
		return nil
	case MailTLSNone:
		if c.Env.IsProduction() {
			return &InvalidSettingError{Key: "mail.smtp.tls", Reason: "none is not allowed in production; use starttls or implicit"}
		}
		return nil
	default:
		return &InvalidSettingError{Key: "mail.smtp.tls", Reason: "must be starttls, implicit or none"}
	}
}
