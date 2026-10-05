package cli

import (
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// DefaultURL is the hosted auth-server API used when nothing else is configured.
const DefaultURL = "https://auth-server.altalune.id"

// resolveURL precedence: --url > AUTH_URL > config http.baseURL > DefaultURL.
func resolveURL(cmd *cobra.Command) string {
	if v := explicitURL(cmd); v != "" {
		return v
	}
	if cfg := configFromCtx(cmd.Context()); cfg != nil && cfg.HTTP.BaseURL != "" {
		return cfg.HTTP.BaseURL
	}
	return DefaultURL
}

func explicitURL(cmd *cobra.Command) string {
	if f := cmd.Root().PersistentFlags().Lookup("url"); f != nil && f.Changed {
		return strings.TrimSpace(f.Value.String())
	}
	return strings.TrimSpace(os.Getenv("AUTH_URL"))
}
