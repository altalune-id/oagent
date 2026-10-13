package boot

import (
	"context"
	"errors"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"altalune.id/oagent/internal/kernel"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/org/member"
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
	return a.runMCP(ctx, &sdkmcp.StdioTransport{})
}

// SECURITY: stdio is trusted (whoever runs it holds the DB DSN), so it acts as the explicit system Principal, never as "no Principal".
func (a *App) runMCP(ctx context.Context, t sdkmcp.Transport) error {
	if a.MCP == nil {
		return errors.New("boot: mcp is disabled (set mcp.enabled)")
	}
	return a.MCP.SDK().Run(session.PrincipalInto(ctx, session.System()), t)
}

// SetPassword replaces the password of the user with email and signs out every session of theirs.
func (a *App) SetPassword(ctx context.Context, email, password string) error {
	return a.Services.Auth.SetPassword(ctx, email, password)
}

// AddMember makes the existing user with email a member of the org orgPublicID, acting as the operator.
func (a *App) AddMember(ctx context.Context, email, orgPublicID string, role member.Role) (*member.Member, error) {
	return a.Services.Members.AddByEmail(ctx, member.Requester{Operator: true}, email, orgPublicID, role)
}

// Close drains telemetry then closes the kernel's resources.
func (a *App) Close(ctx context.Context) error {
	err := a.shutdownTelemetry(ctx)
	if cerr := a.Kernel.Close(); cerr != nil && err == nil {
		err = cerr
	}
	return err
}
