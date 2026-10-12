package user

import (
	"bufio"
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"altalune.id/oagent/internal/prompt"
	"altalune.id/oagent/password"
)

// SetPasswordFn sets a user's password and signs out their sessions; the caller boots the server graph.
type SetPasswordFn func(ctx context.Context, email, password string) error

// NewCmd builds the `user` command tree.
func NewCmd(setPassword SetPasswordFn) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "user",
		Short:   "Operator actions on users (needs database access)",
		GroupID: "runtime",
	}
	cmd.AddCommand(newSetPasswordCmd(setPassword))
	return cmd
}

func newSetPasswordCmd(setPassword SetPasswordFn) *cobra.Command {
	var email string
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "set-password",
		Short: "Set a user's password and sign out all their sessions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			in := bufio.NewReader(cmd.InOrStdin())
			label := fmt.Sprintf("New password (at least %d characters): ", password.MinLen)
			plain, err := prompt.NewSecret(cmd, in, label, "Repeat password: ", passwordStdin)
			if errors.Is(err, prompt.ErrMismatch) {
				return fmt.Errorf("set-password: %w", err)
			}
			if err != nil {
				return fmt.Errorf("set-password: read password: %w", err)
			}
			if err := setPassword(cmd.Context(), email, plain); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Password updated for %s; their sessions were signed out.\n", email)
			return nil
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "the user's email")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "read the new password as one line from stdin")
	_ = cmd.MarkFlagRequired("email")
	return cmd
}
