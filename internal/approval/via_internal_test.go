package approval

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/internal/kernel/session"
)

func TestViaOf_TheCallerDecidesOverConnect(t *testing.T) {
	person := session.PrincipalInto(context.Background(), session.Principal{Source: session.SourceLocal, UserID: uuid.New()})
	key := session.PrincipalInto(context.Background(), session.Principal{Source: session.SourceAPIKey, KeyID: uuid.New()})
	for name, tc := range map[string]struct {
		ctx      context.Context
		protocol string
		want     Via
	}{
		"a person over Connect":     {person, "connect", ViaConsole},
		"a key over Connect":        {key, "connect", ViaAPI},
		"the MCP tool, any caller":  {key, "", ViaMCP},
		"the MCP tool for a person": {person, "", ViaMCP},
	} {
		assert.Equal(t, tc.want, viaOf(tc.ctx, tc.protocol), name)
	}
}
