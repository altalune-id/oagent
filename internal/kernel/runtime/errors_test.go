package runtime_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/internal/kernel/runtime"
)

func TestErrors_NameTheirCause(t *testing.T) {
	cause := errors.New("boom")
	for _, c := range []struct {
		err  error
		want string
		is   func(error) bool
	}{
		{&runtime.ContractError{Status: 400, Code: "invalid_request"}, "400 invalid_request", runtime.IsContractError},
		{&runtime.ContractError{Status: 200, Cause: cause}, "boom", runtime.IsContractError},
		{&runtime.SnapshotUnloadableError{}, "snapshot", runtime.IsSnapshotUnloadableError},
		{&runtime.BusyError{}, "busy", runtime.IsBusyError},
		{&runtime.StatusError{Status: 502}, "502", runtime.IsStatusError},
		{&runtime.NoTerminalError{}, "terminal", runtime.IsNoTerminalError},
		{&runtime.NoTerminalError{Cause: cause}, "boom", runtime.IsNoTerminalError},
	} {
		assert.Contains(t, c.err.Error(), c.want)
		assert.True(t, c.is(fmt.Errorf("wrapped: %w", c.err)))
	}
	assert.ErrorIs(t, &runtime.ContractError{Cause: cause}, cause)
	assert.ErrorIs(t, &runtime.NoTerminalError{Cause: cause}, cause)
}
