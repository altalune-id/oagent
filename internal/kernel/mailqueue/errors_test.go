package mailqueue

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrors(t *testing.T) {
	assert.True(t, IsSealerRequiredError(fmt.Errorf("w: %w", &SealerRequiredError{})))
	assert.Contains(t, (&SealerRequiredError{}).Error(), "secrets.encryptionKey")
	assert.True(t, IsPayloadError(fmt.Errorf("w: %w", &PayloadError{ID: "a", Reason: "missing"})))
	assert.Equal(t, "mailqueue: entry a payload: missing", (&PayloadError{ID: "a", Reason: "missing"}).Error())
	assert.True(t, IsSettleRefusedError(fmt.Errorf("w: %w", &SettleRefusedError{ID: "a"})))
	assert.Contains(t, (&SettleRefusedError{ID: "a"}).Error(), "a")
	assert.False(t, IsPayloadError(errBoom))
}
