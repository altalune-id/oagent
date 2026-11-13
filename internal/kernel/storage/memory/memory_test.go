package memory

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/storage"
	"altalune.id/oagent/internal/kernel/storage/storagetest"
)

func TestDriver_Contract(t *testing.T) {
	d := New()
	storagetest.Run(t, d, func(_ *testing.T, key, ct string, body []byte) { d.Put(key, ct, body) }, "oagent")
}

func TestDriver_StandsInForABucket(t *testing.T) {
	ctx := context.Background()
	d := New()
	up, err := d.PresignPut(ctx, "public/oagent/projects/p/logo/f.png", "image/png", 3, time.Minute)
	require.NoError(t, err)
	assert.Equal(t, "memory://upload/public%2Foagent%2Fprojects%2Fp%2Flogo%2Ff.png", up.URL)
	assert.Equal(t, map[string]string{"Content-Type": "image/png", "Content-Length": "3"}, up.Headers)
	d.Put("k", "image/png", []byte("abc"))
	assert.True(t, d.Has("k"))
	u, err := d.PresignGet(ctx, "k", time.Minute)
	require.NoError(t, err)
	assert.Contains(t, u, "memory://get/k?expires=")
	require.NoError(t, d.Delete(ctx, "k"))
	assert.False(t, d.Has("k"))
	_, err = d.Head(ctx, "k")
	assert.True(t, storage.IsObjectNotFoundError(err))
}
