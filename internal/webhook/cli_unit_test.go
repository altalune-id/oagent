package webhook

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	webhookv1 "altalune.id/oagent/gen/go/webhook/v1"
)

func TestWebhookColumnsAndRow(t *testing.T) {
	assert.Len(t, webhookColumns(), 5)
	e := &webhookv1.Webhook{Id: "w", ProjectId: "p", Url: "u", Active: true, EventTypes: []string{"a", "b"}}
	assert.Equal(t, []string{"w", "p", "u", "true", "a,b"}, webhookRow(e))
	assert.Equal(t, "false", boolText(false))
}

func TestIsJSON(t *testing.T) {
	t.Setenv("OAGENT_OUTPUT", "text")
	assert.False(t, isJSON(&cobra.Command{}))
	t.Setenv("OAGENT_OUTPUT", "json")
	assert.True(t, isJSON(&cobra.Command{}))
}
