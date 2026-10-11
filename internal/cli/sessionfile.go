package cli

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const sessionFileVersion = 1

type sessionFile struct {
	Version  int                `json:"version"`
	Profiles map[string]profile `json:"profiles"`
}

type profile struct {
	URL       string        `json:"url"`
	Session   string        `json:"session"` // SECURITY: the signed oagent_sid value; never printed
	ExpiresAt time.Time     `json:"expiresAt"`
	Principal principalView `json:"principal"`
}

// NOTE: principalView mirrors the JSON /login returns and whoami refreshes.
type principalView struct {
	Source        string   `json:"source"`
	UserID        string   `json:"userId,omitempty"`
	Email         string   `json:"email,omitempty"`
	Name          string   `json:"name,omitempty"`
	ActiveOrgID   string   `json:"activeOrgId,omitempty"`
	ActiveOrgSlug string   `json:"activeOrgSlug,omitempty"`
	Permissions   []string `json:"permissions"`
	Operator      bool     `json:"operator"`
}

func loadSessionFile(path string) (*sessionFile, error) {
	b, err := os.ReadFile(path) //nolint:gosec // operator-chosen path (cli.sessionFile).
	if errors.Is(err, fs.ErrNotExist) {
		return &sessionFile{Version: sessionFileVersion, Profiles: map[string]profile{}}, nil
	}
	if err != nil {
		return nil, &SessionFileError{Path: path, Err: err}
	}
	var f sessionFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, &SessionFileError{Path: path, Err: err}
	}
	if f.Profiles == nil {
		f.Profiles = map[string]profile{}
	}
	return &f, nil
}

// NOTE: written to path+".tmp" then renamed, so a crash never leaves a half-written file.
func saveSessionFile(path string, f *sessionFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return &SessionFileError{Path: path, Err: err}
	}
	f.Version = sessionFileVersion
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return &SessionFileError{Path: path, Err: err}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return &SessionFileError{Path: path, Err: err}
	}
	if err := os.Rename(tmp, path); err != nil {
		return &SessionFileError{Path: path, Err: err}
	}
	return nil
}

func expandHome(path string) string {
	rest, ok := strings.CutPrefix(path, "~/")
	if !ok {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, rest)
}
