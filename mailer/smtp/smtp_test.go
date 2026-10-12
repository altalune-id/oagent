package smtp

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"net/textproto"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/mailer"
)

type fakeSMTPOpts struct {
	implicitTLS bool
	offerTLS    bool
	hang        bool
	// reply answers a verb with this line instead of the default; "drop" closes the connection.
	reply map[string]string
	// afterData answers the end of the message body instead of "250 queued".
	afterData string
}

type fakeSMTP struct {
	t       *testing.T
	ln      net.Listener
	opts    fakeSMTPOpts
	tlsCfg  *tls.Config
	roots   *x509.CertPool
	mu      sync.Mutex
	cmds    []string
	data    []byte
	usedTLS bool
	done    chan struct{}
}

func testCert(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	ts := httptest.NewTLSServer(http.NotFoundHandler())
	defer ts.Close()
	pool := x509.NewCertPool()
	pool.AddCert(ts.Certificate())
	return ts.TLS.Certificates[0], pool
}

func newFakeSMTP(t *testing.T, opts fakeSMTPOpts) *fakeSMTP {
	t.Helper()
	cert, pool := testCert(t)
	tlsCfg := &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	var (
		ln  net.Listener
		err error
	)
	if opts.implicitTLS {
		ln, err = tls.Listen("tcp", "127.0.0.1:0", tlsCfg)
	} else {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	require.NoError(t, err)
	f := &fakeSMTP{t: t, ln: ln, opts: opts, tlsCfg: tlsCfg, roots: pool, done: make(chan struct{})}
	go f.serve()
	t.Cleanup(func() {
		_ = ln.Close()
		<-f.done
	})
	return f
}

func (f *fakeSMTP) port() int {
	_, p, _ := net.SplitHostPort(f.ln.Addr().String())
	n, _ := strconv.Atoi(p)
	return n
}

func (f *fakeSMTP) serve() {
	defer close(f.done)
	conn, err := f.ln.Accept()
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	if f.opts.hang {
		_, _ = io.Copy(io.Discard, conn)
		return
	}
	_, isTLS := conn.(*tls.Conn)
	f.setTLS(isTLS)
	tp := textproto.NewConn(conn)
	_ = tp.PrintfLine("220 fake ESMTP")
	for {
		line, err := tp.ReadLine()
		if err != nil {
			return
		}
		f.record(line)
		verb := strings.ToUpper(strings.SplitN(line, " ", 2)[0])
		if r, ok := f.opts.reply[verb]; ok {
			if r == "drop" {
				return
			}
			_ = tp.PrintfLine("%s", r)
			continue
		}
		switch verb {
		case "EHLO":
			_ = tp.PrintfLine("250-fake")
			if f.opts.offerTLS && !isTLS {
				_ = tp.PrintfLine("250-STARTTLS")
			}
			_ = tp.PrintfLine("250 AUTH PLAIN")
		case "STARTTLS":
			_ = tp.PrintfLine("220 go ahead")
			tc := tls.Server(conn, f.tlsCfg)
			if tc.Handshake() != nil {
				return
			}
			conn, isTLS = tc, true
			f.setTLS(true)
			tp = textproto.NewConn(conn)
		case "AUTH":
			_ = tp.PrintfLine("235 ok")
		case "MAIL", "RCPT", "RSET", "NOOP":
			_ = tp.PrintfLine("250 ok")
		case "DATA":
			_ = tp.PrintfLine("354 go")
			data, err := tp.ReadDotBytes()
			if err != nil {
				return
			}
			f.mu.Lock()
			f.data = data
			f.mu.Unlock()
			_ = tp.PrintfLine("%s", cmp.Or(f.opts.afterData, "250 queued"))
		case "QUIT":
			_ = tp.PrintfLine("221 bye")
			return
		default:
			_ = tp.PrintfLine("502 unknown")
		}
	}
}

func (f *fakeSMTP) record(line string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cmds = append(f.cmds, line)
}

func (f *fakeSMTP) setTLS(v bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.usedTLS = v
}

func (f *fakeSMTP) result() (cmds []string, data []byte, usedTLS bool) {
	<-f.done
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cmds, f.data, f.usedTLS
}

func (f *fakeSMTP) client(t *testing.T, cfg Config) *Sender {
	t.Helper()
	cfg.Host = "127.0.0.1"
	cfg.Port = f.port()
	s, err := New("Sender <f@example.com>", cfg)
	require.NoError(t, err)
	s.rootCAs = f.roots
	return s
}

func smtpMessage() mailer.Message {
	return mailer.Message{To: "t@example.com", Subject: "Hello é", TextBody: "plain line", HTMLBody: "<p>rich line</p>"}
}

func readMessage(t *testing.T, data []byte) *mail.Message {
	t.Helper()
	msg, err := mail.ReadMessage(bytes.NewReader(data))
	require.NoError(t, err)
	return msg
}

func TestSMTP_Send_MultipartAlternative(t *testing.T) {
	f := newFakeSMTP(t, fakeSMTPOpts{})
	s := f.client(t, Config{TLS: TLSNone})

	require.NoError(t, s.Send(t.Context(), smtpMessage()))

	cmds, data, _ := f.result()
	assert.Contains(t, cmds, "MAIL FROM:<f@example.com>")
	assert.Contains(t, cmds, "RCPT TO:<t@example.com>")

	msg := readMessage(t, data)
	subject, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	require.NoError(t, err)
	assert.Equal(t, "Hello é", subject)
	assert.Equal(t, "1.0", msg.Header.Get("MIME-Version"))
	assert.NotEmpty(t, msg.Header.Get("Date"))
	assert.Contains(t, msg.Header.Get("Message-ID"), "@example.com>")
	from, err := msg.Header.AddressList("From")
	require.NoError(t, err)
	assert.Equal(t, "Sender", from[0].Name)

	mediaType, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	require.NoError(t, err)
	require.Equal(t, "multipart/alternative", mediaType)

	mr := multipart.NewReader(msg.Body, params["boundary"])
	var types, bodies []string
	for {
		p, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		b, err := io.ReadAll(p)
		require.NoError(t, err)
		types = append(types, p.Header.Get("Content-Type"))
		bodies = append(bodies, string(b))
	}
	assert.Equal(t, []string{"text/plain; charset=utf-8", "text/html; charset=utf-8"}, types)
	assert.Equal(t, []string{"plain line", "<p>rich line</p>"}, bodies)
}

func TestSMTP_Send_SinglePart(t *testing.T) {
	for name, tc := range map[string]struct {
		m    mailer.Message
		ct   string
		body string
	}{
		"text only": {mailer.Message{To: "t@example.com", Subject: "s", TextBody: "only text"}, "text/plain; charset=utf-8", "only text"},
		"html only": {mailer.Message{To: "t@example.com", Subject: "s", HTMLBody: "<p>only html</p>"}, "text/html; charset=utf-8", "<p>only html</p>"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFakeSMTP(t, fakeSMTPOpts{})
			require.NoError(t, f.client(t, Config{TLS: TLSNone}).Send(t.Context(), tc.m))
			_, data, _ := f.result()
			msg := readMessage(t, data)
			assert.Equal(t, tc.ct, msg.Header.Get("Content-Type"))
			assert.Equal(t, "quoted-printable", msg.Header.Get("Content-Transfer-Encoding"))
			assert.NotContains(t, string(data), "multipart")
		})
	}
}

