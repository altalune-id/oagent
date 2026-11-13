package s3_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/storage"
	storages3 "altalune.id/oagent/internal/kernel/storage/s3"
	s3conn "altalune.id/oagent/s3"
)

const noSuchKey = `<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchKey</Code><Message>missing</Message></Error>`

func fakeBucket(t *testing.T, objectStatus int) *storages3.Driver {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead && r.URL.Path == "/b" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(objectStatus)
		if objectStatus == http.StatusNotFound && r.Method != http.MethodHead {
			_, _ = w.Write([]byte(noSuchKey))
		}
	}))
	t.Cleanup(srv.Close)
	c, err := s3conn.Open(t.Context(), s3conn.Config{
		Endpoint: srv.URL, Region: "garage", Bucket: "b", AccessKeyID: "k", SecretAccessKey: "s",
		UsePathStyle: true, Timeout: 2 * time.Second,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	return storages3.New(c)
}

func TestDriver_MissingObjectIsObjectNotFound(t *testing.T) {
	d := fakeBucket(t, http.StatusNotFound)
	_, err := d.Head(t.Context(), "k")
	assert.True(t, storage.IsObjectNotFoundError(err), "head: %v", err)
	_, err = d.ReadPrefix(t.Context(), "k", 16)
	assert.True(t, storage.IsObjectNotFoundError(err), "read: %v", err)
}

func TestDriver_OtherFailuresAreWrapped(t *testing.T) {
	d := fakeBucket(t, http.StatusForbidden)
	_, err := d.Head(t.Context(), "k")
	require.Error(t, err)
	assert.False(t, storage.IsObjectNotFoundError(err))
	assert.True(t, strings.HasPrefix(err.Error(), "storage.s3: head:"), err.Error())
	_, err = d.ReadPrefix(t.Context(), "k", 16)
	assert.ErrorContains(t, err, "storage.s3: read:")
	assert.ErrorContains(t, d.Delete(t.Context(), "k"), "storage.s3: delete:")
}

func TestDriver_PresignedURLsCoverTheContentTypeAndLength(t *testing.T) {
	d := fakeBucket(t, http.StatusOK)
	up, err := d.PresignPut(t.Context(), "public/a/b.png", "image/png", 42, time.Minute)
	require.NoError(t, err)
	assert.Contains(t, up.URL, "/b/public/a/b.png?")
	assert.Contains(t, up.URL, "X-Amz-SignedHeaders=content-length%3Bcontent-type%3Bhost")
	assert.Equal(t, map[string]string{"Content-Type": "image/png", "Content-Length": "42"}, up.Headers)
	assert.WithinDuration(t, time.Now().Add(time.Minute), up.ExpiresAt, 5*time.Second)

	get, err := d.PresignGet(t.Context(), "private/a/b.csv", time.Minute)
	require.NoError(t, err)
	assert.Contains(t, get, "/b/private/a/b.csv?")
	assert.Contains(t, get, "X-Amz-Expires=60")
}
