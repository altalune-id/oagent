package nats

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/testutil/natstest"
)

const (
	testPassword = "hunter2-nats-secret"
	testToken    = "s3cr3t-nats-token"
)

func testConfig(url string) Config {
	return Config{URL: url, Name: "oagent", ConnectTimeout: 300 * time.Millisecond}
}

func discard() *slog.Logger { return slog.New(slog.DiscardHandler) }

func bufferLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, nil)), &buf
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func assertNoSecrets(t *testing.T, s string) {
	t.Helper()
	assert.NotContains(t, s, testPassword)
	assert.NotContains(t, s, testToken)
}

// Review Focus 3: a refused connection names the host, never the URL credentials, the token or the password.
func TestOpen_RefusedHasNoCredentials(t *testing.T) {
	for name, c := range map[string]Config{
		"url credentials": testConfig("nats://oagent:" + testPassword + "@127.0.0.1:1"),
		"token":           {URL: "nats://127.0.0.1:1", Token: testToken, ConnectTimeout: 300 * time.Millisecond},
		"password":        {URL: "nats://127.0.0.1:1", User: "oagent", Password: testPassword, ConnectTimeout: 300 * time.Millisecond},
	} {
		t.Run(name, func(t *testing.T) {
			log, logs := bufferLogger()
			_, err := Open(t.Context(), c, log)
			require.Error(t, err)
			assert.ErrorContains(t, err, "nats: cannot connect to 127.0.0.1:1 (nats.url)")
			assertNoSecrets(t, err.Error())
			assertNoSecrets(t, logs.String())
		})
	}
}

func TestOpen_BadURLIsNotEchoed(t *testing.T) {
	_, err := Open(t.Context(), testConfig("nats://oagent:"+testPassword+"@nats internal:4222"), discard())
	require.Error(t, err)
	assert.ErrorContains(t, err, "nats.url")
	assertNoSecrets(t, err.Error())
	assert.NotContains(t, err.Error(), "internal")
}

func TestOpen_WrongPasswordIsRefusedWithoutEcho(t *testing.T) {
	ns := natstest.StartAccounts(t)
	c := testConfig(ns.ClientURL())
	c.User, c.Password = natstest.UserA, testPassword

	_, err := Open(t.Context(), c, discard())
	require.Error(t, err)
	assert.ErrorContains(t, err, "Authorization Violation")
	assertNoSecrets(t, err.Error())
}

func TestOpen_AccountUserWithClientName(t *testing.T) {
	ns := natstest.StartAccounts(t)
	c := testConfig(ns.ClientURL())
	c.User, c.Password, c.Name = natstest.UserA, natstest.PasswordA, "oagent-test"

	nc, err := Open(t.Context(), c, discard())
	require.NoError(t, err)
	t.Cleanup(nc.Close)
	assert.True(t, nc.IsConnected())
	connz, err := ns.Connz(&server.ConnzOptions{Username: true})
	require.NoError(t, err)
	require.Len(t, connz.Conns, 1)
	assert.Equal(t, "oagent-test", connz.Conns[0].Name)
	assert.Equal(t, natstest.UserA, connz.Conns[0].AuthorizedUser)
}

func TestOpen_CredentialsInTheURL(t *testing.T) {
	ns := natstest.StartAccounts(t)
	url := "nats://" + natstest.UserB + ":" + natstest.PasswordB + "@" + ns.Addr().String()
	nc, err := Open(t.Context(), testConfig(url), discard())
	require.NoError(t, err)
	t.Cleanup(nc.Close)
	assert.True(t, nc.IsConnected())
}

func TestOpen_Token(t *testing.T) {
	opts := &server.Options{Host: "127.0.0.1", Port: -1, Authorization: testToken, NoLog: true, NoSigs: true}
	ns, err := server.NewServer(opts)
	require.NoError(t, err)
	go ns.Start()
	require.True(t, ns.ReadyForConnections(5*time.Second))
	t.Cleanup(ns.Shutdown)

	c := testConfig(ns.ClientURL())
	c.Token = testToken
	nc, err := Open(t.Context(), c, discard())
	require.NoError(t, err)
	t.Cleanup(nc.Close)
	assert.True(t, nc.IsConnected())
}

