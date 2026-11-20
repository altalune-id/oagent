//go:build integration

package snapshotstore_test

import (
	"testing"

	"altalune.id/oagent/internal/testutil/pgtest"
)

func TestMain(m *testing.M) {
	pgtest.Main(m)
}
