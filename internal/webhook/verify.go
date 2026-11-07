package webhook

import (
	"crypto/hmac"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// InvalidSignatureError reports a delivery whose signature or timestamp does not verify; it never says which part failed.
type InvalidSignatureError struct{}

func (e *InvalidSignatureError) Error() string { return "webhook: invalid signature" }

// IsInvalidSignatureError reports whether err's tree contains an *InvalidSignatureError.
func IsInvalidSignatureError(err error) bool {
	_, ok := errors.AsType[*InvalidSignatureError](err)
	return ok
}

const maxSignatureFields = 4

var timestampPattern = regexp.MustCompile(`^\d{1,12}$`)

// VerifyV1 checks a v1 signature header sent at timestamp (unix seconds) against every secret, within window of now.
func VerifyV1(secrets []string, timestamp, header string, body []byte, now time.Time, window time.Duration) error {
	return verifyV1(Sign, secrets, timestamp, header, body, now, window)
}

func verifyV1(
	sign func(secret, timestamp string, body []byte) string,
	secrets []string, timestamp, header string, body []byte, now time.Time, window time.Duration,
) error {
	if !timestampPattern.MatchString(timestamp) {
		return &InvalidSignatureError{}
	}
	sent, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return &InvalidSignatureError{}
	}
	if skew := now.Sub(time.Unix(sent, 0)).Abs(); skew > window {
		return &InvalidSignatureError{}
	}
	expected := make([][]byte, 0, len(secrets))
	for _, secret := range secrets {
		if secret != "" {
			expected = append(expected, []byte(sign(secret, timestamp, body)))
		}
	}
	seen := 0
	for field := range strings.FieldsSeq(header) {
		if !strings.HasPrefix(field, signatureScheme) {
			continue
		}
		seen++
		if seen > maxSignatureFields {
			break
		}
		for _, e := range expected {
			if hmac.Equal([]byte(field), e) {
				return nil
			}
		}
	}
	return &InvalidSignatureError{}
}
