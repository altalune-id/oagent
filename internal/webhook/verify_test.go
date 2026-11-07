package webhook_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/webhook"
)

type verifyVector struct {
	Name      string   `json:"name"`
	Secrets   []string `json:"secrets"`
	Timestamp string   `json:"timestamp"`
	Body      string   `json:"body"`
	Header    string   `json:"header"`
	Now       int64    `json:"now"`
	Valid     bool     `json:"valid"`
}

func TestVerifyV1_Vectors(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "verify_vectors.json"))
	require.NoError(t, err)
	var vectors []verifyVector
	require.NoError(t, json.Unmarshal(raw, &vectors))
	require.GreaterOrEqual(t, len(vectors), 19)
	for _, v := range vectors {
		t.Run(v.Name, func(t *testing.T) {
			err := webhook.VerifyV1(v.Secrets, v.Timestamp, v.Header, []byte(v.Body), time.Unix(v.Now, 0), 5*time.Minute)
			if v.Valid {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.True(t, webhook.IsInvalidSignatureError(err), "every failure is the same error: %v", err)
			assert.Equal(t, "webhook: invalid signature", err.Error(), "no detail on which part failed")
		})
	}
}

func TestVerifyV1_NoSecretVerifiesNothing(t *testing.T) {
	ts := "1790000000"
	header := webhook.Sign("s", ts, []byte("{}"))
	assert.True(t, webhook.IsInvalidSignatureError(webhook.VerifyV1(nil, ts, header, []byte("{}"), time.Unix(1790000000, 0), time.Minute)))
	assert.True(t, webhook.IsInvalidSignatureError(webhook.VerifyV1([]string{""}, ts, webhook.Sign("", ts, []byte("{}")), []byte("{}"), time.Unix(1790000000, 0), time.Minute)),
		"an empty secret is never a key")
}

// SECURITY: a signature compared with == leaks its prefix through timing; every comparison goes through hmac.Equal.
func TestVerifyV1_ComparesInConstantTime(t *testing.T) {
	src, err := os.ReadFile("verify.go")
	require.NoError(t, err)
	assert.Contains(t, string(src), "hmac.Equal(")
	assert.False(t, regexp.MustCompile(`(==|!=)\s*(want|got|sig|Sign\()`).Match(src), "no ==/!= on a signature")
	assert.False(t, regexp.MustCompile(`(want|got|sig)\s*(==|!=)`).Match(src), "no ==/!= on a signature")
}

func TestVerifyV1_HeaderFieldsCostNoExtraHMAC(t *testing.T) {
	ts, body := "1790000000", []byte(strings.Repeat("x", 1<<16))
	junk := strings.Repeat("v1=00 ", 10_000)
	secrets := []string{"primary", "secondary"}
	for name, header := range map[string]string{
		"all junk":       junk,
		"valid at front": webhook.Sign("secondary", ts, body) + " " + junk,
	} {
		t.Run(name, func(t *testing.T) {
			signs, _ := webhook.VerifyV1Counting(secrets, ts, header, body, time.Unix(1790000000, 0), time.Minute)
			assert.LessOrEqual(t, signs, len(secrets), "at most one HMAC per secret, however many fields the header has")
		})
	}
}

func TestVerifyV1_ConsidersOnlyTheFirstFields(t *testing.T) {
	ts, body := "1790000000", []byte("{}")
	valid := webhook.Sign("s", ts, body)
	now := time.Unix(1790000000, 0)
	require.NoError(t, webhook.VerifyV1([]string{"s"}, ts, "v1=00 v1=00 v1=00 "+valid, body, now, time.Minute))
	assert.True(t, webhook.IsInvalidSignatureError(webhook.VerifyV1([]string{"s"}, ts, "v1=00 v1=00 v1=00 v1=00 "+valid, body, now, time.Minute)),
		"a signature past the field cap is never checked")
}

func TestVerifyV1_TimestampIsDigitsOnly(t *testing.T) {
	body := []byte("{}")
	now := time.Unix(1790000000, 0)
	for _, ts := range []string{"+1790000000", " 1790000000", "1790000000.0", "", "1234567890123"} {
		assert.True(t, webhook.IsInvalidSignatureError(webhook.VerifyV1([]string{"s"}, ts, webhook.Sign("s", ts, body), body, now, time.Minute)), "timestamp %q", ts)
	}
}
