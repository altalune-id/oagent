package channel

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInboxMessage_KeepsTheQuotedID(t *testing.T) {
	in := Inbound{ExternalID: "wamid-2", Type: "text", Text: "ya", QuotedID: "wamid-prompt", Time: time.Now()}
	m, err := inboxMessage(in, in.Text, uuid.New())
	require.NoError(t, err)
	assert.Equal(t, "wamid-prompt", m.QuotedID(), "the group decision rule reads the quoted WhatsApp id")

	in.QuotedID = ""
	m, err = inboxMessage(in, in.Text, uuid.New())
	require.NoError(t, err)
	assert.Empty(t, m.QuotedID())
	assert.NotContains(t, string(m.Body), "quoted")
}
