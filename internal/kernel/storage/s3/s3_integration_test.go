//go:build integration

package s3_test

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/storage"
	storages3 "altalune.id/oagent/internal/kernel/storage/s3"
	"altalune.id/oagent/internal/kernel/storage/storagetest"
	"altalune.id/oagent/internal/testutil/s3test"
	s3conn "altalune.id/oagent/s3"
)

func newDriver(t *testing.T) (*storages3.Driver, *s3test.Handle) {
	t.Helper()
	h := s3test.New(t)
	h.Client(t)
	c, err := s3conn.Open(t.Context(), s3conn.Config{
		Endpoint: h.Endpoint, Region: h.Region, Bucket: h.Bucket, AccessKeyID: h.AccessKeyID,
		SecretAccessKey: h.SecretAccessKey, UsePathStyle: true, Timeout: 5 * time.Second,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	return storages3.New(c), h
}

func send(t *testing.T, up storage.Upload, body []byte) int {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), up.Method, up.URL, bytes.NewReader(body))
	require.NoError(t, err)
	for k, v := range up.Headers {
		if k != "Content-Length" {
			req.Header.Set(k, v)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	return resp.StatusCode
}

func TestDriver_UploadHeadReadDelete(t *testing.T) {
	d, h := newDriver(t)
	ctx := t.Context()
	key := h.Prefix + "public/oagent/projects/p/logo/f.png"
	body := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0xab}, 92)...)

	_, err := d.Head(ctx, key)
	assert.True(t, storage.IsObjectNotFoundError(err), "%v", err)
	_, err = d.ReadPrefix(ctx, key, 16)
	assert.True(t, storage.IsObjectNotFoundError(err), "%v", err)

	up, err := d.PresignPut(ctx, key, "image/png", int64(len(body)), time.Minute)
	require.NoError(t, err)
	assert.Equal(t, "PUT", up.Method)
	assert.Equal(t, map[string]string{"Content-Type": "image/png", "Content-Length": "100"}, up.Headers)
	assert.Equal(t, http.StatusForbidden, send(t, storage.Upload{URL: up.URL, Method: up.Method, Headers: map[string]string{"Content-Type": "image/jpeg"}}, body))
	assert.Equal(t, http.StatusOK, send(t, up, body))

	obj, err := d.Head(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, storage.Object{Size: 100, ContentType: "image/png"}, obj)
	head, err := d.ReadPrefix(ctx, key, 16)
	require.NoError(t, err)
	assert.Equal(t, body[:16], head)

	get, err := d.PresignGet(ctx, key, time.Minute)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(get, h.Endpoint+"/"), get)
	resp, err := http.Get(get) //nolint:noctx // a one-off fetch of a signed URL in a test.
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	require.NoError(t, d.Delete(ctx, key))
	require.NoError(t, d.Delete(ctx, key), "a missing object is not an error")
	_, err = d.Head(ctx, key)
	assert.True(t, storage.IsObjectNotFoundError(err))
}

func TestDriver_Contract(t *testing.T) {
	d, h := newDriver(t)
	sdk := h.Client(t)
	storagetest.Run(t, d, func(t *testing.T, key, contentType string, body []byte) {
		t.Helper()
		_, err := sdk.PutObject(t.Context(), &awss3.PutObjectInput{
			Bucket: aws.String(h.Bucket), Key: aws.String(key), ContentType: aws.String(contentType), Body: bytes.NewReader(body),
		})
		require.NoError(t, err)
	}, strings.ReplaceAll(strings.TrimSuffix(h.Prefix, "/"), "/", "-"))
}
