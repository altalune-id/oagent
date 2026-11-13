package memory_test

import (
	"strings"
	"testing"

	"buf.build/go/protovalidate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	memoryv1 "altalune.id/oagent/gen/go/memory/v1"
	"altalune.id/oagent/nanoid"
)

func TestProto_PageTokenFitsADriversOwnToken(t *testing.T) {
	v, err := protovalidate.New()
	require.NoError(t, err)
	user, store := nanoid.MustNew(21), nanoid.MustNew(21)
	assert.NoError(t, v.Validate(&memoryv1.ListMemoriesRequest{StoreId: store, EndUserId: user, PageToken: strings.Repeat("t", 4096)}))
	assert.Error(t, v.Validate(&memoryv1.ListMemoriesRequest{StoreId: store, EndUserId: user, PageToken: strings.Repeat("t", 4097)}))
	assert.NoError(t, v.Validate(&memoryv1.DeleteMemoryRequest{StoreId: store, EndUserId: user, Id: "rec-01H8_x"}), "a driver's own record id")
	assert.Error(t, v.Validate(&memoryv1.DeleteMemoryRequest{StoreId: store, EndUserId: user, Id: "bad id"}))
}

func TestProto_EveryFactCallNamesAStore(t *testing.T) {
	v, err := protovalidate.New()
	require.NoError(t, err)
	user := nanoid.MustNew(21)
	assert.Error(t, v.Validate(&memoryv1.ListMemoriesRequest{EndUserId: user}))
	assert.Error(t, v.Validate(&memoryv1.DeleteMemoryRequest{EndUserId: user, Id: "x"}))
	assert.Error(t, v.Validate(&memoryv1.ClearMemoriesRequest{EndUserId: user}))
	assert.Error(t, v.Validate(&memoryv1.ClearMemoriesRequest{EndUserId: user, StoreId: "not a public id!"}))
}