func TestSMTP_Send_ImplicitTLS(t *testing.T) {
	f := newFakeSMTP(t, fakeSMTPOpts{implicitTLS: true})
	s := f.client(t, Config{TLS: TLSImplicit, Username: "u", Password: "p"})

	require.NoError(t, s.Send(t.Context(), smtpMessage()))

	cmds, data, usedTLS := f.result()
	assert.True(t, usedTLS)
	assert.NotEmpty(t, data)
	assert.True(t, hasPrefix(cmds, "AUTH PLAIN"), "cmds: %v", cmds)
}

func TestSMTP_Send_ImplicitTLSRejectsUntrustedCert(t *testing.T) {
	f := newFakeSMTP(t, fakeSMTPOpts{implicitTLS: true})
	s := f.client(t, Config{TLS: TLSImplicit})
	s.rootCAs = x509.NewCertPool()

	require.Error(t, s.Send(t.Context(), smtpMessage()))
}

func TestSMTP_Send_StartTLS(t *testing.T) {
	f := newFakeSMTP(t, fakeSMTPOpts{offerTLS: true})
	s := f.client(t, Config{TLS: TLSStartTLS, Username: "u", Password: "p"})

	require.NoError(t, s.Send(t.Context(), smtpMessage()))

	cmds, data, usedTLS := f.result()
	assert.True(t, usedTLS)
	assert.Contains(t, cmds, "STARTTLS")
	assert.True(t, hasPrefix(cmds, "AUTH PLAIN"), "cmds: %v", cmds)
	assert.NotEmpty(t, data)
}

