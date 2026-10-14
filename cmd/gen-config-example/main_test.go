package main

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/config"
)

func TestGenerated_MatchesConfig(t *testing.T) {
	for format, path := range map[string]string{
		formatEnv:  "../../.env.reference",
		formatYAML: "../../config.reference.yml",
	} {
		t.Run(format, func(t *testing.T) {
			got, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, generate(format), string(got), "%s is stale; run make config-examples", filepath.Base(path))
		})
	}
}

func TestGenerate_SplitsBootstrapFromRuntime(t *testing.T) {
	out := generate(formatEnv)
	bootstrap, runtime, ok := strings.Cut(out, "RUNTIME — safe to change via env + restart\n# ═")
	require.True(t, ok)

	assert.Contains(t, bootstrap, "# [bootstrap] postgres.schema\nOAGENT_POSTGRES_SCHEMA=")
	assert.NotContains(t, runtime, "OAGENT_POSTGRES_SCHEMA=")
	assert.Contains(t, runtime, "\n# ─── cli ───\n")
	assert.Contains(t, runtime, "OAGENT_CLI_SESSION_FILE=~/.oagent/session.json\n")
}

func TestGenerate_OmitsSecretValues(t *testing.T) {
	var secret config.EnvKey
	for _, k := range config.WalkEnvKeys(config.EnvPrefix) {
		if hasMarker(k.Awareness, "secret") {
			secret = k
			break
		}
	}
	require.NotEmpty(t, secret.Key, "the config has at least one secret field")

	assert.Contains(t, generate(formatEnv), "\n"+secret.Key+"=\n")
	assert.Contains(t, generate(formatYAML), "secret] env: "+secret.Key+"\n")
}

func TestWriteEnvSection(t *testing.T) {
	var b strings.Builder
	writeEnvSection(&b, []config.EnvKey{
		{Key: "X_ZED_PORT", YAML: "zed.port", Default: 8080},
		{Key: "X_ALPHA", YAML: "alpha"},
		{Key: "X_ZED_TOKEN", YAML: "zed.token", Awareness: []string{"required", "secret"}, Default: "hunter2"},
	})
	assert.Equal(t, `
# ─── alpha ───
# alpha
X_ALPHA=

# ─── zed ───
# zed.port
X_ZED_PORT=8080
# [required, secret] zed.token
X_ZED_TOKEN=
`, b.String())
}

func TestWriteYAMLTree(t *testing.T) {
	var b strings.Builder
	writeYAMLTree(&b, []config.EnvKey{
		{Key: "X_SERVER_ADDR", YAML: "server.addr", Default: ":8080"},
		{Key: "X_SERVER", YAML: "server", Default: "on"},
		{Key: "X_SERVER_KEY", YAML: "server.key", Awareness: []string{"secret"}, Default: "hunter2"},
		{Key: "X_APP_NAME", YAML: "app.name", Awareness: []string{"bootstrap"}, Default: "oagent"},
	})
	assert.Equal(t, `
app:
  # [bootstrap] env: X_APP_NAME
  name: oagent

server:
  # env: X_SERVER
  value: on
  # env: X_SERVER_ADDR
  addr: ":8080"
  # [secret] env: X_SERVER_KEY
  key: ""
`, b.String())
}

func TestYAMLScalar(t *testing.T) {
	for in, want := range map[string]string{
		"":         `""`,
		"plain":    "plain",
		"a:b":      `"a:b"`,
		"a#b":      `"a#b"`,
		"a\nb":     `"a\nb"`,
		"-1":       `"-1"`,
		"&anchor":  `"&anchor"`,
		"*alias":   `"*alias"`,
		"30s":      "30s",
		"in-place": "in-place",
	} {
		assert.Equal(t, want, yamlScalar(in), "yamlScalar(%q)", in)
	}
}

func TestNormalizeDefault(t *testing.T) {
	t.Setenv("HOME", "/home/tester")
	assert.Empty(t, normalizeDefault(""))
	assert.Equal(t, "~/.oagent/session.json", normalizeDefault("/home/tester/.oagent/session.json"))
	assert.Equal(t, "/var/lib/oagent", normalizeDefault("/var/lib/oagent"))

	t.Setenv("HOME", "")
	assert.Equal(t, "/home/tester/x", normalizeDefault("/home/tester/x"))
}

