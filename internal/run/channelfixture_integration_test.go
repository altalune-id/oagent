//go:build integration

package run_test

import (
	"strconv"

	"altalune.id/oagent/internal/channel"
)

var quietDefaults = strconv.FormatInt(channel.DefaultQuietWindow.Milliseconds(), 10) + ", " +
	strconv.FormatInt(channel.DefaultQuietMax.Milliseconds(), 10)
