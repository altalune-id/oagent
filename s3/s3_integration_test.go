//go:build integration

package s3

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/testutil/s3test"
)

func otherHost(endpoint string) string {
	if strings.Contains(endpoint, "localhost") {
		return strings.Replace(endpoint, "localhost", "127.0.0.1", 1)
	}
	return strings.Replace(endpoint, "127.0.0.1", "localhost", 1)
}

func TestOpen_RealBucket_PresignedPutEnforcesTypeAndSizeOnTheBrowserEndpoint(t *testing.T) {
	h := s3test.New(t)
	sdk := h.Client(t)
	cfg := Config{
		Endpoint: h.Endpoint, BrowserEndpoint: otherHost(h.Endpoint), Region: h.Region, Bucket: h.Bucket,
		AccessKeyID: h.AccessKeyID, SecretAccessKey: h.SecretAccessKey, UsePathStyle: true, Timeout: 5 * time.Second,
	}
	c, err := Open(t.Context(), cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	body := append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, bytes.Repeat([]byte{0xab}, 92)...)
	put := func(key, contentType string, b []byte) int {
		t.Helper()
		req, err := c.Presign.PresignPutObject(t.Context(), &awss3.PutObjectInput{
			Bucket: aws.String(c.Bucket), Key: aws.String(h.Prefix + key),
			ContentType: aws.String("image/png"), ContentLength: aws.Int64(int64(len(body))),
		})
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(req.URL, cfg.BrowserEndpoint+"/"), req.URL)
		hr, err := http.NewRequestWithContext(t.Context(), http.MethodPut, req.URL, bytes.NewReader(b))
		require.NoError(t, err)
		hr.Header.Set("Content-Type", contentType)
		resp, err := http.DefaultClient.Do(hr)
		require.NoError(t, err)
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	assert.Equal(t, http.StatusOK, put("ok.png", "image/png", body))
	assert.Equal(t, http.StatusForbidden, put("type.png", "image/jpeg", body))
	assert.Equal(t, http.StatusForbidden, put("big.png", "image/png", append(body, 0)))

	head, err := sdk.HeadObject(t.Context(), &awss3.HeadObjectInput{Bucket: aws.String(h.Bucket), Key: aws.String(h.Prefix + "ok.png")})
	require.NoError(t, err)
	assert.EqualValues(t, len(body), aws.ToInt64(head.ContentLength))
	assert.Equal(t, "image/png", aws.ToString(head.ContentType))
	got, err := sdk.GetObject(t.Context(), &awss3.GetObjectInput{Bucket: aws.String(h.Bucket), Key: aws.String(h.Prefix + "ok.png"), Range: aws.String("bytes=0-15")})
	require.NoError(t, err)
	first, err := io.ReadAll(got.Body)
	require.NoError(t, err)
	assert.Equal(t, body[:16], first)
	for _, k := range []string{"type.png", "big.png"} {
		_, err := sdk.HeadObject(t.Context(), &awss3.HeadObjectInput{Bucket: aws.String(h.Bucket), Key: aws.String(h.Prefix + k)})
		assert.Error(t, err, "a refused PUT writes nothing: %s", k)
	}

	health := NewHealth(c, time.Second, discard())
	health.probe(t.Context())
	assert.True(t, health.Ready())
}

func TestOpen_RealServer_UnknownBucketOrWrongKeyFails(t *testing.T) {
	h := s3test.New(t)
	cfg := Config{
		Endpoint: h.Endpoint, Region: h.Region, Bucket: "no-such-bucket",
		AccessKeyID: h.AccessKeyID, SecretAccessKey: h.SecretAccessKey, UsePathStyle: true, Timeout: 5 * time.Second,
	}
	_, err := Open(t.Context(), cfg)
	assert.ErrorContains(t, err, `s3: cannot reach bucket "no-such-bucket"`)

	cfg.Bucket, cfg.SecretAccessKey = h.Bucket, "wrong-secret-"+strings.Repeat("0", 51)
	_, err = Open(t.Context(), cfg)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), cfg.SecretAccessKey)
}
