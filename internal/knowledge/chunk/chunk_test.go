package chunk_test

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/knowledge/chunk"
	"altalune.id/oagent/internal/knowledge/extract"
)

var update = flag.Bool("update", false, "rewrite the golden files instead of comparing against them")

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "testdata", "docs", name))
	require.NoError(t, err)
	return b
}

func TestChunk_Golden(t *testing.T) {
	for _, tc := range []struct {
		golden, file, url string
		kind              extract.Kind
	}{
		{"guide_md", "guide.md", "", extract.KindMarkdown},
		{"page_html", "page.html", "https://example.com/page", extract.KindHTML},
		{"headings_docx", "headings.docx", "", extract.KindDOCX},
		{"text_3pages_pdf", "text_3pages.pdf", "", extract.KindPDF},
	} {
		t.Run(tc.golden, func(t *testing.T) {
			var (
				sections []extract.Section
				err      error
			)
			if tc.kind == extract.KindPDF {
				sections, err = extract.PDF{}.Pages(fixture(t, tc.file))
			} else {
				sections, err = extract.Extract(context.Background(), tc.kind, fixture(t, tc.file))
			}
			require.NoError(t, err)
			got, err := chunk.Split(chunk.Doc{Title: tc.file, URL: tc.url, Sections: sections})
			require.NoError(t, err)
			require.NotEmpty(t, got)
			raw, err := json.MarshalIndent(got, "", "  ")
			require.NoError(t, err)
			raw = append(raw, '\n')
			path := filepath.Join("testdata", tc.golden+".json")
			if *update {
				require.NoError(t, os.WriteFile(path, raw, 0o600))
			}
			want, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, string(want), string(raw))
		})
	}
}

func TestChunk_NormalizesAndKeepsParagraphs(t *testing.T) {
	got, err := chunk.Split(chunk.Doc{Title: "T", Sections: []extract.Section{
		{Heading: "  Ｈｅａｄ  ", Text: "ﬁrst\t\t line   here\nsecond  line"},
	}})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, chunk.Chunk{
		Ordinal: 0, HeadingPath: "Head", Text: "first line here\nsecond line",
		EmbedText: "T — Head\n\nfirst line here\nsecond line",
	}, got[0])
}

func TestChunk_EmbedTextWithoutHeadingOrTitle(t *testing.T) {
	got, err := chunk.Split(chunk.Doc{Title: "T", Sections: []extract.Section{{Text: "body"}}})
	require.NoError(t, err)
	assert.Equal(t, "T\n\nbody", got[0].EmbedText)
	got, err = chunk.Split(chunk.Doc{Sections: []extract.Section{{Heading: "H", Text: "body"}}})
	require.NoError(t, err)
	assert.Equal(t, "H\n\nbody", got[0].EmbedText)
}

func TestChunk_SplitsByHeadingAndPage(t *testing.T) {
	got, err := chunk.Split(chunk.Doc{URL: "https://example.com/a", Sections: []extract.Section{
		{Page: 1, Text: "one"}, {Page: 2, Text: "two"}, {Heading: "H", Text: "three"}, {Heading: "Empty"},
	}})
	require.NoError(t, err)
	require.Len(t, got, 3, "an empty section makes no chunk")
	for i, want := range []chunk.Chunk{
		{Ordinal: 0, Page: 1, Text: "one"}, {Ordinal: 1, Page: 2, Text: "two"}, {Ordinal: 2, HeadingPath: "H", Text: "three"},
	} {
		assert.Equal(t, want.Ordinal, got[i].Ordinal)
		assert.Equal(t, want.Page, got[i].Page)
		assert.Equal(t, want.HeadingPath, got[i].HeadingPath)
		assert.Equal(t, want.Text, got[i].Text)
		assert.Equal(t, "https://example.com/a", got[i].URL)
	}
}

func sentence(i int) string {
	return "Sentence " + strings.Repeat("x", 40) + " number " + string(rune('a'+i%26)) + "."
}

