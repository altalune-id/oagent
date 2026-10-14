package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// NOTE: precedence is --token, then --token-file, then OAGENT_TOKEN.
func resolveToken(cmd *cobra.Command) (string, error) {
	root := cmd.Root()
	if f := root.PersistentFlags().Lookup("token"); f != nil && f.Changed {
		return strings.TrimSpace(f.Value.String()), nil
	}
	if f := root.PersistentFlags().Lookup("token-file"); f != nil && f.Changed {
		b, err := os.ReadFile(f.Value.String()) //nolint:gosec // operator-supplied path, not tainted input.
		if err != nil {
			return "", fmt.Errorf("cli: read token file: %w", err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	return strings.TrimSpace(os.Getenv("OAGENT_TOKEN")), nil
}
