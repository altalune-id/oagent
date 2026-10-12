package password

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fixtureCase struct {
	Name     string   `json:"name"`
	Password string   `json:"password"`
	Email    string   `json:"email"`
	NameHint string   `json:"name_hint"`
	Reasons  []Reason `json:"reasons"`
}

func TestViolations_SharedCases(t *testing.T) {
	raw, err := os.ReadFile("testdata/cases.json")
	require.NoError(t, err)
	var cases []fixtureCase
	require.NoError(t, json.Unmarshal(raw, &cases))
	require.NotEmpty(t, cases)
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			got := Violations(c.Password, Hints{Email: c.Email, Name: c.NameHint})
			if len(c.Reasons) == 0 {
				assert.Empty(t, got)
				return
			}
			assert.Equal(t, c.Reasons, got)
		})
	}
}

func TestViolations_Rules(t *testing.T) {
	for name, tc := range map[string]struct {
		plain string
		hints Hints
		want  []Reason
	}{
		"exactly min":            {"tea-for-two!", Hints{}, nil},
		"exactly max":            {strings.Repeat("ab1-", MaxLen/4-1) + "wxyz", Hints{}, nil},
		"min multibyte":          {"pässwörd-für", Hints{}, nil},
		"one under min":          {"pässwörd-fü", Hints{}, []Reason{ReasonTooShort}},
		"repeated 4":             {"abcdabcdabcd", Hints{}, []Reason{ReasonRepeated}},
		"unit 5 is not repeated": {"abcdeabcdeabcde", Hints{}, nil},
		"digits sequential":      {"0123456789", Hints{}, []Reason{ReasonTooShort, ReasonSequential}},
		"step 2 not sequential":  {"acegikmoqsuw", Hints{}, nil},
		"full email":             {"x-bo@x.io-long-enough", Hints{Email: "bo@x.io"}, []Reason{ReasonContainsEmail}},
		"email case and spaces":  {"Alice-likes-tea", Hints{Email: "  ALICE@example.com "}, []Reason{ReasonContainsEmail}},
		"name case":              {"I am MARIA forever", Hints{Name: "ana maria"}, []Reason{ReasonContainsName}},
		"email and name":         {"alice-and-maria-x", Hints{Email: "alice@x.io", Name: "Maria"}, []Reason{ReasonContainsEmail, ReasonContainsName}},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, Violations(tc.plain, tc.hints))
		})
	}
}

func TestPolicyJSON_MatchesTheConstants(t *testing.T) {
	raw, err := os.ReadFile("policy.json")
	require.NoError(t, err)
	var p struct{ MinLength, MaxLength, HintMinLength int }
	require.NoError(t, json.Unmarshal(raw, &p))
	assert.Equal(t, MinLen, p.MinLength)
	assert.Equal(t, MaxLen, p.MaxLength)
	assert.Equal(t, hintMinLen, p.HintMinLength)
}

func TestCommonList_Shape(t *testing.T) {
	n := 0
	for line := range strings.Lines(commonRaw) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		n++
		assert.Equal(t, strings.ToLower(line), line)
		l := utf8.RuneCountInString(line)
		assert.True(t, l >= MinLen && l <= MaxLen, "%q", line)
	}
	assert.Positive(t, n)
	assert.Len(t, commonSet(), n, "duplicates in common.txt")
}

func TestCheck_FirstViolationAndLimits(t *testing.T) {
	err := Check("alicealice", Hints{Email: "alice@example.com"})
	var pe *PolicyError
	require.ErrorAs(t, err, &pe)
	assert.Equal(t, PolicyError{Reason: ReasonTooShort, Min: MinLen, Max: MaxLen}, *pe)
	assert.NoError(t, Check("correct horse battery", Hints{}))
}

func TestViolations_TooLongSkipsTheRest(t *testing.T) {
	assert.Equal(t, []Reason{ReasonTooLong}, Violations(strings.Repeat("a", MaxLen+1), Hints{Email: "aaaa@x.io"}))
}
