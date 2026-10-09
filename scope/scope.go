// Package scope is the single catalog of API scopes and the level each is authoritative at.
// The catalog data lives in scopes.json, shared verbatim with the console.
package scope

import (
	_ "embed"
	"encoding/json"
	"slices"
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
	Name  string `json:"name"`
	Level Level  `json:"level"`
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
