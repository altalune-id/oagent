package approval_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/internal/approval"
)

func TestWords_Vectors(t *testing.T) {
	for text, want := range map[string]approval.Word{
		"Ya!":           approval.WordYes,
		"oke 👍":         approval.WordYes,
		"ok 👍🏽":         approval.WordYes,
		"  sip  ":       approval.WordYes,
		"YES.":          approval.WordYes,
		"lanjut!!":      approval.WordYes,
		"ga":            approval.WordNo,
		"Nggak...":      approval.WordNo,
		"batal ❤️":      approval.WordNo,
		"tidak, 20000":  approval.WordNone,
		"ya tapi 20000": approval.WordNone,
		"yess":          approval.WordNone,
		"":              approval.WordNone,
		"👍":             approval.WordNone,
		"¿ya":           approval.WordNone,
	} {
		assert.Equal(t, want, approval.MatchWord(text), "%q", text)
	}
}