func TestOpen_DeadPortFailsAfterConnectTimeout(t *testing.T) {
	ns := natstest.Start(t)
	url := ns.ClientURL()
	ns.Shutdown()

	c := testConfig(url)
	c.ConnectTimeout = time.Second
	start := time.Now()
	_, err := Open(t.Context(), c, discard())
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.GreaterOrEqual(t, elapsed, time.Second)
	assert.Less(t, elapsed, 4*time.Second)
}

func TestOpen_ZeroConnectTimeoutUsesTheDefault(t *testing.T) {
	ns := natstest.Start(t)
	c := testConfig(ns.ClientURL())
	c.ConnectTimeout = 0
	nc, err := Open(t.Context(), c, discard())
	require.NoError(t, err)
	nc.Close()
}

func TestOpen_CancelledContextStopsRetrying(t *testing.T) {
	ns := natstest.Start(t)
	url := ns.ClientURL()
	ns.Shutdown()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	c := testConfig(url)
	c.ConnectTimeout = 10 * time.Second
	start := time.Now()
	_, err := Open(ctx, c, discard())

	require.ErrorIs(t, err, context.Canceled)
	assert.Less(t, time.Since(start), 2*time.Second)
}

func TestOpen_DisconnectIsLoggedWithoutCredentials(t *testing.T) {
	ns := natstest.StartAccounts(t)
	url := "nats://" + natstest.UserA + ":" + natstest.PasswordA + "@" + ns.Addr().String()
	logs := &lockedBuffer{}
	nc, err := Open(t.Context(), testConfig(url), slog.New(slog.NewTextHandler(logs, nil)))
	require.NoError(t, err)
	t.Cleanup(nc.Close)

	ns.Shutdown()
	require.Eventually(t, func() bool { return strings.Contains(logs.String(), "nats: disconnected") }, 5*time.Second, 10*time.Millisecond)
	assert.NotContains(t, logs.String(), natstest.PasswordA)
}

func TestCloser_ClosedTwiceIsFine(t *testing.T) {
	ns := natstest.Start(t)
	nc, err := natsgo.Connect(ns.ClientURL())
	require.NoError(t, err)
	c := Closer(nc)
	require.NoError(t, c.Close())
	assert.True(t, nc.IsClosed())
	require.NoError(t, c.Close())
}

func TestRedact_RemovesEverySecret(t *testing.T) {
	c := Config{
		URL:      "nats://a.internal:4222,nats://oagent:" + testPassword + "x@b.internal:4222",
		Token:    testToken,
		Password: testPassword,
	}
	err := redact(errors.New("auth failed for "+testPassword+"x and "+testToken), c)
	assertNoSecrets(t, err.Error())
	assert.Contains(t, err.Error(), "[redacted]")

	plain := errors.New("connection refused")
	assert.Same(t, plain, redact(plain, c), "an error without secrets keeps its chain")
	assert.NoError(t, redact(nil, c))
}

func TestRedact_TokenInTheURL(t *testing.T) {
	c := Config{URL: "nats://" + testToken + "@a.internal:4222"}
	err := redact(errors.New("server said "+testToken), c)
	assertNoSecrets(t, err.Error())
}