func TestSMTP_Send_StartTLSRequiredButNotOffered(t *testing.T) {
	f := newFakeSMTP(t, fakeSMTPOpts{})
	s := f.client(t, Config{TLS: TLSStartTLS})

	err := s.Send(t.Context(), smtpMessage())
	assert.True(t, IsStartTLSUnsupportedError(err), "got %T: %v", err, err)

	cmds, data, _ := f.result()
	assert.Empty(t, data)
	assert.False(t, hasPrefix(cmds, "MAIL"), "no mail may be sent in the clear: %v", cmds)
}

func TestSMTP_Send_PlainWhenTLSNone(t *testing.T) {
	f := newFakeSMTP(t, fakeSMTPOpts{offerTLS: true})
	s := f.client(t, Config{TLS: TLSNone})

	require.NoError(t, s.Send(t.Context(), smtpMessage()))

	cmds, data, usedTLS := f.result()
	assert.False(t, usedTLS)
	assert.NotContains(t, cmds, "STARTTLS")
	assert.NotEmpty(t, data)
}

func TestSMTP_Send_DeadlineAbortsHangingServer(t *testing.T) {
	f := newFakeSMTP(t, fakeSMTPOpts{hang: true})
	s := f.client(t, Config{TLS: TLSNone})

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := s.Send(ctx, smtpMessage())
	require.Error(t, err)
	assert.Less(t, time.Since(start), 2*time.Second)
}

func TestSMTP_Send_TimeoutAbortsHangingServer(t *testing.T) {
	f := newFakeSMTP(t, fakeSMTPOpts{hang: true})
	s := f.client(t, Config{TLS: TLSNone, Timeout: 100 * time.Millisecond})

	start := time.Now()
	require.Error(t, s.Send(t.Context(), smtpMessage()))
	assert.Less(t, time.Since(start), 2*time.Second)
}

func TestSMTP_Send_ServerRejectsAStep(t *testing.T) {
	cases := map[string]struct {
		opts fakeSMTPOpts
		cfg  Config
		step string
		code int
	}{
		"auth":      {fakeSMTPOpts{reply: map[string]string{"AUTH": "535 bad credentials"}}, Config{Username: "u", Password: "p"}, "auth", 535},
		"mail from": {fakeSMTPOpts{reply: map[string]string{"MAIL": "550 sender refused"}}, Config{}, "mail from", 550},
		"rcpt to":   {fakeSMTPOpts{reply: map[string]string{"RCPT": "550 no such user"}}, Config{}, "rcpt to", 550},
		"data":      {fakeSMTPOpts{reply: map[string]string{"DATA": "554 no data"}}, Config{}, "data", 554},
		"body":      {fakeSMTPOpts{afterData: "552 too big"}, Config{}, "data", 552},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFakeSMTP(t, tc.opts)
			tc.cfg.TLS = TLSNone
			s := f.client(t, tc.cfg)
			if tc.cfg.Username != "" {
				s.cfg.Host = "localhost"
			}

			err := s.Send(t.Context(), smtpMessage())
			te, ok := errors.AsType[*textproto.Error](err)
			require.True(t, ok, "got %v", err)
			assert.Equal(t, tc.code, te.Code)
			assert.Contains(t, err.Error(), "smtp "+tc.step+":")
		})
	}
}

