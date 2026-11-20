//go:build integration

package redis_test

import (
	"testing"

	"altalune.id/oagent/internal/testutil/redistest"
)

func TestMain(m *testing.M) {
	redistest.Main(m)
}
