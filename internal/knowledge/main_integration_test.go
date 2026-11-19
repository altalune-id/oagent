//go:build integration

package knowledge

import (
	"testing"

	"altalune.id/oagent/internal/testutil/pgtest"
)

func runTests(m *testing.M) {
	pgtest.Main(m)
}
