package project

import (
	"connectrpc.com/connect"
	"github.com/spf13/cobra"

	projectv1 "altalune.id/oagent/gen/go/project/v1"
	projectv1connect "altalune.id/oagent/gen/go/project/v1/projectv1connect"
	"altalune.id/oagent/internal/render"
)

// ClientFn returns the Project Connect client for a command; the caller resolves URL and token.
type ClientFn func(cmd *cobra.Command) (projectv1connect.ProjectServiceClient, error)

// NewCmd builds the `project` command tree.
func NewCmd(clientFn ClientFn) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "project",
		Short:   "Manage projects",
		GroupID: "domain",
	}
	cmd.AddCommand(
		newCreateCmd(clientFn),
		newGetCmd(clientFn),
		newListCmd(clientFn),
	)
	return cmd
}

func newCreateCmd(clientFn ClientFn) *cobra.Command {
	var orgID, slug, name string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a project",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			resp, err := client.CreateProject(
				cmd.Context(),
				connect.NewRequest(&projectv1.CreateProjectRequest{OrgId: orgID, Slug: slug, Name: name}),
			)
			if err != nil {
				return err
			}
			p := resp.Msg.GetProject()
			if isJSON(cmd) {
				return render.JSON(cmd.OutOrStdout(), projectMap(p))
			}
			cmd.Printf("Created project %s (%s)\n", p.GetSlug(), p.GetId())
			return nil
		},
	}
	cmd.Flags().StringVar(&orgID, "org-id", "", "Organization public id")
	cmd.Flags().StringVar(&slug, "slug", "", "URL-safe project slug (generated when omitted)")
	cmd.Flags().StringVar(&name, "name", "", "Project name")
	_ = cmd.MarkFlagRequired("org-id")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func newGetCmd(clientFn ClientFn) *cobra.Command {
	return &cobra.Command{
		Use:   "get <public-id>",
		Short: "Get a project by id",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			resp, err := client.GetProject(cmd.Context(), connect.NewRequest(&projectv1.GetProjectRequest{Id: args[0]}))
			if err != nil {
				return err
			}
			p := resp.Msg.GetProject()
			if isJSON(cmd) {
				return render.JSON(cmd.OutOrStdout(), projectMap(p))
			}
			return render.Table(cmd.OutOrStdout(), []string{"ID", "ORG_ID", "SLUG", "NAME"},
				[][]string{{p.GetId(), p.GetOrgId(), p.GetSlug(), p.GetName()}})
		},
	}
}

func newListCmd(clientFn ClientFn) *cobra.Command {
	var orgID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List projects in an organization",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			resp, err := client.ListProjects(cmd.Context(), connect.NewRequest(&projectv1.ListProjectsRequest{OrgId: orgID}))
			if err != nil {
				return err
			}
			projects := resp.Msg.GetProjects()
			if isJSON(cmd) {
				out := make([]map[string]any, 0, len(projects))
				for _, p := range projects {
					out = append(out, projectMap(p))
				}
				return render.JSON(cmd.OutOrStdout(), out)
			}
			rows := make([][]string, 0, len(projects))
			for _, p := range projects {
				rows = append(rows, []string{p.GetId(), p.GetOrgId(), p.GetSlug(), p.GetName()})
			}
			return render.Table(cmd.OutOrStdout(), []string{"ID", "ORG_ID", "SLUG", "NAME"}, rows)
		},
	}
	cmd.Flags().StringVar(&orgID, "org-id", "", "Organization public id")
	_ = cmd.MarkFlagRequired("org-id")
	return cmd
}

func isJSON(cmd *cobra.Command) bool {
	switch render.Detect(cmd) {
	case render.FormatJSON, render.FormatNDJSON:
		return true
	default:
		return false
	}
}

func projectMap(p *projectv1.Project) map[string]any {
	return map[string]any{
		"id":    p.GetId(),
		"orgId": p.GetOrgId(),
		"slug":  p.GetSlug(),
		"name":  p.GetName(),
	}
}
