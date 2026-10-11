package cli

import (
	"errors"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"altalune.id/oagent/internal/apiclient"
	"altalune.id/oagent/internal/auth"
)

const defaultProfile = "default"

// NOTE: precedence is --profile > OAGENT_PROFILE > "default".
func profileName(cmd *cobra.Command) string {
	if f := cmd.Root().PersistentFlags().Lookup("profile"); f != nil && f.Changed {
		if v := strings.TrimSpace(f.Value.String()); v != "" {
			return v
		}
	}
	if v := strings.TrimSpace(os.Getenv("OAGENT_PROFILE")); v != "" {
		return v
	}
	return defaultProfile
}

// SECURITY: precedence is --token > --token-file > OAGENT_TOKEN > the profile's session, sent only to its own URL while valid.
func resolveCredential(cmd *cobra.Command, url string, p *profile, now time.Time) (apiclient.Credential, error) {
	token, err := resolveToken(cmd)
	if err != nil {
		return apiclient.Credential{}, err
	}
	if token != "" {
		return apiclient.Credential{Token: token}, nil
	}
	if p == nil || p.Session == "" {
		return apiclient.Credential{}, nil
	}
	name := profileName(cmd)
	if !sameURL(p.URL, url) {
		return apiclient.Credential{}, &ProfileHostMismatchError{Profile: name, ProfileURL: p.URL, URL: url}
	}
	if !now.Before(p.ExpiresAt) {
		return apiclient.Credential{}, &SessionExpiredError{Profile: name}
	}
	return apiclient.Credential{Session: p.Session}, nil
}

func sameURL(a, b string) bool {
	return strings.TrimRight(a, "/") == strings.TrimRight(b, "/")
}

type remote struct {
	path    string
	file    *sessionFile
	name    string
	profile *profile
	url     string
	cred    apiclient.Credential
}

func (r remote) fromProfile() bool { return r.cred.Session != "" }

func resolveRemote(cmd *cobra.Command) (remote, error) {
	r, err := loadProfile(cmd)
	if err != nil {
		return remote{}, err
	}
	r.url = resolveURL(cmd, r.profile)
	r.cred, err = resolveCredential(cmd, r.url, r.profile, time.Now())
	return r, err
}

func loadProfile(cmd *cobra.Command) (remote, error) {
	cfg := configFromCtx(cmd.Context())
	if cfg == nil || cfg.CLI.SessionFile == "" {
		return remote{}, errors.New("cli: cli.sessionFile is not set")
	}
	r := remote{path: expandHome(cfg.CLI.SessionFile), name: profileName(cmd)}
	f, err := loadSessionFile(r.path)
	if err != nil {
		return remote{}, err
	}
	r.file = f
	if p, ok := f.Profiles[r.name]; ok {
		r.profile = &p
	}
	return r, nil
}

func (r remote) save(p profile) error {
	r.file.Profiles[r.name] = p
	return saveSessionFile(r.path, r.file)
}

type sessionProfiles struct{}

var _ auth.CLIProfiles = sessionProfiles{}

func (sessionProfiles) Target(cmd *cobra.Command) (name, url string, err error) {
	r, err := loadProfile(cmd)
	if err != nil {
		return "", "", err
	}
	return r.name, resolveURL(cmd, r.profile), nil
}

func (sessionProfiles) Current(cmd *cobra.Command) (string, *auth.CLISession, error) {
	r, err := loadProfile(cmd)
	if err != nil || r.profile == nil {
		return r.name, nil, err
	}
	s := r.profile.session()
	return r.name, &s, nil
}

func (sessionProfiles) Save(cmd *cobra.Command, s auth.CLISession) error {
	r, err := loadProfile(cmd)
	if err != nil {
		return err
	}
	return r.save(profileOf(s))
}

func (sessionProfiles) Remove(cmd *cobra.Command) error {
	r, err := loadProfile(cmd)
	if err != nil {
		return err
	}
	delete(r.file.Profiles, r.name)
	return saveSessionFile(r.path, r.file)
}

func (p profile) session() auth.CLISession {
	v := p.Principal
	return auth.CLISession{
		URL: p.URL, Session: p.Session, ExpiresAt: p.ExpiresAt,
		Principal: auth.CLIPrincipal{
			Source: v.Source, UserID: v.UserID, Email: v.Email, Name: v.Name,
			ActiveOrgID: v.ActiveOrgID, ActiveOrgSlug: v.ActiveOrgSlug, Permissions: v.Permissions, Operator: v.Operator,
		},
	}
}

func profileOf(s auth.CLISession) profile {
	v := s.Principal
	return profile{
		URL: s.URL, Session: s.Session, ExpiresAt: s.ExpiresAt,
		Principal: principalView{
			Source: v.Source, UserID: v.UserID, Email: v.Email, Name: v.Name,
			ActiveOrgID: v.ActiveOrgID, ActiveOrgSlug: v.ActiveOrgSlug, Permissions: v.Permissions, Operator: v.Operator,
		},
	}
}
