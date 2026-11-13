package cli

import (
	"github.com/spf13/cobra"

	"altalune.id/oagent/internal/knowledge/extract"
)

func newExtractCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "extract <kind>",
		Short:       "Read one document from stdin and print its text as JSON; the server runs it as a child process",
		Hidden:      true,
		Annotations: map[string]string{skipConfigAnnotation: "true"},
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return extract.Serve(cmd.Context(), extract.Kind(args[0]), cmd.InOrStdin(), cmd.OutOrStdout(), extract.PDF{})
		},
	}
}
