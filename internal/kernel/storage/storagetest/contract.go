// Package storagetest is the contract every storage.Driver passes.
package storagetest

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/storage"
)

// Put stores body under key as an upload would.
type Put func(t *testing.T, key, contentType string, body []byte)

// Run checks d against the Driver contract; keys live under app so drivers sharing a bucket stay apart, and are deleted when t ends.
func Run(t *testing.T, d storage.Driver, put Put, app string) {
	t.Helper()
	ctx := context.Background()
	owner := func(id string) storage.Prefix {
		t.Helper()
		p, err := storage.OwnerPrefix(app, storage.Purpose{Name: "knowledge", OwnerKind: storage.OwnerProjects, Visibility: storage.Private}, id)
		require.NoError(t, err)
		t.Cleanup(func() { assert.NoError(t, d.DeletePrefix(context.Background(), p)) })
		return p
	}
	files := owner("f")
	key := files.String() + "f.txt"

	t.Run("missing objects", func(t *testing.T) {
		_, err := d.Head(ctx, files.String()+"missing")
		assert.True(t, storage.IsObjectNotFoundError(err), "head: %v", err)
		_, err = d.ReadPrefix(ctx, files.String()+"missing", 4)
		assert.True(t, storage.IsObjectNotFoundError(err), "read prefix: %v", err)
		_, err = d.Open(ctx, files.String()+"missing", 10)
		assert.True(t, storage.IsObjectNotFoundError(err), "open: %v", err)
		require.NoError(t, d.Delete(ctx, files.String()+"missing"), "a missing object is not an error")
	})

	t.Run("head, prefix, open", func(t *testing.T) {
		put(t, key, "text/plain", []byte("hello world"))
		obj, err := d.Head(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, storage.Object{Size: 11, ContentType: "text/plain"}, obj)
		head, err := d.ReadPrefix(ctx, key, 5)
		require.NoError(t, err)
		assert.Equal(t, []byte("hello"), head)
		rc, err := d.Open(ctx, key, 11)
		require.NoError(t, err)
		all, err := io.ReadAll(rc)
		require.NoError(t, err)
		require.NoError(t, rc.Close())
		assert.Equal(t, []byte("hello world"), all)
	})

	t.Run("open is capped", func(t *testing.T) {
		put(t, key, "text/plain", bytes.Repeat([]byte("x"), 64))
		rc, err := d.Open(ctx, key, 16)
		require.NoError(t, err)
		_, err = io.ReadAll(rc)
		_ = rc.Close()
		assert.True(t, storage.IsObjectTooLargeError(err), "got %v", err)
	})

	t.Run("presign", func(t *testing.T) {
		up, err := d.PresignPut(ctx, key, "text/plain", 3, time.Minute)
		require.NoError(t, err)
		assert.Equal(t, storage.UploadMethod, up.Method)
		assert.Equal(t, "text/plain", up.Headers["Content-Type"])
		assert.WithinDuration(t, time.Now().Add(time.Minute), up.ExpiresAt, 10*time.Second)
		get, err := d.PresignGet(ctx, key, time.Minute)
		require.NoError(t, err)
		assert.NotEmpty(t, get)
	})

	t.Run("delete prefix", func(t *testing.T) {
		p := owner("p")
		sub, err := p.Sub("sub")
		require.NoError(t, err)
		gone := []string{p.String() + "a.txt", p.String() + "b.txt", sub.String() + "c.txt"}
		for _, k := range gone {
			put(t, k, "text/plain", []byte("x"))
		}
		sibling := owner("p2").String() + "keep.txt"
		put(t, sibling, "text/plain", []byte("x"))
		require.NoError(t, d.DeletePrefix(ctx, p))
		for _, k := range gone {
			_, err := d.Head(ctx, k)
			assert.True(t, storage.IsObjectNotFoundError(err), k)
		}
		_, err = d.Head(ctx, sibling)
		assert.NoError(t, err, "projects/p2/ survives deleting projects/p/")
		require.NoError(t, d.DeletePrefix(ctx, p), "an empty prefix is not an error")
	})

	t.Run("the zero prefix deletes nothing", func(t *testing.T) {
		keep := owner("z").String() + "keep.txt"
		put(t, keep, "text/plain", []byte("x"))
		err := d.DeletePrefix(ctx, storage.Prefix{})
		assert.True(t, storage.IsInvalidPrefixError(err), "got %v", err)
		_, err = d.Head(ctx, keep)
		assert.NoError(t, err, "nothing is deleted")
	})
}
