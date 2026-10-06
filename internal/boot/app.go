package boot

import (
	"context"
	"errors"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"altalune.id/oagent/internal/kernel"
	rootmcp "altalune.id/oagent/mcp"
	"altalune.id/oagent/worker"
)

// App is the wired runtime graph.
type App struct {
	Kernel     kernel.Kernel
	Services   *Services
	Supervisor *worker.Supervisor
	MCP        *rootmcp.Server

	shutdownTelemetry func(context.Context) error
}

// Run starts every supervised worker until ctx is cancelled or one fails.
func (a *App) Run(ctx context.Context) error { return a.Supervisor.Run(ctx) }

// RunMCP runs the MCP server over stdio until ctx is cancelled or the client disconnects.
func (a *App) RunMCP(ctx context.Context) error {
	if a.MCP == nil {
		return errors.New("boot: mcp is disabled (set mcp.enabled)")
	}
	return a.MCP.SDK().Run(ctx, &sdkmcp.StdioTransport{})
}

// Close drains telemetry then closes the kernel's resources.
func (a *App) Close(ctx context.Context) error {
	err := a.shutdownTelemetry(ctx)
	if cerr := a.Kernel.Close(); cerr != nil && err == nil {
		err = cerr
	}
	return err
}
