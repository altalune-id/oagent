package boot

import (
	"context"

	"altalune.id/auth-server/internal/kernel"
	"altalune.id/auth-server/worker"
)

// App is the wired runtime graph.
type App struct {
	Kernel     kernel.Kernel
	Services   *Services
	Supervisor *worker.Supervisor

	shutdownTelemetry func(context.Context) error
}

// Run starts every supervised worker until ctx is cancelled or one fails.
func (a *App) Run(ctx context.Context) error { return a.Supervisor.Run(ctx) }

// Close drains telemetry then closes the kernel's resources.
func (a *App) Close(ctx context.Context) error {
	err := a.shutdownTelemetry(ctx)
	if cerr := a.Kernel.Close(); cerr != nil && err == nil {
		err = cerr
	}
	return err
}
