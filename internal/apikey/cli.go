package apikey

import (
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/timestamppb"

	apikeyv1 "altalune.id/oagent/gen/go/apikey/v1"
	apikeyv1connect "altalune.id/oagent/gen/go/apikey/v1/apikeyv1connect"
	"altalune.id/oagent/internal/render"
)

// ClientFn returns the APIKey Connect client for a command; the caller resolves URL and token.
type ClientFn func(cmd *cobra.Command) (apikeyv1connect.APIKeyServiceClient, error)

// NewCmd builds the `apikey` command tree.
func NewCmd(clientFn ClientFn) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "apikey",
		Short:   "Manage API keys",
		GroupID: "auth",
	}
	cmd.AddCommand(
		newCreateCmd(clientFn),
		newListCmd(clientFn),
		newRevokeCmd(clientFn),
	)
	return cmd
}

func newCreateCmd(clientFn ClientFn) *cobra.Command {
	var orgID, projectID, kind, name, scopes, projectIDs string
	var allProjects bool
	var expiresIn time.Duration
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an API key",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			expiresAt := timestamppb.New(time.Now().UTC().Add(expiresIn))
			resp, err := client.CreateAPIKey(cmd.Context(), connect.NewRequest(&apikeyv1.CreateAPIKeyRequest{
				OrgId:       orgID,
				ProjectId:   projectID,
				Kind:        kind,
				Name:        name,
				Scopes:      splitCSV(scopes),
				ProjectIds:  splitCSV(projectIDs),
				AllProjects: allProjects,
				ExpiresAt:   expiresAt,
			}))
			if err != nil {
				return err
			}
			k := resp.Msg.GetKey()
			if isJSON(cmd) {
				return render.JSON(cmd.OutOrStdout(), map[string]any{"key": keyMap(k), "plaintext": resp.Msg.GetPlaintext()})
			}
			cmd.Printf("Created API key %s (…%s)\n", k.GetName(), k.GetSecretHint())
			cmd.Printf("Secret: %s\n", resp.Msg.GetPlaintext())
			cmd.PrintErrln("Store this secret now; it will not be shown again.")
			return nil
		},
	}
	cmd.Flags().StringVar(&orgID, "org-id", "", "Organization public id")
	cmd.Flags().StringVar(&kind, "kind", "org", "Key kind: project|org|personal (personal needs a signed-in session)")
	cmd.Flags().StringVar(&projectID, "project-id", "", "Project public id (project keys)")
	cmd.Flags().StringVar(&name, "name", "", "Key name")
	cmd.Flags().StringVar(&scopes, "scopes", "", "Comma-separated scopes")
	cmd.Flags().StringVar(&projectIDs, "project-ids", "", "Comma-separated project public ids (org and personal keys)")
	cmd.Flags().BoolVar(&allProjects, "all-projects", false, "Grant the key every project of the org")
	cmd.Flags().DurationVar(&expiresIn, "expires-in", 720*time.Hour, "Lifetime from now (max 366 days)")
	_ = cmd.MarkFlagRequired("org-id")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func newListCmd(clientFn ClientFn) *cobra.Command {
	var orgID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List an organization's API keys",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			resp, err := client.ListAPIKeys(cmd.Context(), connect.NewRequest(&apikeyv1.ListAPIKeysRequest{OrgId: orgID}))
			if err != nil {
				return err
			}
			keys := resp.Msg.GetKeys()
			if isJSON(cmd) {
				out := make([]map[string]any, 0, len(keys))
				for _, k := range keys {
					out = append(out, keyMap(k))
				}
				return render.JSON(cmd.OutOrStdout(), out)
			}
			rows := make([][]string, 0, len(keys))
			for _, k := range keys {
				rows = append(rows, []string{k.GetSecretHint(), k.GetKind(), k.GetName(), strings.Join(k.GetScopes(), ",")})
			}
			return render.Table(cmd.OutOrStdout(), []string{"HINT", "KIND", "NAME", "SCOPES"}, rows)
		},
	}
	cmd.Flags().StringVar(&orgID, "org-id", "", "Organization public id")
	_ = cmd.MarkFlagRequired("org-id")
	return cmd
}

func newRevokeCmd(clientFn ClientFn) *cobra.Command {
	var orgID, hint string
	cmd := &cobra.Command{
		Use:   "revoke",
		Short: "Revoke an API key by its hint",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			_, err = client.RevokeAPIKey(
				cmd.Context(),
				connect.NewRequest(&apikeyv1.RevokeAPIKeyRequest{OrgId: orgID, Hint: hint}),
			)
			if err != nil {
				return err
			}
			cmd.Printf("Revoked API key …%s\n", hint)
			return nil
		},
	}
	cmd.Flags().StringVar(&orgID, "org-id", "", "Organization public id")
	cmd.Flags().StringVar(&hint, "hint", "", "The key's hint: the last 4 characters of its secret")
	_ = cmd.MarkFlagRequired("org-id")
	_ = cmd.MarkFlagRequired("hint")
	return cmd
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func isJSON(cmd *cobra.Command) bool {
	switch render.Detect(cmd) {
	case render.FormatJSON, render.FormatNDJSON:
		return true
	default:
		return false
	}
}

func keyMap(k *apikeyv1.APIKey) map[string]any {
	return map[string]any{
		"orgId":       k.GetOrgId(),
		"projectId":   k.GetProjectId(),
		"kind":        k.GetKind(),
		"name":        k.GetName(),
		"scopes":      k.GetScopes(),
		"projectIds":  k.GetProjectIds(),
		"allProjects": k.GetAllProjects(),
		"secretHint":  k.GetSecretHint(),
	}
}
