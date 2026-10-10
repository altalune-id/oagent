package session_test

import (
	"testing"

	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/kernel/session/sessiontest"
)

func TestMemoryStore_Contract(t *testing.T) {
	sessiontest.RunStoreContract(t, func(*testing.T) session.Store { return session.NewMemoryStore() })
}
