package layout

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPath(t *testing.T) {
	cases := []struct {
		name     string
		basePath string
		sub      string
		want     string
	}{
		{"empty base and sub", "", "", "/"},
		{"base only, empty sub", "/app", "", "/app"},
		{"empty base, plain sub", "", "login", "/login"},
		{"empty base, slashed sub", "", "/login", "/login"},
		{"base, plain sub", "/app", "login", "/app/login"},
		{"base, slashed sub", "/app/", "/login", "/app/login"},
		{"base with trailing slash", "/app/", "", "/app"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, Path(c.basePath, c.sub))
		})
	}
}