func TestSMTP_Send_AcceptedMessageIsSentEvenIfQuitFails(t *testing.T) {
	f := newFakeSMTP(t, fakeSMTPOpts{reply: map[string]string{"QUIT": "drop"}})
	s := f.client(t, Config{TLS: TLSNone})

	require.NoError(t, s.Send(t.Context(), smtpMessage()), "the server took the message, so a retry would send it twice")
	_, data, _ := f.result()
	assert.NotEmpty(t, data)
}

func TestSMTP_Send_FailsOnUnreachableHost(t *testing.T) {
	s, err := New("f@x", Config{Host: "127.0.0.1", Port: 1, TLS: TLSNone})
	require.NoError(t, err)
	require.Error(t, s.Send(t.Context(), smtpMessage()))
}

func TestSMTP_Send_RejectsBadMessages(t *testing.T) {
	s, err := New("f@x", Config{Host: "127.0.0.1", Port: 1, TLS: TLSNone})
	require.NoError(t, err)

	m := smtpMessage()
	m.Subject = "a\r\nBcc: e@v"
	assert.True(t, mailer.IsHeaderInjectionError(s.Send(t.Context(), m)))

	m = smtpMessage()
	m.To = "not an address"
	assert.True(t, mailer.IsInvalidAddressError(s.Send(t.Context(), m)))

	m = smtpMessage()
	m.From = "also bad"
	assert.True(t, mailer.IsInvalidAddressError(s.Send(t.Context(), m)))
}

func hasPrefix(cmds []string, prefix string) bool {
	for _, c := range cmds {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

func TestBuildMIME_DotLinesAndLongLines(t *testing.T) {
	from := &mail.Address{Address: "f@example.com"}
	to := &mail.Address{Address: "t@example.com"}
	long := strings.Repeat("x", 200)
	raw, err := buildMIME(from, to, mailer.Message{Subject: "s", TextBody: long}, time.Unix(0, 0))
	require.NoError(t, err)
	sc := bufio.NewScanner(bytes.NewReader(raw))
	for sc.Scan() {
		assert.LessOrEqual(t, len(sc.Text()), 998)
	}
	msg := readMessage(t, raw)
	assert.Equal(t, "quoted-printable", msg.Header.Get("Content-Transfer-Encoding"))
}

func TestNew_Defaults(t *testing.T) {
	s, err := New("f@x", Config{Host: "smtp.example.com"})
	require.NoError(t, err)
	assert.Equal(t, DefaultPort, s.cfg.Port)
	assert.Equal(t, TLSStartTLS, s.cfg.TLS)
	assert.Equal(t, DefaultTimeout, s.cfg.Timeout)
}

func TestNew_Validation(t *testing.T) {
	cases := []struct {
		name  string
		from  string
		cfg   Config
		check func(error) bool
	}{
		{"no from", "", Config{Host: "smtp.example.com"}, mailer.IsMissingConfigError},
		{"no host", "f@x", Config{}, mailer.IsMissingConfigError},
		{"bad tls", "f@x", Config{Host: "h", TLS: "ssl"}, mailer.IsInvalidConfigError},
		{"bad port", "f@x", Config{Host: "h", Port: 70000}, mailer.IsInvalidConfigError},
		{"negative timeout", "f@x", Config{Host: "h", Timeout: -time.Second}, mailer.IsInvalidConfigError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, err := New(tc.from, tc.cfg)
			assert.True(t, tc.check(err), "got %T: %v", err, err)
			assert.Nil(t, s)
		})
	}
}

func TestStartTLSUnsupportedError(t *testing.T) {
	err := &StartTLSUnsupportedError{Host: "h"}
	assert.True(t, strings.HasPrefix(err.Error(), "mailer: "), err.Error())
	assert.False(t, IsStartTLSUnsupportedError(errors.New("x")))
}
