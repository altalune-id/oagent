package channeltest_test

import (
	"testing"

	"altalune.id/oagent/internal/channel/channeltest"
)

func TestFakeProvider_PassesTheContract(t *testing.T) {
	channeltest.Run(t, channeltest.NewFake)
}
