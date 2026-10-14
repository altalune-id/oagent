// Package cli builds the cobra command tree — every command is a factory so tests get a fresh tree.
package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/config"
	agentv1connect "altalune.id/oagent/gen/go/agent/v1/agentv1connect"
	apikeyv1connect "altalune.id/oagent/gen/go/apikey/v1/apikeyv1connect"
	authv1connect "altalune.id/oagent/gen/go/auth/v1/authv1connect"
	invitev1connect "altalune.id/oagent/gen/go/invite/v1/invitev1connect"
	memberv1connect "altalune.id/oagent/gen/go/member/v1/memberv1connect"
	orgv1connect "altalune.id/oagent/gen/go/org/v1/orgv1connect"
	projectv1connect "altalune.id/oagent/gen/go/project/v1/projectv1connect"
	webhookv1connect "altalune.id/oagent/gen/go/webhook/v1/webhookv1connect"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/apiclient"
	"altalune.id/oagent/internal/apikey"
	"altalune.id/oagent/internal/auth"
	"altalune.id/oagent/internal/boot"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/org/invite"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/user"
	"altalune.id/oagent/internal/webhook"
	"altalune.id/oagent/reqid"
	"altalune.id/oagent/version"
)

// App is the running server the serve, mcp, user and member add commands drive.
type App interface {
	Run(ctx context.Context) error
	RunMCP(ctx context.Context) error
	SetPassword(ctx context.Context, email, password string) error
	AddMember(ctx context.Context, email, orgPublicID string, role member.Role) (*member.Member, error)
	Close(ctx context.Context) error
}

// ServerBootFn boots the fully-wired server graph (DB, migrations, services).
type ServerBootFn func(ctx context.Context, cfg config.Config, log *slog.Logger, opts ...boot.Option) (App, error)

// ClientBootFn builds the remote client graph admin commands use (no DB).
type ClientBootFn func(
	ctx context.Context, cfg config.Config, log *slog.Logger, baseURL string, cred apiclient.Credential,
) (*apiclient.Client, error)

type ctxKey int

const ctxKeyRuntime ctxKey = iota + 1

// NOTE: commands carrying this annotation load config without validation, so they need no DB.
const clientAnnotation = "oagent/client"

const operatorAnnotation = "oagent/operator"

type runtime struct {
	cfg *config.Config
	log *slog.Logger
}

