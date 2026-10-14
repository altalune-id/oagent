package boot

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strings"
	"sync/atomic"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/auth"
	"altalune.id/oagent/internal/install"
	"altalune.id/oagent/internal/kernel"
	"altalune.id/oagent/internal/kernel/capabilities"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/sealer"
)

const generatedStateSecretLen = 32

func buildAuthSecrets(cfg config.Config, s *Services, log *slog.Logger) error {
	secret, err := stateSecret(cfg.HTTP, secretsLog(log, s.Caps))
	if err != nil {
		return err
	}
	if !cfg.HTTP.CookieSecure && cfg.Env != config.EnvDevelopment {
		secretsLog(log, s.Caps).Warn("boot: http.cookieSecure is false; session cookies are sent over plain HTTP")
	}
	s.StateSecret = secret
	if s.InstallToken, err = install.NewToken(cfg.Install.Token); err != nil {
		return fmt.Errorf("boot: install token: %w", err)
	}
	return nil
}

func basicCredentials(cfg config.Config, caps capabilities.Capabilities) auth.BasicCredentials {
	if !caps.BasicAuth {
		return auth.BasicCredentials{}
	}
	return auth.BasicCredentials{User: cfg.Auth.Basic.User, Password: cfg.Auth.Basic.Password}
}

func secretsLog(log *slog.Logger, caps capabilities.Capabilities) *slog.Logger {
	if !caps.LocalLogin {
		return slog.New(slog.DiscardHandler)
	}
	return orDiscard(log)
}

// SECURITY: without a valid key sessions stay in memory, so nothing is ever sealed with a throwaway key.
func buildSessionStore(k kernel.Kernel, cfg config.Config, sl sealer.Sealer, log *slog.Logger) session.Store {
	if _, err := sealer.ParseKey(cfg.Secrets.EncryptionKey); err == nil {
		return session.NewPostgresStore(k.Pool.W, cfg.Postgres.Schema, cfg.Postgres.TablePrefix, sl, k.Reporter.Unexpected)
	}
	log.Warn("boot: secrets.encryptionKey is missing or invalid; sessions are kept in memory and reset on restart")
	return session.NewMemoryStore()
}

func stateSecret(c config.HTTPConfig, log *slog.Logger) ([]byte, error) {
	if c.StateSecret != "" {
		return []byte(c.StateSecret), nil
	}
	b := make([]byte, generatedStateSecretLen)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("boot: generate state secret: %w", err)
	}
	log.Warn("boot: http.stateSecret is missing or invalid; using a random one, so sessions reset on restart")
	return b, nil
}

func firstBoot(ctx context.Context, cfg config.Config, s *Services, log *slog.Logger, logURL bool) error {
	log = orDiscard(log)
	switch {
	case s.Caps.BasicAuth:
		return s.Install.EnsureBasicSystemOrg(ctx)
	case !s.Caps.LocalLogin:
		return nil
	}
	if err := s.Legal.Publish(ctx); err != nil {
		return fmt.Errorf("boot: legal documents: %w", err)
	}
	required, err := s.Install.Required(ctx, false)
	if err != nil || !required {
		return err
	}
	if s.Caps.EnvGenesis {
		return installGenesis(ctx, cfg.Auth.Genesis, s.Install, log)
	}
	if !logURL {
		return nil
	}
	log.Info("boot: setup required — open the one-time install URL", "url", installURL(cfg.HTTP, s.InstallToken.Value()))
	return nil
}

func installGenesis(ctx context.Context, g config.GenesisConfig, svc *install.Service, log *slog.Logger) error {
	_, err := svc.Install(ctx, install.Request{Email: g.Email, Password: g.Password})
	switch {
	case install.IsAlreadyInstalledError(err):
		return nil
	case err != nil:
		return fmt.Errorf("boot: install from auth.genesis: %w", err)
	}
	log.Info("boot: installed the first owner from auth.genesis")
	return nil
}

// SECURITY: only local login has an install page; a failed check keeps the gate shut rather than open.
func installRequired(ctx context.Context, s *Services) (*atomic.Bool, error) {
	required := &atomic.Bool{}
	if !s.Caps.LocalLogin {
		return required, nil
	}
	need, err := s.Install.Required(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("boot: install state: %w", err)
	}
	required.Store(need)
	return required, nil
}

func logsInstallURL(o *options) bool {
	return o.installURL && !o.schedulerOnly && !o.consumerOnly
}

func installURL(c config.HTTPConfig, token string) string {
	return publicBase(c) + "/install?token=" + url.QueryEscape(token)
}

func publicBase(c config.HTTPConfig) string {
	base := strings.TrimSuffix(c.BaseURL, "/")
	if base == "" {
		base = "http://localhost"
		if _, port, err := net.SplitHostPort(c.Addr); err == nil && port != "" {
			base += ":" + port
		}
	}
	return base + strings.TrimSuffix(c.BasePath, "/")
}

func orDiscard(log *slog.Logger) *slog.Logger {
	if log == nil {
		return slog.New(slog.DiscardHandler)
	}
	return log
}
