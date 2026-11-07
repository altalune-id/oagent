package channel_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/channel"
)

func TestFormat_Golden(t *testing.T) {
	inputs, err := filepath.Glob(filepath.Join("testdata", "format", "*.md"))
	require.NoError(t, err)
	require.NotEmpty(t, inputs)
	for _, in := range inputs {
		t.Run(filepath.Base(in), func(t *testing.T) {
			src, err := os.ReadFile(in)
			require.NoError(t, err)
			want, err := os.ReadFile(strings.TrimSuffix(in, ".md") + ".golden")
			require.NoError(t, err)
			assert.Equal(t, []string{string(want)}, channel.Format(string(src)))
		})
	}
}

func paragraph(r rune, n int) string { return strings.Repeat(string(r), n) }

func TestFormat_SplitsAtParagraphBreaks(t *testing.T) {
	a, b, c := paragraph('a', 2500), paragraph('b', 2500), paragraph('c', 1000)
	assert.Equal(t, []string{a, b + "\n\n" + c}, channel.Format(a+"\n\n"+b+"\n\n"+c))
}

func TestFormat_AtMostThreePartsThenCut(t *testing.T) {
	text := strings.Join([]string{paragraph('a', 3900), paragraph('b', 3900), paragraph('c', 3900), paragraph('d', 100)}, "\n\n")
	parts := channel.Format(text)
	require.Len(t, parts, 3)
	assert.Equal(t, paragraph('a', 3900), parts[0])
	assert.Equal(t, paragraph('b', 3900), parts[1])
	assert.True(t, strings.HasSuffix(parts[2], "…"), "the rest is cut with an ellipsis")
	for _, p := range parts {
		assert.LessOrEqual(t, utf8.RuneCountInString(p), 4000)
	}
}

func TestFormat_LongParagraphIsCutAtRunes(t *testing.T) {
	parts := channel.Format(paragraph('é', 4001))
	require.Len(t, parts, 2)
	assert.Equal(t, paragraph('é', 4000), parts[0])
	assert.Equal(t, "é", parts[1])
}

func TestFormat_EmptyHasNoParts(t *testing.T) {
	assert.Empty(t, channel.Format("  \n "))
}
