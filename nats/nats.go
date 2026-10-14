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
	"sync"
	"sync/atomic"
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
	e := &events{c: c, log: log}
	opts := []natsgo.Option{
		natsgo.Name(c.Name),
		natsgo.MaxReconnects(-1),
		natsgo.Timeout(dialTimeout),
		natsgo.DrainTimeout(drainTimeout),
		natsgo.ErrorHandler(e.asyncError),
		natsgo.DisconnectErrHandler(e.disconnected),
		natsgo.ReconnectErrHandler(e.reconnectFailed),
		natsgo.ReconnectHandler(e.reconnected),
	}
	switch {
	case c.User != "":
		return append(opts, natsgo.UserInfo(c.User, c.Password))
	case c.Token != "":
		return append(opts, natsgo.Token(c.Token))
	}
	return opts
}

type events struct {
	c       Config
	log     *slog.Logger
	failing atomic.Bool
}

func (e *events) asyncError(_ *natsgo.Conn, sub *natsgo.Subscription, err error) {
	attrs := []any{"err", redact(err, e.c)}
	if sub != nil {
		attrs = append(attrs, "subject", sub.Subject)
	}
	e.log.Warn("nats: async error", attrs...)
}

func (e *events) disconnected(_ *natsgo.Conn, err error) {
	e.log.Warn("nats: disconnected", "err", redact(err, e.c))
}

func (e *events) reconnectFailed(_ *natsgo.Conn, err error) {
	level := slog.LevelDebug
	if e.failing.CompareAndSwap(false, true) {
		level = slog.LevelWarn
	}
	e.log.Log(context.Background(), level, "nats: reconnect failed", "err", redact(err, e.c))
}

func (e *events) reconnected(*natsgo.Conn) {
	e.failing.Store(false)
	e.log.Info("nats: reconnected")
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

// Closer drains nc on the first Close and waits for it to close; every Close returns that first result.
func Closer(nc *natsgo.Conn) io.Closer { return &closer{nc: nc} }

type closer struct {
	nc   *natsgo.Conn
	once sync.Once
	err  error
}

func (c *closer) Close() error {
	c.once.Do(func() { c.err = c.drain() })
	return c.err
}

func (c *closer) drain() error {
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
