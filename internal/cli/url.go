package cli

import (
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// DefaultURL is the hosted oagent API used when nothing else is configured.
const DefaultURL = "https://oagent.altalune.id"

// NOTE: precedence is --url > OAGENT_URL > the profile's URL > config http.baseURL > DefaultURL.
func resolveURL(cmd *cobra.Command, p *profile) string {
	if v := explicitURL(cmd); v != "" {
		return v
	}
	if p != nil && p.URL != "" {
		return p.URL
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
	return strings.TrimSpace(os.Getenv("OAGENT_URL"))
}