func TestOpen_AsyncErrorsGoToTheLogRedacted(t *testing.T) {
	opts := &server.Options{}
	require.NoError(t, opts.ProcessConfigString(`
listen: "127.0.0.1:-1"
authorization {
  users: [{ user: oagent, password: "`+testPassword+`", permissions: { publish: { deny: "deny.>" } } }]
}
`))
	opts.NoLog, opts.NoSigs = true, true
	ns, err := server.NewServer(opts)
	require.NoError(t, err)
	go ns.Start()
	require.True(t, ns.ReadyForConnections(5*time.Second))
	t.Cleanup(ns.Shutdown)

	logs := &lockedBuffer{}
	c := testConfig("nats://" + ns.Addr().String())
	c.User, c.Password = "oagent", testPassword
	nc, err := Open(t.Context(), c, slog.New(slog.NewTextHandler(logs, nil)))
	require.NoError(t, err)
	t.Cleanup(nc.Close)

	require.NoError(t, nc.Publish("deny."+testPassword, []byte("x")))
	slow, err := nc.SubscribeSync("slow")
	require.NoError(t, err)
	require.NoError(t, slow.SetPendingLimits(1, -1))
	for range 3 {
		require.NoError(t, nc.Publish("slow", []byte("x")))
	}
	require.NoError(t, nc.Flush())

	require.Eventually(t, func() bool {
		return strings.Count(logs.String(), `level=WARN msg="nats: async error"`) >= 2
	}, 5*time.Second, 10*time.Millisecond)
	assert.Contains(t, logs.String(), "subject=slow")
	assert.Contains(t, logs.String(), "[redacted]")
	assertNoSecrets(t, logs.String())
}

func TestOpen_ReconnectFailureIsLoggedWithoutCredentials(t *testing.T) {
	ns := natstest.StartAccounts(t)
	url := "nats://" + natstest.UserA + ":" + natstest.PasswordA + "@" + ns.Addr().String()
	logs := &lockedBuffer{}
	nc, err := Open(t.Context(), testConfig(url), slog.New(slog.NewTextHandler(logs, nil)))
	require.NoError(t, err)
	t.Cleanup(nc.Close)

	ns.Shutdown()
	require.Eventually(t, func() bool {
		return strings.Contains(logs.String(), `level=WARN msg="nats: reconnect failed"`)
	}, 10*time.Second, 20*time.Millisecond)
	assert.NotContains(t, logs.String(), natstest.PasswordA)
}

func TestEvents_ReconnectFailuresWarnOncePerOutage(t *testing.T) {
	logs := &lockedBuffer{}
	c := Config{URL: "nats://oagent:" + testPassword + "@a.internal:4222", Password: testPassword}
	e := &events{c: c, log: slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))}
	failed := errors.New("dial " + testPassword + ": connection refused")

	e.reconnectFailed(nil, failed)
	e.reconnectFailed(nil, failed)
	e.reconnectFailed(nil, failed)
	e.reconnected(nil)
	e.reconnectFailed(nil, failed)
	e.reconnectFailed(nil, failed)

	var levels []string
	for line := range strings.Lines(logs.String()) {
		level, _, _ := strings.Cut(strings.SplitN(line, "level=", 2)[1], " ")
		msg := strings.SplitN(line, "msg=", 2)[1]
		levels = append(levels, level+" "+strings.SplitN(msg, `"`, 3)[1])
	}
	assert.Equal(t, []string{
		"WARN nats: reconnect failed",
		"DEBUG nats: reconnect failed",
		"DEBUG nats: reconnect failed",
		"INFO nats: reconnected",
		"WARN nats: reconnect failed",
		"DEBUG nats: reconnect failed",
	}, levels)
	assertNoSecrets(t, logs.String())
}

func TestEvents_ConcurrentCallbacks(t *testing.T) {
	e := &events{log: discard()}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			e.reconnectFailed(nil, errors.New("refused"))
			e.reconnected(nil)
		})
	}
	wg.Wait()
}

func TestCloser_ConcurrentCloseRunsOnce(t *testing.T) {
	ns := natstest.Start(t)
	nc, err := natsgo.Connect(ns.ClientURL())
	require.NoError(t, err)
	_, err = nc.Subscribe("busy", func(*natsgo.Msg) {})
	require.NoError(t, err)
	c := Closer(nc)

	start := time.Now()
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Go(func() { errs[i] = c.Close() })
	}
	wg.Wait()

	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	assert.True(t, nc.IsClosed())
	assert.Less(t, time.Since(start), drainTimeout)
}
