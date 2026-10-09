// Package cli builds the cobra command tree — every command is a factory so tests get a fresh tree.
package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"altalune.id/oagent/config"
	agentv1connect "altalune.id/oagent/gen/go/agent/v1/agentv1connect"
	apikeyv1connect "altalune.id/oagent/gen/go/apikey/v1/apikeyv1connect"
	orgv1connect "altalune.id/oagent/gen/go/org/v1/orgv1connect"
	projectv1connect "altalune.id/oagent/gen/go/project/v1/projectv1connect"
	webhookv1connect "altalune.id/oagent/gen/go/webhook/v1/webhookv1connect"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/apikey"
	"altalune.id/oagent/internal/boot"
	"altalune.id/oagent/internal/controlplane"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/webhook"
	"altalune.id/oagent/reqid"
	"altalune.id/oagent/version"
)

// App is the running server the serve command drives.
type App interface {
	Run(ctx context.Context) error
	RunMCP(ctx context.Context) error
	Close(ctx context.Context) error
}

// ServerBootFn boots the fully-wired server graph (DB, migrations, services).
type ServerBootFn func(ctx context.Context, cfg config.Config, log *slog.Logger, opts ...boot.Option) (App, error)

// ClientBootFn builds the remote client graph admin commands use (no DB).
type ClientBootFn func(ctx context.Context, cfg config.Config, log *slog.Logger, baseURL, token string) (*controlplane.Client, error)

type ctxKey int

const ctxKeyRuntime ctxKey = iota + 1

// clientAnnotation marks remote commands; their config is loaded without validation (no DB required).
const clientAnnotation = "oagent/client"

type runtime struct {
	cfg *config.Config
	log *slog.Logger
}

// NewRootCmd builds a fresh command tree bound to the given boot fns.
func NewRootCmd(bootServer ServerBootFn, bootClient ClientBootFn) *cobra.Command {
	var configPath, logLevel, logFormat string

	root := &cobra.Command{
		Use:           "oagent",
		Short:         "oagent — authentication and organization service",
		Version:       version.String(),
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	p := root.PersistentFlags()
	p.StringVarP(&configPath, "config", "c", "", "config file (yaml); default search: $XDG_CONFIG_HOME/oagent, /etc/oagent, cwd")
	p.StringVar(&logLevel, "log-level", "info", "log level: debug|info|warn|error")
	p.StringVar(&logFormat, "log-format", "text", "log format: json|text")
	p.String("url", "", "remote oagent base URL (also: OAGENT_URL)")
	p.String("token", "", "bearer token (also: OAGENT_TOKEN)")
	p.String("token-file", "", "read the bearer token from a file")
	p.String("output", "", "output format: text|json|ndjson (also: OAGENT_OUTPUT)")
	registerConfigFlags(p)

	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		switch cmd.Name() {
		case "version", "completion", "help":
			return nil
		}
		var cfgOpts []config.Option
		if configPath != "" {
			cfgOpts = append(cfgOpts, config.WithRequireFile())
		}
		if cmd.Annotations[clientAnnotation] == "true" {
			cfgOpts = append(cfgOpts, config.WithoutValidation())
		}
		loader := config.NewLoader()
		if err := bindConfigFlags(loader, cmd); err != nil {
			return err
		}
		cfg, err := loader.Load(configPath, cfgOpts...)
		if err != nil {
			return err
		}
		log, err := newLogger(logLevel, logFormat, cmd.ErrOrStderr())
		if err != nil {
			return err
		}
		ctx, _ := reqid.Ensure(cmd.Context())
		cmd.SetContext(context.WithValue(ctx, ctxKeyRuntime, &runtime{cfg: cfg, log: log}))
		return nil
	}

	root.AddGroup(
		&cobra.Group{ID: "runtime", Title: "Runtime:"},
		&cobra.Group{ID: "auth", Title: "Auth:"},
		&cobra.Group{ID: "tenancy", Title: "Tenancy:"},
		&cobra.Group{ID: "domain", Title: "Domain:"},
		&cobra.Group{ID: "meta", Title: "Meta:"},
	)

	orgCmd := org.NewCmd(orgClientFn(bootClient))
	markClientTree(orgCmd)

	projectCmd := project.NewCmd(projectClientFn(bootClient))
	markClientTree(projectCmd)

	agentCmd := agent.NewCmd(agentClientFn(bootClient))
	markClientTree(agentCmd)

	apiKeyCmd := apikey.NewCmd(apiKeyClientFn(bootClient))
	markClientTree(apiKeyCmd)

	webhookCmd := webhook.NewCmd(webhookClientFn(bootClient))
	markClientTree(webhookCmd)

	root.AddCommand(
		newServeCmd(bootServer),
		newMCPCmd(bootServer),
		orgCmd,
		projectCmd,
		agentCmd,
		apiKeyCmd,
		webhookCmd,
		newVersionCmd(),
	)
	return root
}

