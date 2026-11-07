package channel_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/internal/channel"
)

// SECURITY: a phone number written with separators is still a phone number.
func TestCleanDisplayName_MasksASeparatedPhoneNumber(t *testing.T) {
	for raw, want := range map[string]string{
		"Sari 0812-3456-7890":     "Sari ***",
		"+62 812 3456 7890":       "***",
		"(0812) 3456 7890 kantor": "*** kantor",
		"0812.3456.7890":          "***",
		"wa +62 812‑3456 7890":    "wa ***",
		"call 0812-3456-7890 now": "call *** now",
		"kode 1234-5678":          "kode ***",
		"Room 1234 Floor 5678":    "Room 1234 Floor 5678",
		"Rina 123 4567":           "Rina 123 4567",
		"Toko 99 Jaya 12":         "Toko 99 Jaya 12",
		"R2-D2 v1.5":              "R2-D2 v1.5",
	} {
		assert.Equal(t, want, channel.CleanDisplayName(raw), "%q", raw)
	}
}
