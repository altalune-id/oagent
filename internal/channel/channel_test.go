package channel_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/channel"
)

func input() channel.Input {
	return channel.Input{BaseURL: "https://wa.example.com", OpenWAOrg: "acme", OpenWAProject: "main", DeviceID: "dev_V1StGXR8Z5jdHi6B"}
}

func TestNew_Defaults(t *testing.T) {
	c, err := channel.New(uuid.New(), uuid.New(), uuid.New(), input(), time.Now())
	require.NoError(t, err)
	assert.Len(t, c.PublicID, 21)
	assert.Equal(t, channel.ProviderOpenWA, c.Provider)
	assert.Equal(t, channel.StatusActive, c.Status)
	assert.Equal(t, 3*time.Second, c.QuietWindow)
	assert.Equal(t, 10*time.Second, c.QuietMax)
	assert.NotEmpty(t, c.ReplyFailure)
	assert.NotEmpty(t, c.ReplyVoice)
	assert.NotEmpty(t, c.ReplyPhotoUnreadable)
	assert.Equal(t, "waiting", c.ReportedStatus(), "no verified outcome yet")
}

func TestNew_Validates(t *testing.T) {
	for name, c := range map[string]struct {
		edit         func(*channel.Input)
		allowPrivate bool
		field        string
	}{
		"http without the operator flag":  {edit: func(in *channel.Input) { in.BaseURL = "http://wa.internal" }, field: "base_url"},
		"a CR or LF":                      {edit: func(in *channel.Input) { in.BaseURL = "https://wa.example.com/\r\nX: y" }, field: "base_url"},
		"not absolute":                    {edit: func(in *channel.Input) { in.BaseURL = "/api" }, field: "base_url"},
		"over 2048":                       {edit: func(in *channel.Input) { in.BaseURL = "https://wa.example.com/" + strings.Repeat("p", 2030) }, field: "base_url"},
		"a device id that isn't OpenWA's": {edit: func(in *channel.Input) { in.DeviceID = "device-1" }, field: "device_id"},
		"no org":                          {edit: func(in *channel.Input) { in.OpenWAOrg = "" }, field: "openwa_org"},
		"no project":                      {edit: func(in *channel.Input) { in.OpenWAProject = " " }, field: "openwa_project"},
		"quiet window over 10 s":          {edit: func(in *channel.Input) { in.QuietWindow = new(11 * time.Second) }, field: "quiet_window_ms"},
		"quiet max below the window":      {edit: func(in *channel.Input) { in.QuietWindow, in.QuietMax = new(5*time.Second), new(4*time.Second) }, field: "quiet_max_ms"},
		"a set reply over 1000":           {edit: func(in *channel.Input) { in.ReplyFailure = strings.Repeat("x", 1001) }, field: "reply_failure"},
		"an unknown status":               {edit: func(in *channel.Input) { in.Status = "paused" }, field: "status"},
		"a query":                         {edit: func(in *channel.Input) { in.BaseURL = "https://wa.example.com/?x=1" }, field: "base_url"},
		"an empty query":                  {edit: func(in *channel.Input) { in.BaseURL = "https://wa.example.com/?" }, field: "base_url"},
		"a fragment":                      {edit: func(in *channel.Input) { in.BaseURL = "https://wa.example.com/#x" }, field: "base_url"},
	} {
		t.Run(name, func(t *testing.T) {
			in := input()
			c.edit(&in)
			_, err := channel.New(uuid.New(), uuid.New(), uuid.New(), in, time.Now(), channel.AllowPrivateHosts(c.allowPrivate))
			var invalid *channel.InvalidError
			require.ErrorAs(t, err, &invalid)
			assert.Equal(t, c.field, invalid.Field)
		})
	}
	in := input()
	in.BaseURL = "http://wa.internal:8080/prefix"
	_, err := channel.New(uuid.New(), uuid.New(), uuid.New(), in, time.Now(), channel.AllowPrivateHosts(true))
	assert.NoError(t, err, "http is allowed only with the operator flag")
}

func TestConnection_ReportedStatusComesFromVerifiedOutcomes(t *testing.T) {
	c, err := channel.New(uuid.New(), uuid.New(), uuid.New(), input(), time.Now())
	require.NoError(t, err)
	c.LastResult = channel.ResultConnected
	assert.Equal(t, "connected", c.ReportedStatus())
	c.LastBadSignatureAt = new(time.Now())
	assert.Equal(t, "connected", c.ReportedStatus(), "a bad signature never changes the status")
}
