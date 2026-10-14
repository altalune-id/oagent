// Package nats opens, drains and watches the one NATS connection the drivers share.
package nats

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"strings"
	"time"

	natsgo "github.com/nats-io/nats.go"
)

const (
	defaultConnectTimeout = 10 * time.Second
	backoffBase           = 250 * time.Millisecond
	backoffMax            = 2 * time.Second
	dialTimeout           = 2 * time.Second
	drainTimeout          = 5 * time.Second
)

// Config is the connection: URL (one or several, comma-separated) and either User+Password or Token.
type Config struct {
	URL            string
	Token          string
	User           string
	Password       string
	Name           string
	ConnectTimeout time.Duration
}

// Open dials until ConnectTimeout (10s when zero). SECURITY: errors and logs never carry the URL's credentials, the token or the password.
func Open(ctx context.Context, c Config, log *slog.Logger) (*natsgo.Conn, error) {
	hosts, err := hostsOf(c.URL)
	if err != nil {
		return nil, err
	}
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	timeout := c.ConnectTimeout
	if timeout <= 0 {
		timeout = defaultConnectTimeout
	}
	opts := options(c, log)
	deadline := time.Now().Add(timeout)
	backoff := backoffBase
	for {
		nc, err := natsgo.Connect(c.URL, opts...)
		if err == nil {
			return nc, nil
		}
		err = redact(err, c)
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, fmt.Errorf("nats: cannot connect to %s (nats.url) within %s: %w", hosts, timeout, err)
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("nats: cannot connect to %s (nats.url): %w", hosts, errors.Join(ctx.Err(), err))
		case <-time.After(min(backoff, remaining)):
		}
		backoff = min(backoff*2, backoffMax)
	}
}

func options(c Config, log *slog.Logger) []natsgo.Option {
	opts := []natsgo.Option{
		natsgo.Name(c.Name),
		natsgo.MaxReconnects(-1),
		natsgo.Timeout(dialTimeout),
		natsgo.DrainTimeout(drainTimeout),
		natsgo.DisconnectErrHandler(func(_ *natsgo.Conn, err error) {
			log.Warn("nats: disconnected", "err", redact(err, c))
		}),
		natsgo.ReconnectHandler(func(*natsgo.Conn) { log.Info("nats: reconnected") }),
	}
	switch {
	case c.User != "":
		return append(opts, natsgo.UserInfo(c.User, c.Password))
	case c.Token != "":
		return append(opts, natsgo.Token(c.Token))
	}
	return opts
}

// SECURITY: the URL may hold a password, so no error repeats it or the parser's message.
func hostsOf(raw string) (string, error) {
	var hosts []string
	for server := range strings.SplitSeq(raw, ",") {
		u, err := url.Parse(strings.TrimSpace(server))
		if err != nil || u.Host == "" {
			return "", errors.New("nats: nats.url is not a valid nats://, tls://, ws:// or wss:// URL")
		}
		hosts = append(hosts, u.Host)
	}
	return strings.Join(hosts, ", "), nil
}

func redact(err error, c Config) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	secrets := []string{c.Password, c.Token}
	for server := range strings.SplitSeq(c.URL, ",") {
		if u, perr := url.Parse(strings.TrimSpace(server)); perr == nil && u.User != nil {
			if pw, ok := u.User.Password(); ok {
				secrets = append(secrets, pw)
			} else {
				secrets = append(secrets, u.User.Username())
			}
		}
	}
	leaked := false
	for _, s := range secrets {
		if s != "" && strings.Contains(msg, s) {
			msg, leaked = strings.ReplaceAll(msg, s, "[redacted]"), true
		}
	}
	if !leaked {
		return err
	}
	return errors.New(msg)
}

// Closer drains nc on Close and waits for it to close; a closed nc is fine.
func Closer(nc *natsgo.Conn) io.Closer { return closer{nc: nc} }

type closer struct{ nc *natsgo.Conn }

func (c closer) Close() error {
	if c.nc.IsClosed() {
		return nil
	}
	closed := make(chan struct{})
	c.nc.SetClosedHandler(func(*natsgo.Conn) { close(closed) })
	if err := c.nc.Drain(); err != nil && !errors.Is(err, natsgo.ErrConnectionReconnecting) {
		return fmt.Errorf("nats: drain: %w", err)
	}
	select {
	case <-closed:
	case <-time.After(drainTimeout + time.Second):
		c.nc.Close()
	}
	return nil
}
