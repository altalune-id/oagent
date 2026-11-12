package crawl_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/knowledge/crawl"
)

type robotsVector struct {
	Name       string `json:"name"`
	Robots     string `json:"robots"`
	PadComment int    `json:"padComment"`
	Status     int    `json:"status"`
	Error      string `json:"error"`
	Path       string `json:"path"`
	Allowed    bool   `json:"allowed"`
}

type robotsFile struct {
	UserAgent string         `json:"userAgent"`
	MaxBytes  int            `json:"maxBytes"`
	Vectors   []robotsVector `json:"vectors"`
}

func loadRobotsVectors(t *testing.T) robotsFile {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "testdata", "robots_vectors.json"))
	require.NoError(t, err)
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var f robotsFile
	require.NoError(t, dec.Decode(&f))
	return f
}

func body(v robotsVector) []byte {
	if v.PadComment == 0 {
		return []byte(v.Robots)
	}
	first, rest, _ := strings.Cut(v.Robots, "\n")
	return []byte(first + "\n#" + strings.Repeat("x", v.PadComment) + "\n" + rest)
}

func TestRobots_SharedVectors(t *testing.T) {
	f := loadRobotsVectors(t)
	assert.Equal(t, crawl.RobotsAgent, f.UserAgent)
	assert.Equal(t, crawl.MaxRobotsBytes, f.MaxBytes)
	require.NotEmpty(t, f.Vectors)
	for _, v := range f.Vectors {
		t.Run(v.Name, func(t *testing.T) {
			var fetchErr error
			if v.Error != "" {
				fetchErr = errors.New(v.Error)
			}
			rules := crawl.RulesFromFetch(v.Status, body(v), fetchErr)
			assert.Equal(t, v.Allowed, rules.Allows(v.Path))
		})
	}
}

func TestRobots_RefusesUnknownVectorFields(t *testing.T) {
	dec := json.NewDecoder(strings.NewReader(`{"vectors":[{"name":"x","path":"/","allowed":true,"extra":1}]}`))
	dec.DisallowUnknownFields()
	assert.Error(t, dec.Decode(&robotsFile{}))
}

func TestRobots_PatternsAreLinear(t *testing.T) {
	rules := crawl.ParseRobots("User-agent: *\nDisallow: /"+strings.Repeat("*a", 500)+"$\n", crawl.RobotsAgent)
	assert.True(t, rules.Allows("/"+strings.Repeat("a", 20_000)+"b"))
}

func TestRobots_HugePatternNeverPanics(t *testing.T) {
	huge := "/" + strings.Repeat("a*", 200_000)
	assert.NotPanics(t, func() {
		rules := crawl.ParseRobots("User-agent: *\nDisallow: "+huge+"\nDisallow: /private\n", crawl.RobotsAgent)
		assert.False(t, rules.Allows("/private/x"))
	})
}
