package auth

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"

	"altalune.id/oagent/apperror"
	authv1 "altalune.id/oagent/gen/go/auth/v1"
	authv1connect "altalune.id/oagent/gen/go/auth/v1/authv1connect"
	"altalune.id/oagent/httpclient"
	"altalune.id/oagent/internal/apiclient"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/prompt"
	"altalune.id/oagent/internal/render"
)

// CLISession is a signed-in CLI profile: the server, the session value and who it acts as.
type CLISession struct {
	URL       string
	Session   string // SECURITY: the signed oagent_sid value; never printed
	ExpiresAt time.Time
	Principal CLIPrincipal
}

// CLIPrincipal is who a CLI session acts as: set by login, refreshed by whoami.
type CLIPrincipal struct {
	Source        string
	UserID        string
	Email         string
	Name          string
	ActiveOrgID   string
	ActiveOrgSlug string
	Permissions   []string
	Operator      bool
}

// ClientFn returns the Auth client for the command's credential and whether that credential is the profile's session.
type ClientFn func(cmd *cobra.Command) (client authv1connect.AuthServiceClient, fromProfile bool, err error)

// SessionClientFn returns an Auth client that presents session to url.
type SessionClientFn func(cmd *cobra.Command, url, session string) (authv1connect.AuthServiceClient, error)

// NewCmd builds the `auth` command tree.
func NewCmd(profiles CLIProfiles, clientFn ClientFn, sessionClientFn SessionClientFn) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "auth",
		Short:   "Sign in to an oagent server (login, logout, whoami)",
		GroupID: "auth",
	}
	cmd.AddCommand(
		newLoginCmd(profiles, sessionClientFn),
		newLogoutCmd(profiles),
		newWhoamiCmd(profiles, clientFn),
	)
	return cmd
}

func newLoginCmd(profiles CLIProfiles, sessionClientFn SessionClientFn) *cobra.Command {
	var email string
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Sign in with email and password and save the session in a profile",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runLogin(cmd, profiles, sessionClientFn, email, passwordStdin)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "email (prompted when empty)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "read the password as one line from stdin")
	return cmd
}

func runLogin(cmd *cobra.Command, profiles CLIProfiles, sessionClientFn SessionClientFn, email string, passwordStdin bool) error {
	name, base, err := profiles.Target(cmd)
	if err != nil {
		return err
	}
	in := bufio.NewReader(cmd.InOrStdin())
	email = strings.TrimSpace(email)
	if email == "" {
		_, _ = fmt.Fprint(cmd.ErrOrStderr(), "Email: ")
		if email, err = prompt.Line(in); err != nil {
			return fmt.Errorf("login: read email: %w", err)
		}
		email = strings.TrimSpace(email)
	}
	password, err := prompt.Secret(cmd, in, "Password: ", passwordStdin)
	if err != nil {
		return fmt.Errorf("login: read password: %w", err)
	}
	s, err := postLogin(cmd.Context(), base, email, password)
	if err != nil {
		return err
	}
	if p, werr := whoamiWithSession(cmd, sessionClientFn, base, s.Session); werr != nil {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "warn: signed in, but whoami failed:", werr)
	} else {
		s.Principal = p
	}
	if err := profiles.Save(cmd, s); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Signed in to %s as %s (profile %s)\n", base, s.Principal.Email, name)
	return nil
}

// postLogin signs in at base+"/login". SECURITY: redirects are not followed and an error body is never echoed.
func postLogin(ctx context.Context, base, email, password string) (CLISession, error) {
	form := url.Values{"email": {email}, "password": {password}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/login", strings.NewReader(form.Encode()))
	if err != nil {
		return CLISession{}, fmt.Errorf("login: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := noRedirectClient().Do(req) //nolint:gosec // G704: the operator chose the URL.
	if err != nil {
		return CLISession{}, fmt.Errorf("login: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized:
		return CLISession{}, apperror.New(apperror.CodeAuthInvalidCredentials, "invalid email or password")
	default:
		return CLISession{}, fmt.Errorf("login: %s answered %s", base, resp.Status)
	}
	var body loginResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return CLISession{}, fmt.Errorf("login: decode response: %w", err)
	}
	pr := body.Principal
	return CLISession{
		URL:       base,
		Session:   body.Session,
		ExpiresAt: body.ExpiresAt,
		Principal: CLIPrincipal{
			Source: pr.Source, UserID: pr.UserID, Email: pr.Email, Name: pr.Name,
			ActiveOrgID: pr.ActiveOrgID, Permissions: pr.Permissions, Operator: pr.Operator,
		},
	}, nil
}

func noRedirectClient() *http.Client {
	c := httpclient.New(httpclient.WithAllowPrivateHosts(true), httpclient.WithTimeout(apiclient.DefaultTimeout))
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return c
}

func newLogoutCmd(profiles CLIProfiles) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "End the profile's session on the server and remove the profile",
		RunE: func(cmd *cobra.Command, _ []string) error {
			name, s, err := profiles.Current(cmd)
			if err != nil {
				return err
			}
			if s == nil {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "not signed in (profile %s)\n", name)
				return nil
			}
			if err := postLogout(cmd.Context(), *s); err != nil {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "warn: the server session was not ended:", err)
			}
			if err := profiles.Remove(cmd); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Signed out of %s (profile %s)\n", s.URL, name)
			return nil
		},
	}
}