func TestChunk_TargetSizeAndOverlap(t *testing.T) {
	var paras []string
	for i := range 40 {
		var s []string
		for j := range 5 {
			s = append(s, sentence(i*5+j))
		}
		paras = append(paras, strings.Join(s, " "))
	}
	got, err := chunk.Split(chunk.Doc{Sections: []extract.Section{{Text: strings.Join(paras, "\n")}}})
	require.NoError(t, err)
	require.Greater(t, len(got), 3)
	for i, c := range got {
		n := utf8.RuneCountInString(c.Text)
		assert.LessOrEqual(t, n, chunk.TargetChars, "chunk %d", i)
		if i < len(got)-1 {
			assert.Greater(t, n, chunk.TargetChars-utf8.RuneCountInString(paras[0])-chunk.OverlapChars, "chunk %d is filled", i)
		}
		if i > 0 {
			prev := got[i-1].Text
			head := c.Text[:strings.Index(c.Text, ".")+1]
			assert.Contains(t, prev[len(prev)-chunk.OverlapChars-len(head):], head, "chunk %d starts inside the tail of chunk %d", i, i-1)
		}
	}
}

func TestChunk_LongParagraphSplitsBySentenceThenWord(t *testing.T) {
	var s []string
	for i := range 200 {
		s = append(s, sentence(i))
	}
	long := strings.Join(s, " ")
	words := strings.Repeat("word ", 2000)
	got, err := chunk.Split(chunk.Doc{Sections: []extract.Section{{Text: long}, {Text: words}, {Text: strings.Repeat("y", 9000)}}})
	require.NoError(t, err)
	for i, c := range got {
		assert.LessOrEqual(t, utf8.RuneCountInString(c.Text), chunk.TargetChars, "chunk %d", i)
		assert.True(t, utf8.ValidString(c.Text))
		if c.Text[0] == 'S' {
			assert.True(t, strings.HasSuffix(c.Text, "."), "a sentence chunk ends on a sentence: %q", c.Text[len(c.Text)-10:])
		}
	}
	var ys int
	for _, c := range got {
		if c.Text[0] == 'y' {
			ys++
		}
	}
	assert.GreaterOrEqual(t, ys, 3, "a run with no spaces is cut at the target")
}

func TestChunk_Caps(t *testing.T) {
	_, err := chunk.Split(chunk.Doc{Sections: []extract.Section{{Text: strings.Repeat("a ", chunk.MaxTextBytes/2+1)}}})
	le, ok := chunk.AsLimitError(err)
	require.True(t, ok, "got %v", err)
	assert.Equal(t, chunk.LimitTextBytes, le.Limit)

	sections := make([]extract.Section, chunk.MaxChunks+1)
	for i := range sections {
		sections[i] = extract.Section{Page: i + 1, Text: "p"}
	}
	_, err = chunk.Split(chunk.Doc{Sections: sections})
	le, ok = chunk.AsLimitError(err)
	require.True(t, ok, "got %v", err)
	assert.Equal(t, chunk.LimitChunks, le.Limit)
	assert.Contains(t, le.Error(), "2000")

	got, err := chunk.Split(chunk.Doc{Sections: sections[:chunk.MaxChunks]})
	require.NoError(t, err)
	assert.Len(t, got, chunk.MaxChunks, "exactly the cap is allowed")
}

func TestChunk_NoTextIsNoChunks(t *testing.T) {
	got, err := chunk.Split(chunk.Doc{Sections: []extract.Section{{Text: " \n\t"}}})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestEmbedText_RebuildsWhatSplitStored(t *testing.T) {
	doc := chunk.Doc{Title: "  Refund policy\n v２ ", Sections: []extract.Section{
		{Heading: " Timing\t\n", Text: "Refunds take 7 days.\nCards only."},
		{Text: "No heading here."},
	}}
	got, err := chunk.Split(doc)
	require.NoError(t, err)
	require.Len(t, got, 2)
	for _, c := range got {
		assert.Equal(t, c.EmbedText, chunk.EmbedText(doc.Title, c.HeadingPath, c.Text), "chunk %d", c.Ordinal)
	}
	assert.Equal(t, "body", chunk.EmbedText("", "", "body"))
}
