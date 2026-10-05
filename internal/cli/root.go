// Package cli builds the cobra command tree — every command is a factory so tests get a fresh tree.
package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"altalune.id/auth-server/config"
	"altalune.id/auth-server/internal/boot"
	"altalune.id/auth-server/reqid"
	"altalune.id/auth-server/version"
)

// App is the running application the serve command drives.
type App interface {
	Run(ctx context.Context) error
	Close(ctx context.Context) error
}

// ServerBootFn boots the fully-wired server graph (DB, migrations, services).
type ServerBootFn func(ctx context.Context, cfg config.Config, log *slog.Logger, opts ...boot.Option) (App, error)

type ctxKey int

const ctxKeyRuntime ctxKey = iota + 1

type runtime struct {
	cfg *config.Config
	log *slog.Logger
}

// NewRootCmd builds a fresh command tree bound to bootServer.
func NewRootCmd(bootServer ServerBootFn) *cobra.Command {
	var configPath, logLevel, logFormat string

	root := &cobra.Command{
		Use:           "auth-server",
		Short:         "auth-server — authentication and organization service",
		Version:       version.String(),
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	p := root.PersistentFlags()
	p.StringVarP(&configPath, "config", "c", "", "config file (yaml); default search: $XDG_CONFIG_HOME/auth-server, /etc/auth-server, cwd")
	p.StringVar(&logLevel, "log-level", "info", "log level: debug|info|warn|error")
	p.StringVar(&logFormat, "log-format", "text", "log format: json|text")
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
		&cobra.Group{ID: "meta", Title: "Meta:"},
	)

	root.AddCommand(
		newServeCmd(bootServer),
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