// SECURITY: the cookie goes only to the profile's own URL.
func postLogout(ctx context.Context, s CLISession) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(s.URL, "/")+"/logout", http.NoBody)
	if err != nil {
		return err
	}
	req.Header.Set("Cookie", session.CookieName+"="+s.Session)
	resp, err := noRedirectClient().Do(req) //nolint:gosec // G704: the profile's own URL.
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

func newWhoamiCmd(profiles CLIProfiles, clientFn ClientFn) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show who the current credential acts as",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, fromProfile, err := clientFn(cmd)
			if err != nil {
				return err
			}
			resp, err := client.Whoami(cmd.Context(), connect.NewRequest(&authv1.WhoamiRequest{}))
			if err != nil {
				return err
			}
			if fromProfile {
				if err := refreshProfile(cmd, profiles, resp.Msg); err != nil {
					return err
				}
			}
			return printWhoami(cmd, resp.Msg)
		},
	}
}

func refreshProfile(cmd *cobra.Command, profiles CLIProfiles, w *authv1.WhoamiResponse) error {
	_, s, err := profiles.Current(cmd)
	if err != nil || s == nil {
		return err
	}
	s.Principal = principalOfWhoami(w)
	return profiles.Save(cmd, *s)
}

func whoamiWithSession(cmd *cobra.Command, sessionClientFn SessionClientFn, base, sid string) (CLIPrincipal, error) {
	client, err := sessionClientFn(cmd, base, sid)
	if err != nil {
		return CLIPrincipal{}, err
	}
	resp, err := client.Whoami(cmd.Context(), connect.NewRequest(&authv1.WhoamiRequest{}))
	if err != nil {
		return CLIPrincipal{}, err
	}
	return principalOfWhoami(resp.Msg), nil
}

func principalOfWhoami(w *authv1.WhoamiResponse) CLIPrincipal {
	return CLIPrincipal{
		Source: w.GetSource(), UserID: w.GetUserId(), Email: w.GetEmail(), Name: w.GetName(),
		ActiveOrgID: w.GetActiveOrgId(), ActiveOrgSlug: w.GetActiveOrgSlug(),
		Permissions: w.GetPermissions(), Operator: w.GetOperator(),
	}
}

func printWhoami(cmd *cobra.Command, w *authv1.WhoamiResponse) error {
	if f := render.Detect(cmd); f == render.FormatJSON || f == render.FormatNDJSON {
		return render.JSON(cmd.OutOrStdout(), map[string]any{
			"source":          w.GetSource(),
			"user_id":         w.GetUserId(),
			"email":           w.GetEmail(),
			"name":            w.GetName(),
			"key_hint":        w.GetKeyHint(),
			"owner_id":        w.GetOwnerId(),
			"active_org_id":   w.GetActiveOrgId(),
			"active_org_slug": w.GetActiveOrgSlug(),
			"permissions":     w.GetPermissions(),
			"operator":        w.GetOperator(),
		})
	}
	rows := [][]string{{"source", w.GetSource()}}
	if w.GetKeyHint() != "" {
		rows = append(rows, []string{"key", "…" + w.GetKeyHint()})
	}
	if w.GetEmail() != "" {
		rows = append(rows, []string{"email", w.GetEmail()})
	}
	if w.GetName() != "" {
		rows = append(rows, []string{"name", w.GetName()})
	}
	rows = append(rows,
		[]string{"org", w.GetActiveOrgSlug()},
		[]string{"permissions", strings.Join(w.GetPermissions(), ", ")},
		[]string{"operator", fmt.Sprint(w.GetOperator())},
	)
	for _, row := range rows {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%-12s %s\n", row[0]+":", row[1])
	}
	return nil
}
