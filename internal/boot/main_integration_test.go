//go:build integration

package boot

import (
	"testing"

	"altalune.id/oagent/internal/testutil/pgtest"
	"altalune.id/oagent/internal/testutil/redistest"
)

func TestMain(m *testing.M) {
	pgtest.Main(redistest.Wrap(m))
}
