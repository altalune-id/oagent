package password

import (
	_ "embed"
	"strings"
	"sync"
)

//go:embed common.txt
var commonRaw string

//nolint:gochecknoglobals // computed once, read-only afterwards.
var commonSet = sync.OnceValue(func() map[string]struct{} {
	set := map[string]struct{}{}
	for line := range strings.Lines(commonRaw) {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			set[line] = struct{}{}
		}
	}
	return set
})