// NewRootCmd builds a fresh command tree bound to the given boot fns.
func NewRootCmd(bootServer ServerBootFn, bootClient ClientBootFn) *cobra.Command {
	var configPath, logLevel, logFormat string

	root := &cobra.Command{
		Use:           "oagent",
		Short:         "oagent — build, manage and integrate custom AI agents",
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
	p.String("profile", "", "signed-in profile to use (also: OAGENT_PROFILE; default \"default\")")
	p.String("output", "", "output format: text|json|ndjson (also: OAGENT_OUTPUT)")
	registerConfigFlags(p)

	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		switch cmd.Name() {
		case "version", "completion", "help":
			return nil
		}
		if err := refuseRemoteFlags(cmd); err != nil {
			return err
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

	memberCmd := member.NewCmd(memberClientFn(bootClient))
	markClientTree(memberCmd)
	memberCmd.AddCommand(markOperator(member.NewAddCmd(addMemberFn(bootServer))))

	inviteCmd := invite.NewCmd(inviteClientFn(bootClient))
	markClientTree(inviteCmd)

	projectCmd := project.NewCmd(projectClientFn(bootClient))
	markClientTree(projectCmd)

	agentCmd := agent.NewCmd(agentClientFn(bootClient))
	markClientTree(agentCmd)

	apiKeyCmd := apikey.NewCmd(apiKeyClientFn(bootClient))
	markClientTree(apiKeyCmd)

	webhookCmd := webhook.NewCmd(webhookClientFn(bootClient))
	markClientTree(webhookCmd)

	authCmd := auth.NewCmd(sessionProfiles{}, authClientFn(bootClient), authSessionClientFn(bootClient))
	markClientTree(authCmd)

	root.AddCommand(
		newServeCmd(bootServer),
		newMCPCmd(bootServer),
		markOperator(user.NewCmd(setPasswordFn(bootServer))),
		authCmd,
		orgCmd,
		memberCmd,
		inviteCmd,
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

func connectClient(cmd *cobra.Command, bootClient ClientBootFn) (*apiclient.Client, error) {
	rt, err := runtimeFromCtx(cmd.Context())
	if err != nil {
		return nil, err
	}
	r, err := resolveRemote(cmd)
	if err != nil {
		return nil, err
	}
	return bootClient(cmd.Context(), *rt.cfg, rt.log, r.url, r.cred)
}

func orgClientFn(bootClient ClientBootFn) org.ClientFn {
	return func(cmd *cobra.Command) (orgv1connect.OrgServiceClient, error) {
		client, err := connectClient(cmd, bootClient)
		if err != nil {
			return nil, err
		}
		return client.Org, nil
	}
}

func memberClientFn(bootClient ClientBootFn) member.ClientFn {
	return func(cmd *cobra.Command) (memberv1connect.MemberServiceClient, error) {
		client, err := connectClient(cmd, bootClient)
		if err != nil {
			return nil, err
		}
		return client.Members, nil
	}
}

func inviteClientFn(bootClient ClientBootFn) invite.ClientFn {
	return func(cmd *cobra.Command) (invitev1connect.InviteServiceClient, error) {
		client, err := connectClient(cmd, bootClient)
		if err != nil {
			return nil, err
		}
		return client.Invites, nil
	}
}

func projectClientFn(bootClient ClientBootFn) project.ClientFn {
	return func(cmd *cobra.Command) (projectv1connect.ProjectServiceClient, error) {
		client, err := connectClient(cmd, bootClient)
		if err != nil {
			return nil, err
		}
		return client.Projects, nil
	}
}

func agentClientFn(bootClient ClientBootFn) agent.ClientFn {
	return func(cmd *cobra.Command) (agentv1connect.AgentServiceClient, error) {
		client, err := connectClient(cmd, bootClient)
		if err != nil {
			return nil, err
		}
		return client.Agents, nil
	}
}

func apiKeyClientFn(bootClient ClientBootFn) apikey.ClientFn {
	return func(cmd *cobra.Command) (apikeyv1connect.APIKeyServiceClient, error) {
		client, err := connectClient(cmd, bootClient)
		if err != nil {
			return nil, err
		}
		return client.APIKeys, nil
	}
}

func webhookClientFn(bootClient ClientBootFn) webhook.ClientFn {
	return func(cmd *cobra.Command) (webhookv1connect.WebhookServiceClient, error) {
		client, err := connectClient(cmd, bootClient)
		if err != nil {
			return nil, err
		}
		return client.Webhooks, nil
	}
}

func authClientFn(bootClient ClientBootFn) auth.ClientFn {
	return func(cmd *cobra.Command) (authv1connect.AuthServiceClient, bool, error) {
		rt, err := runtimeFromCtx(cmd.Context())
		if err != nil {
			return nil, false, err
		}
		r, err := resolveRemote(cmd)
		if err != nil {
			return nil, false, err
		}
		client, err := bootClient(cmd.Context(), *rt.cfg, rt.log, r.url, r.cred)
		if err != nil {
			return nil, false, err
		}
		return client.Auth, r.fromProfile(), nil
	}
}

func authSessionClientFn(bootClient ClientBootFn) auth.SessionClientFn {
	return func(cmd *cobra.Command, url, sid string) (authv1connect.AuthServiceClient, error) {
		rt, err := runtimeFromCtx(cmd.Context())
		if err != nil {
			return nil, err
		}
		client, err := bootClient(cmd.Context(), *rt.cfg, rt.log, url, apiclient.Credential{Session: sid})
		if err != nil {
			return nil, err
		}
		return client.Auth, nil
	}
}

func setPasswordFn(bootServer ServerBootFn) user.SetPasswordFn {
	return func(ctx context.Context, email, password string) error {
		return withOperatorApp(ctx, bootServer, func(app App) error {
			return app.SetPassword(ctx, email, password)
		})
	}
}

func addMemberFn(bootServer ServerBootFn) member.AddFn {
	return func(ctx context.Context, email, orgPublicID string, role member.Role) (m *member.Member, err error) {
		err = withOperatorApp(ctx, bootServer, func(app App) error {
			m, err = app.AddMember(ctx, email, orgPublicID, role)
			return err
		})
		return m, err
	}
}

func withOperatorApp(ctx context.Context, bootServer ServerBootFn, run func(App) error) error {
	rt, err := runtimeFromCtx(ctx)
	if err != nil {
		return err
	}
	app, err := bootServer(ctx, *rt.cfg, rt.log,
		boot.WithInstallURL(false),
		boot.WithScheduler(false),
		boot.WithConsumer(false),
	)
	if err != nil {
		return err
	}
	defer func() { _ = app.Close(context.WithoutCancel(ctx)) }()
	return run(app)
}

func markClientTree(cmd *cobra.Command) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[clientAnnotation] = "true"
	for _, sub := range cmd.Commands() {
		markClientTree(sub)
	}
}

func markOperator(cmd *cobra.Command) *cobra.Command {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[operatorAnnotation] = "true"
	for _, sub := range cmd.Commands() {
		markOperator(sub)
	}
	return cmd
}

func refuseRemoteFlags(cmd *cobra.Command) error {
	if cmd.Annotations[operatorAnnotation] != "true" {
		return nil
	}
	for _, name := range []string{"url", "token", "token-file", "profile"} {
		if f := cmd.Flags().Lookup(name); f != nil && f.Changed {
			path := strings.TrimPrefix(cmd.CommandPath(), cmd.Root().Name()+" ")
			return apperror.New(apperror.CodeValidation, fmt.Sprintf(
				"%s runs on the server with database access; it can't use --%s. Run it on the server without --url, --token, --token-file or --profile",
				path, name))
		}
	}
	return nil
}

func registerConfigFlags(fs *pflag.FlagSet) {
	fs.String("mode", "", "config override: mode (selfhosted|cloud)")
	fs.String("env", "", "config override: env (development|production)")
	fs.String("http-addr", "", "config override: http.addr")
	fs.String("http-base-path", "", "config override: http.basePath")
	fs.String("postgres-dsn", "", "config override: postgres.dsn")
	fs.String("postgres-schema", "", "config override: postgres.schema")
	fs.String("postgres-table-prefix", "", "config override: postgres.tablePrefix")
	fs.Bool("db-auto-migrate", false, "config override: db.autoMigrate")
	fs.Bool("scheduler-enabled", false, "config override: scheduler.enabled")
	fs.Bool("queue-enabled", false, "config override: queue.enabled")
	fs.String("nats-url", "", "config override: nats.url")
}

func bindConfigFlags(loader *config.Loader, cmd *cobra.Command) error {
	pairs := []struct{ key, flag string }{
		{"mode", "mode"},
		{"env", "env"},
		{"http.addr", "http-addr"},
		{"http.basePath", "http-base-path"},
		{"postgres.dsn", "postgres-dsn"},
		{"postgres.schema", "postgres-schema"},
		{"postgres.tablePrefix", "postgres-table-prefix"},
		{"db.autoMigrate", "db-auto-migrate"},
		{"scheduler.enabled", "scheduler-enabled"},
		{"queue.enabled", "queue-enabled"},
		{"nats.url", "nats-url"},
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
