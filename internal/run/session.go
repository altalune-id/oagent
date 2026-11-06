package run

import (
	"strings"

	"github.com/google/uuid"
)

// SessionID is the id outside stores know a thread by: "t" plus its 32 lowercase hex digits.
func SessionID(threadID uuid.UUID) string { return "t" + hex32(threadID) }

// AgentKey is the id outside stores know an agent by: "a" plus its 32 lowercase hex digits.
func AgentKey(agentID uuid.UUID) string { return "a" + hex32(agentID) }

func hex32(id uuid.UUID) string { return strings.ReplaceAll(id.String(), "-", "") }
