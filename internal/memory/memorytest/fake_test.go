package memorytest_test

import (
	"testing"

	"altalune.id/oagent/internal/memory/memorytest"
)

func TestFake_KeepsTheFactsContract(t *testing.T) {
	memorytest.Run(t, func(*testing.T) memorytest.Backend { return memorytest.NewFake() })
}

func TestFake_ExtractingKeepsTheFactsContract(t *testing.T) {
	memorytest.Run(t, func(*testing.T) memorytest.Backend { return memorytest.NewFake(memorytest.Extracting()) })
}
