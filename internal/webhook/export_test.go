package webhook

import "time"

// VerifyV1Counting is VerifyV1 that also reports how many HMACs it computed.
func VerifyV1Counting(secrets []string, timestamp, header string, body []byte, now time.Time, window time.Duration) (signs int, err error) {
	count := func(secret, timestamp string, body []byte) string {
		signs++
		return Sign(secret, timestamp, body)
	}
	err = verifyV1(count, secrets, timestamp, header, body, now, window)
	return signs, err
}