func runtimeFromCtx(ctx context.Context) (*runtime, error) {
	rt, ok := ctx.Value(ctxKeyRuntime).(*runtime)
	if !ok || rt == nil {
		return nil, errors.New("cli: runtime not initialized")
	}
	return rt, nil
}

func configFromCtx(ctx context.Context) *config.Config {
	rt, _ := ctx.Value(ctxKeyRuntime).(*runtime)
	if rt == nil {
		return nil
	}
	return rt.cfg
}

// connectClient resolves URL, token and config, then builds the remote client.
func connectClient(cmd *cobra.Command, bootClient ClientBootFn) (*controlplane.Client, error) {
	rt, err := runtimeFromCtx(cmd.Context())
	if err != nil {
		return nil, err
	}
	token, err := resolveToken(cmd)
	if err != nil {
		return nil, err
	}
	return bootClient(cmd.Context(), *rt.cfg, rt.log, resolveURL(cmd), token)
}

// orgClientFn adapts the resolved remote client to the org package's ClientFn.
func orgClientFn(bootClient ClientBootFn) org.ClientFn {
	return func(cmd *cobra.Command) (orgv1connect.OrgServiceClient, error) {
		client, err := connectClient(cmd, bootClient)
		if err != nil {
			return nil, err
		}
		return client.Org, nil
	}
}

// projectClientFn adapts the resolved remote client to the project package's ClientFn.
func projectClientFn(bootClient ClientBootFn) project.ClientFn {
	return func(cmd *cobra.Command) (projectv1connect.ProjectServiceClient, error) {
		client, err := connectClient(cmd, bootClient)
		if err != nil {
			return nil, err
		}
		return client.Projects, nil
	}
}

// agentClientFn adapts the resolved remote client to the agent package's ClientFn.
func agentClientFn(bootClient ClientBootFn) agent.ClientFn {
	return func(cmd *cobra.Command) (agentv1connect.AgentServiceClient, error) {
		client, err := connectClient(cmd, bootClient)
		if err != nil {
			return nil, err
		}
		return client.Agents, nil
	}
}

// apiKeyClientFn adapts the resolved remote client to the apikey package's ClientFn.
func apiKeyClientFn(bootClient ClientBootFn) apikey.ClientFn {
	return func(cmd *cobra.Command) (apikeyv1connect.APIKeyServiceClient, error) {
		client, err := connectClient(cmd, bootClient)
		if err != nil {
			return nil, err
		}
		return client.APIKeys, nil
	}
}

// webhookClientFn adapts the resolved remote client to the webhook package's ClientFn.
func webhookClientFn(bootClient ClientBootFn) webhook.ClientFn {
	return func(cmd *cobra.Command) (webhookv1connect.WebhookServiceClient, error) {
		client, err := connectClient(cmd, bootClient)
		if err != nil {
			return nil, err
		}
		return client.Webhooks, nil
	}
}

// markClientTree marks a command subtree as remote so its config loads without validation.
func markClientTree(cmd *cobra.Command) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[clientAnnotation] = "true"
	for _, sub := range cmd.Commands() {
		markClientTree(sub)
	}
}

// registerConfigFlags declares the flags that override config keys.
func registerConfigFlags(fs *pflag.FlagSet) {
	fs.String("mode", "", "config override: mode (selfhosted|cloud)")
	fs.String("env", "", "config override: env (development|production)")
	fs.String("http-addr", "", "config override: http.addr")
	fs.String("http-base-path", "", "config override: http.basePath")
	fs.String("db-dsn", "", "config override: db.dsn")
	fs.String("db-schema", "", "config override: db.schema")
	fs.String("db-table-prefix", "", "config override: db.tablePrefix")
	fs.Bool("db-auto-migrate", false, "config override: db.autoMigrate")
	fs.Bool("scheduler-enabled", false, "config override: scheduler.enabled")
	fs.Bool("queue-enabled", false, "config override: queue.enabled")
	fs.String("queue-url", "", "config override: queue.url")
}

// bindConfigFlags binds changed flags to their dotted config keys.
func bindConfigFlags(loader *config.Loader, cmd *cobra.Command) error {
	pairs := []struct{ key, flag string }{
		{"mode", "mode"},
		{"env", "env"},
		{"http.addr", "http-addr"},
		{"http.basePath", "http-base-path"},
		{"db.dsn", "db-dsn"},
		{"db.schema", "db-schema"},
		{"db.tablePrefix", "db-table-prefix"},
		{"db.autoMigrate", "db-auto-migrate"},
		{"scheduler.enabled", "scheduler-enabled"},
		{"queue.enabled", "queue-enabled"},
		{"queue.url", "queue-url"},
	}
	for _, p := range pairs {
		flag := cmd.Flags().Lookup(p.flag)
		if flag == nil {
			return fmt.Errorf("cli: config flag %q not registered", p.flag)
		}
		if !flag.Changed {
			continue
		}
		if err := loader.BindFlag(p.key, flag); err != nil {
			return err
		}
	}
	return nil
}
