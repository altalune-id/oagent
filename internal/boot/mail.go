package boot

import (
	"fmt"
	"log/slog"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/mailqueue"
	"altalune.id/oagent/mailer"
	"altalune.id/oagent/mailer/console"
	"altalune.id/oagent/mailer/resend"
	"altalune.id/oagent/mailer/smtp"
	"altalune.id/oagent/sealer"
)

func buildMailer(cfg config.Config) (mailer.Mailer, error) {
	m := cfg.Mail
	var (
		out mailer.Mailer
		err error
	)
	switch m.Driver {
	case "", config.MailDriverConsole:
		out = console.New(m.From, nil)
	case config.MailDriverSMTP:
		out, err = smtp.New(m.From, smtp.Config{
			Host:     m.SMTP.Host,
			Port:     m.SMTP.Port,
			Username: m.SMTP.Username,
			Password: m.SMTP.Password,
			TLS:      string(m.SMTP.TLS),
			Timeout:  m.SMTP.Timeout,
		})
	case config.MailDriverResend:
		out, err = resend.New(m.From, resend.Config{APIKey: m.Resend.APIKey})
	default:
		err = &mailer.InvalidConfigError{Field: "driver", Value: string(m.Driver), Reason: "unknown driver"}
	}
	if err != nil {
		return nil, fmt.Errorf("boot: mail: %w", err)
	}
	return out, nil
}

// SECURITY: only development with the console driver may queue mail unsealed; the console prints it anyway.
func mailAllowPlain(cfg config.Config) bool {
	return cfg.Env == config.EnvDevelopment && cfg.Mail.Driver == config.MailDriverConsole
}

func buildMailQueue(pool db.Pool, cfg config.Config, sl sealer.Sealer) mailqueue.Store {
	return mailqueue.NewStore(pool, cfg.Postgres.Schema, cfg.Postgres.TablePrefix, sl, mailAllowPlain(cfg))
}

func buildMailWorker(svcs *Services, log *slog.Logger) *mailqueue.Worker {
	return mailqueue.NewWorker(svcs.MailQueue, svcs.Mailer, log, mailqueue.WorkerOpts{})
}
