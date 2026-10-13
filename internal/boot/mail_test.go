package boot

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/mailqueue"
	"altalune.id/oagent/mailer"
	"altalune.id/oagent/mailer/console"
	"altalune.id/oagent/mailer/resend"
	"altalune.id/oagent/mailer/smtp"
	"altalune.id/oagent/sealer"
)

func TestBuildMailer(t *testing.T) {
	cases := []struct {
		name string
		mail config.MailConfig
		want any
	}{
		{"console", config.MailConfig{Driver: config.MailDriverConsole}, &console.Sender{}},
		{"default", config.MailConfig{}, &console.Sender{}},
		{"smtp", config.MailConfig{
			Driver: config.MailDriverSMTP, From: "noreply@example.com",
			SMTP: config.MailSMTPConfig{Host: "mx.example.com", Port: 587, TLS: config.MailTLSStartTLS, Timeout: 1},
		}, &smtp.Sender{}},
		{"resend", config.MailConfig{
			Driver: config.MailDriverResend, From: "noreply@example.com",
			Resend: config.MailResendConfig{APIKey: "re_test"},
		}, &resend.Sender{}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			m, err := buildMailer(config.Config{Mail: tt.mail})
			require.NoError(t, err)
			assert.IsType(t, tt.want, m)
		})
	}

	for name, mail := range map[string]config.MailConfig{
		"smtp without from":  {Driver: config.MailDriverSMTP},
		"resend without key": {Driver: config.MailDriverResend, From: "noreply@example.com"},
		"unknown driver":     {Driver: "carrier-pigeon"},
	} {
		t.Run(name, func(t *testing.T) {
			m, err := buildMailer(config.Config{Mail: mail})
			require.Error(t, err)
			assert.True(t, mailer.IsMissingConfigError(err) || mailer.IsInvalidConfigError(err), "got %v", err)
			assert.True(t, m == nil, "a failed build returns a nil Mailer, not one holding a nil driver: %#v", m)
		})
	}
}

func TestMailAllowPlain(t *testing.T) {
	cases := []struct {
		env    config.Env
		driver config.MailDriver
		want   bool
	}{
		{config.EnvDevelopment, config.MailDriverConsole, true},
		{config.EnvDevelopment, config.MailDriverSMTP, false},
		{config.EnvDevelopment, config.MailDriverResend, false},
		{config.EnvProduction, config.MailDriverConsole, false},
		{config.EnvProduction, config.MailDriverSMTP, false},
		{"", config.MailDriverConsole, false},
	}
	for _, tt := range cases {
		t.Run(string(tt.env)+"/"+string(tt.driver), func(t *testing.T) {
			assert.Equal(t, tt.want, mailAllowPlain(config.Config{Env: tt.env, Mail: config.MailConfig{Driver: tt.driver}}))
		})
	}
}

func TestBuildMailQueueRefusesPlainOutsideDevConsole(t *testing.T) {
	cfg := config.Config{Env: config.EnvProduction, Mail: config.MailConfig{Driver: config.MailDriverSMTP}}
	q := buildMailQueue(db.Pool{}, cfg, sealer.Disabled())
	_, err := q.Enqueue(context.Background(), mailer.Message{To: "a@example.com", Subject: "s", TextBody: "b"})
	assert.True(t, mailqueue.IsSealerRequiredError(err))
}

func TestBuildMailWorker(t *testing.T) {
	var buf bytes.Buffer
	svcs := &Services{
		Mailer:    console.New("", &buf),
		MailQueue: mailqueue.NewStore(db.Pool{}, "public", "", sealer.Disabled(), false),
	}
	w := buildMailWorker(svcs, slog.New(slog.NewTextHandler(&buf, nil)))
	require.NotNil(t, w)
	assert.Equal(t, mailqueue.WorkerName, w.Name())
}
