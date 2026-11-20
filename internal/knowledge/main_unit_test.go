//go:build !integration

package knowledge

import (
	"os"
	"testing"
)

func runTests(m *testing.M) {
	os.Exit(m.Run())
}
