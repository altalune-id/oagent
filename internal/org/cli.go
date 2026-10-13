package org

import (
	"connectrpc.com/connect"
	"github.com/spf13/cobra"

	orgv1 "altalune.id/oagent/gen/go/org/v1"
	orgv1connect "altalune.id/oagent/gen/go/org/v1/orgv1connect"
	"altalune.id/oagent/internal/render"
)

// ClientFn returns the Org Connect client for a command; the caller resolves URL and token.
type ClientFn func(cmd *cobra.Command) (orgv1connect.OrgServiceClient, error)

// NewCmd builds the `org` command tree.
func NewCmd(clientFn ClientFn) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "org",
		Short:   "Manage organizations",
		GroupID: "tenancy",
	}
	cmd.AddCommand(
		newCreateCmd(clientFn),
		newGetCmd(clientFn),
		newListCmd(clientFn),
	)
	return cmd
}

func newCreateCmd(clientFn ClientFn) *cobra.Command {
	var slug, name string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an organization",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			resp, err := client.CreateOrg(cmd.Context(), connect.NewRequest(&orgv1.CreateOrgRequest{Slug: slug, Name: name}))
			if err != nil {
				return err
			}
			o := resp.Msg.GetOrg()
			if isJSON(cmd) {
				return render.JSON(cmd.OutOrStdout(), orgMap(o))
			}
			cmd.Printf("Created org %s (%s)\n", o.GetSlug(), o.GetId())
			return nil
		},
	}
	cmd.Flags().StringVar(&slug, "slug", "", "URL-safe org slug (generated when omitted)")
	cmd.Flags().StringVar(&name, "name", "", "Organization name")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func newGetCmd(clientFn ClientFn) *cobra.Command {
	return &cobra.Command{
		Use:   "get <public-id>",
		Short: "Get an organization by id",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			resp, err := client.GetOrg(cmd.Context(), connect.NewRequest(&orgv1.GetOrgRequest{Id: args[0]}))
			if err != nil {
				return err
			}
			o := resp.Msg.GetOrg()
			if isJSON(cmd) {
				return render.JSON(cmd.OutOrStdout(), orgMap(o))
			}
			return render.Table(cmd.OutOrStdout(), []string{"ID", "SLUG", "NAME"},
				[][]string{{o.GetId(), o.GetSlug(), o.GetName()}})
		},
	}
}

func newListCmd(clientFn ClientFn) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List organizations",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			resp, err := client.ListOrgs(cmd.Context(), connect.NewRequest(&orgv1.ListOrgsRequest{}))
			if err != nil {
				return err
			}
			orgs := resp.Msg.GetOrgs()
			if isJSON(cmd) {
				out := make([]map[string]any, 0, len(orgs))
				for _, o := range orgs {
					out = append(out, orgMap(o))
				}
				return render.JSON(cmd.OutOrStdout(), out)
			}
			rows := make([][]string, 0, len(orgs))
			for _, o := range orgs {
				rows = append(rows, []string{o.GetId(), o.GetSlug(), o.GetName()})
			}
			return render.Table(cmd.OutOrStdout(), []string{"ID", "SLUG", "NAME"}, rows)
		},
	}
}

func isJSON(cmd *cobra.Command) bool {
	switch render.Detect(cmd) {
	case render.FormatJSON, render.FormatNDJSON:
		return true
	default:
		return false
	}
}

func orgMap(o *orgv1.Org) map[string]any {
	return map[string]any{
		"id":   o.GetId(),
		"slug": o.GetSlug(),
		"name": o.GetName(),
	}
}
