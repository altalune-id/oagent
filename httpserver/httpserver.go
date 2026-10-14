// Package httpserver adapts an http.Server into a worker.Worker.
package httpserver

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"altalune.id/oagent/worker"
)

//nolint:gochecknoglobals // test seam.
var listen = net.Listen

// New returns a worker.Worker serving h on cfg.Addr with sane timeouts and graceful shutdown.
func New(cfg Config, h http.Handler, log *slog.Logger) worker.Worker {
	if cfg.Name == "" {
		cfg.Name = "http"
	}
	if log == nil {
		log = slog.Default()
	}
	return &httpServer{name: cfg.Name, addr: cfg.Addr, handler: h, log: log}
}

var _ worker.Worker = (*httpServer)(nil)

type httpServer struct {
	name    string
	addr    string
	handler http.Handler
	log     *slog.Logger
}

// Name implements worker.Worker.
func (w *httpServer) Name() string { return w.name }

// Run serves until ctx is cancelled, then drains in-flight requests.
func (w *httpServer) Run(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Serve HTTP/1.1 and unencrypted HTTP/2 (h2c) so the controlplane speaks Connect and gRPC on one port.
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	srv := &http.Server{
		Addr:              w.addr,
		Handler:           w.handler,
		Protocols:         protocols,
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return runCtx },
	}

	ln, err := listen("tcp", w.addr)
	if err != nil {
		return err
	}
	w.log.Info("http worker listening", slog.String("worker", w.name), slog.String("addr", ln.Addr().String()))

	errCh := make(chan error, 1)
	go func() {
		if serveErr := srv.Serve(ln); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- serveErr
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if shutdownErr := srv.Shutdown(shutdownCtx); shutdownErr != nil {
			w.log.Error("http worker shutdown", slog.String("worker", w.name), slog.Any("err", shutdownErr))
			return shutdownErr
		}
		<-errCh
		return nil
	case serveErr := <-errCh:
		return serveErr
	}
}
