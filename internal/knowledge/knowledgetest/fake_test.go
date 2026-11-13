package knowledgetest_test

import (
	"testing"

	"altalune.id/oagent/internal/knowledge/knowledgetest"
)

func TestFake_KeepsTheRetrieverContract(t *testing.T) {
	knowledgetest.Run(t, func(*testing.T) knowledgetest.Backend { return knowledgetest.NewFake(768) })
}