func TestTopSegment(t *testing.T) {
	assert.Equal(t, "postgres", topSegment("postgres.schema"))
	assert.Equal(t, "env", topSegment("env"))
}

func runMain(t *testing.T, args ...string) (stdout, stderr string) {
	t.Helper()
	oldArgs, oldFlags, oldOut, oldErr := os.Args, flag.CommandLine, os.Stdout, os.Stderr
	t.Cleanup(func() { os.Args, flag.CommandLine, os.Stdout, os.Stderr = oldArgs, oldFlags, oldOut, oldErr })

	dir := t.TempDir()
	outFile, err := os.Create(filepath.Join(dir, "stdout"))
	require.NoError(t, err)
	errFile, err := os.Create(filepath.Join(dir, "stderr"))
	require.NoError(t, err)

	os.Args = append([]string{"gen-config-example"}, args...)
	flag.CommandLine = flag.NewFlagSet("gen-config-example", flag.ContinueOnError)
	os.Stdout, os.Stderr = outFile, errFile
	main()
	os.Stdout, os.Stderr = oldOut, oldErr
	require.NoError(t, outFile.Close())
	require.NoError(t, errFile.Close())

	o, err := os.ReadFile(outFile.Name())
	require.NoError(t, err)
	e, err := os.ReadFile(errFile.Name())
	require.NoError(t, err)
	return string(o), string(e)
}

func TestMain_WritesTheDefaultTargets(t *testing.T) {
	t.Chdir(t.TempDir())

	_, stderr := runMain(t)
	assert.Contains(t, stderr, "wrote .env.reference")
	got, err := os.ReadFile(".env.reference")
	require.NoError(t, err)
	assert.Equal(t, generate(formatEnv), string(got))

	_, stderr = runMain(t, "--format=yaml")
	assert.Contains(t, stderr, "wrote config.reference.yml")
	got, err = os.ReadFile("config.reference.yml")
	require.NoError(t, err)
	assert.Equal(t, generate(formatYAML), string(got))

	_, stderr = runMain(t, "--format=yaml", "--check")
	assert.Contains(t, stderr, "config.reference.yml is up-to-date")
}

func TestMain_WritesStdout(t *testing.T) {
	t.Chdir(t.TempDir())

	stdout, _ := runMain(t, "--format=yaml", "--output=-")
	assert.Equal(t, generate(formatYAML), stdout)

	entries, err := os.ReadDir(".")
	require.NoError(t, err)
	assert.Empty(t, entries, "stdout output writes no file")
}

func TestMain_WritesTheNamedOutput(t *testing.T) {
	t.Chdir(t.TempDir())

	_, stderr := runMain(t, "--output=custom.env")
	assert.Contains(t, stderr, "wrote custom.env")
	got, err := os.ReadFile("custom.env")
	require.NoError(t, err)
	assert.Equal(t, generate(formatEnv), string(got))
}

func TestMain_ExitCodes(t *testing.T) {
	if args := os.Getenv("GEN_CONFIG_EXAMPLE_ARGS"); args != "" {
		os.Args = append([]string{"gen-config-example"}, strings.Fields(args)...)
		main()
		return
	}

	dir := t.TempDir()
	stale := filepath.Join(dir, "stale.env")
	require.NoError(t, os.WriteFile(stale, []byte("OLD=1\n"), 0o600))

	for name, tc := range map[string]struct {
		args   string
		code   int
		stderr string
	}{
		"unknown format":  {"--format=toml", 2, `unknown format "toml"`},
		"check missing":   {"--check --output=" + filepath.Join(dir, "missing.env"), 2, "reading"},
		"check stale":     {"--check --output=" + stale, 1, "stale.env is stale"},
		"unwritable path": {"--output=" + filepath.Join(dir, "no", "such", "dir.env"), 1, "no such file or directory"},
	} {
		t.Run(name, func(t *testing.T) {
			cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestMain_ExitCodes$")
			cmd.Env = append(os.Environ(), "GEN_CONFIG_EXAMPLE_ARGS="+tc.args)
			var stderr strings.Builder
			cmd.Stderr = &stderr
			err := cmd.Run()
			var exitErr *exec.ExitError
			require.ErrorAs(t, err, &exitErr)
			assert.Equal(t, tc.code, exitErr.ExitCode())
			assert.Contains(t, stderr.String(), tc.stderr)
		})
	}
}
