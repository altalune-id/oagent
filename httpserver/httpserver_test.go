package httpserver

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_DefaultName(t *testing.T) {
	assert.Equal(t, "http", New(Config{}, http.NewServeMux(), nil).Name())
}

func TestNew_Name(t *testing.T) {
	assert.Equal(t, "custom", New(Config{Name: "custom", Addr: ":0"}, http.NewServeMux(), nil).Name())
}

func TestRun_ServesAndDrainsOnCancel(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()

	orig := listen
	t.Cleanup(func() { listen = orig })
	listen = func(string, string) (net.Listener, error) { return ln, nil }

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	srv := New(Config{Addr: addr}, handler, nil)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Run(ctx) }()

	var resp *http.Response
	require.Eventually(t, func() bool {
		r, getErr := http.Get("http://" + addr + "/") //nolint:noctx,bodyclose // body closed below once captured
		if getErr != nil {
			return false
		}
		resp = r
		return true
	}, 2*time.Second, 10*time.Millisecond)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, "ok", string(body))

	cancel()
	assert.NoError(t, <-errCh)
}

func TestRun_ListenErrorIsReturned(t *testing.T) {
	srv := New(Config{Addr: "bad:addr:extra"}, http.NewServeMux(), nil)
	assert.Error(t, srv.Run(context.Background()))
}

type errListener struct{}

func (errListener) Accept() (net.Conn, error) { return nil, errors.New("accept boom") }
func (errListener) Close() error              { return nil }
func (errListener) Addr() net.Addr            { return fakeAddr{} }

type fakeAddr struct{}

func (fakeAddr) Network() string { return "tcp" }
func (fakeAddr) String() string  { return "fake" }

func TestRun_ServeErrorIsReturned(t *testing.T) {
	orig := listen
	t.Cleanup(func() { listen = orig })
	listen = func(string, string) (net.Listener, error) { return errListener{}, nil }

	srv := New(Config{Addr: "127.0.0.1:0"}, http.NewServeMux(), nil)
	assert.Error(t, srv.Run(context.Background()))
}
