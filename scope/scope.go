// Package scope is the single catalog of API scopes, read from scopes.json, which the console shares verbatim.
package scope

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

//go:embed scopes.json
var scopesJSON []byte

// Level names where a scope's authority lives: inside one project, or across the org itself.
type Level string

// The scope levels. An org-level scope acts on the org (its projects, its keys), never on one project's data.
const (
	LevelOrg     Level = "org"
	LevelProject Level = "project"
)

// Holder names who may hold a scope, from the widest to the narrowest.
type Holder string

// The holders. SECURITY: a key holds only HolderAny, a token also HolderPerson, and only a web session HolderConsole.
const (
	HolderAny     Holder = ""
	HolderPerson  Holder = "person"
	HolderConsole Holder = "console"
)

func (h Holder) rank() int {
	switch h {
	case HolderAny:
		return 0
	case HolderPerson:
		return 1
	case HolderConsole:
		return 2
	}
	return -1
}

// Spec describes one scope.
type Spec struct {
	Name   string `json:"name"`
	Level  Level  `json:"level"`
	Holder Holder `json:"holder,omitempty"`
}

//nolint:gochecknoglobals // immutable catalog parsed once from the embedded file.
var catalog = mustParse(scopesJSON)

func mustParse(b []byte) []Spec {
	out, err := parse(b)
	if err != nil {
		panic("scope: invalid scopes.json: " + err.Error())
	}
	return out
}

func parse(b []byte) ([]Spec, error) {
	var out []Spec
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	for _, s := range out {
		if s.Holder.rank() < 0 {
			return nil, fmt.Errorf("scope %s: unknown holder %q", s.Name, s.Holder)
		}
	}
	return out, nil
}

// All returns the catalog.
func All() []Spec { return slices.Clone(catalog) }

// Names returns every scope name in catalog order.
func Names() []string {
	out := make([]string, 0, len(catalog))
	for _, s := range catalog {
		out = append(out, s.Name)
	}
	return out
}

// Valid reports whether name is in the catalog.
func Valid(name string) bool {
	return slices.ContainsFunc(catalog, func(s Spec) bool { return s.Name == name })
}

// LevelOf returns name's level, or false for a scope outside the catalog.
func LevelOf(name string) (Level, bool) {
	i := slices.IndexFunc(catalog, func(s Spec) bool { return s.Name == name })
	if i < 0 {
		return "", false
	}
	return catalog[i].Level, true
}

// Holds reports whether a caller of kind by may hold name; a name outside the catalog is held by nobody.
func Holds(name string, by Holder) bool {
	i := slices.IndexFunc(catalog, func(s Spec) bool { return s.Name == name })
	return i >= 0 && by.rank() >= catalog[i].Holder.rank()
}

// Grants reports whether granted covers required, honoring "*" and "<resource>:*" wildcards.
func Grants(granted []string, required string) bool {
	if required == "" {
		return false
	}
	resource, _, _ := strings.Cut(required, ":")
	for _, g := range granted {
		if g == "*" || g == required || g == resource+":*" {
			return true
		}
	}
	return false
}

// Covers reports whether have grants every catalog permission want grants, so wildcards compare by what they cover.
func Covers(have, want []string) bool {
	for _, s := range catalog {
		if Grants(want, s.Name) && !Grants(have, s.Name) {
			return false
		}
	}
	return true
}
