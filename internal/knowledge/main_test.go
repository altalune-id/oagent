package knowledge

import (
	"context"
	"os"
	"testing"
	"time"

	"altalune.id/oagent/internal/knowledge/extract"
)

// ExtractChildEnv makes the test binary act as the extract child, as `oagent extract` does in production.
const ExtractChildEnv = "OAGENT_TEST_EXTRACT_CHILD"

func TestMain(m *testing.M) {
	if os.Getenv(ExtractChildEnv) != "" {
		if err := extract.Serve(context.Background(), extract.Kind(os.Args[len(os.Args)-1]), os.Stdin, os.Stdout, extract.PDF{}); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// ChildExtractorForTests runs the test binary as the extract child.
func ChildExtractorForTests() ChildExtractor {
	return ChildExtractor{Child: extract.Child{
		Path: os.Args[0], Args: []string{"-test.run=^$"}, Env: []string{ExtractChildEnv + "=1", "GORACE=atexit_sleep_ms=0"},
		Timeout: 30 * time.Second, MaxOutput: extract.MaxChildOutput, MemoryLimit: 256 << 20,
	}}
}
