// Package scope is the single catalog of API scopes, read from scopes.json, which the console shares verbatim.
package scope

import (
	_ "embed"
	"encoding/json"
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

// Spec describes one scope.
type Spec struct {
	Name       string `json:"name"`
	Level      Level  `json:"level"`
	PersonOnly bool   `json:"personOnly,omitempty"`
}

//nolint:gochecknoglobals // immutable catalog parsed once from the embedded file.
var catalog = mustParse(scopesJSON)

func mustParse(b []byte) []Spec {
	var out []Spec
	if err := json.Unmarshal(b, &out); err != nil {
		panic("scope: invalid scopes.json: " + err.Error())
	}
	return out
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

// Mintable reports whether an API key may hold name. SECURITY: person-only scopes never go on a key.
func Mintable(name string) bool {
	i := slices.IndexFunc(catalog, func(s Spec) bool { return s.Name == name })
	return i >= 0 && !catalog[i].PersonOnly
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
