package extract_test

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/knowledge/extract"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "testdata", "docs", name))
	require.NoError(t, err)
	return raw
}

func headings(sections []extract.Section) []string {
	out := []string{}
	for _, s := range sections {
		out = append(out, s.Heading)
	}
	return out
}

func joined(sections []extract.Section) string {
	parts := make([]string, 0, len(sections))
	for _, s := range sections {
		parts = append(parts, s.Text)
	}
	return strings.Join(parts, "\n")
}

func reason(t *testing.T, err error) extract.RejectReason {
	t.Helper()
	rejected, ok := extract.AsRejectedError(err)
	require.True(t, ok, "want a RejectedError, got %v", err)
	return rejected.Reason
}

func TestDocx_HeadingsFromParagraphStyles(t *testing.T) {
	sections, err := extract.Extract(context.Background(), extract.KindDOCX, fixture(t, "headings.docx"))
	require.NoError(t, err)
	assert.Equal(t, []string{"Employee handbook", "Leave", "Expenses", "Travel"}, headings(sections))
	assert.Equal(t, "Annual leave is 12 days a year.", sections[1].Text)
	assert.Equal(t, "Book flights through the travel desk.", sections[3].Text)
}

func TestDocx_ZipBombLyingHeadersStopped(t *testing.T) {
	_, err := extract.Extract(context.Background(), extract.KindDOCX, fixture(t, "zip_bomb.docx"))
	assert.Equal(t, extract.TooLarge, reason(t, err), "the cap on decompressed bytes stops it, whatever the header says")
}

func TestDocx_MissingDocumentXML(t *testing.T) {
	_, err := extract.Extract(context.Background(), extract.KindDOCX, fixture(t, "missing_document_xml.docx"))
	assert.Equal(t, extract.MissingDocumentXML, reason(t, err))
}

func TestDocx_TooManyEntries(t *testing.T) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for i := range extract.MaxDocxEntries + 1 {
		_, err := w.Create("f" + strconv.Itoa(i))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	_, err := extract.Extract(context.Background(), extract.KindDOCX, buf.Bytes())
	assert.Equal(t, extract.TooManyEntries, reason(t, err))
}

func TestDocx_NotAZip(t *testing.T) {
	_, err := extract.Extract(context.Background(), extract.KindDOCX, []byte("not a zip"))
	assert.True(t, extract.IsUnreadableError(err), "got %v", err)
}

func TestHTML_DropsChrome(t *testing.T) {
	sections, err := extract.Extract(context.Background(), extract.KindHTML, fixture(t, "page.html"))
	require.NoError(t, err)
	assert.Equal(t, []string{"Opening hours", "Holidays"}, headings(sections))
	text := joined(sections)
	assert.Contains(t, text, "We open 08:00 to 22:00 every day.")
	assert.Contains(t, text, "Closed on 1 January.")
	for _, chrome := range []string{"track", "alert", "Home", "Menu", "Subscribe", "body{}"} {
		assert.NotContains(t, text, chrome)
	}
}

func TestHTML_DropsHeaderAndFooter(t *testing.T) {
	page := `<html><body><header>Site name</header><h1>Title</h1><p>Body text.</p><footer>Copyright</footer></body></html>`
	sections, err := extract.Extract(context.Background(), extract.KindHTML, []byte(page))
	require.NoError(t, err)
	assert.Equal(t, "Body text.", joined(sections))
}

func TestMarkdown_SectionsByHeading(t *testing.T) {
	sections, err := extract.Extract(context.Background(), extract.KindMarkdown, fixture(t, "guide.md"))
	require.NoError(t, err)
	assert.Equal(t, []string{"Returns guide", "Eligibility", "Steps"}, headings(sections))
	assert.Contains(t, sections[2].Text, "RMA-0001")
}

func TestMarkdown_HashInsideAFenceIsNotAHeading(t *testing.T) {
	sections, err := extract.Extract(context.Background(), extract.KindMarkdown, []byte("# Top\n\n```\n# not a heading\n```\n"))
	require.NoError(t, err)
	assert.Equal(t, []string{"Top"}, headings(sections))
	assert.Contains(t, sections[0].Text, "# not a heading")
}

func TestText_InvalidUTF8(t *testing.T) {
	for _, kind := range []extract.Kind{extract.KindText, extract.KindMarkdown} {
		_, err := extract.Extract(context.Background(), kind, fixture(t, "cp1252.txt"))
		assert.Equal(t, extract.InvalidUTF8, reason(t, err), kind)
	}
}

func TestText_AsIs(t *testing.T) {
	sections, err := extract.Extract(context.Background(), extract.KindText, []byte("Line one.\nLine two.\n"))
	require.NoError(t, err)
	assert.Equal(t, []extract.Section{{Text: "Line one.\nLine two."}}, sections)
}

func TestExtract_NoText(t *testing.T) {
	for kind, data := range map[extract.Kind][]byte{
		extract.KindText:     []byte(" \n\t "),
		extract.KindMarkdown: []byte("#\n"),
		extract.KindHTML:     []byte("<html><body><script>x()</script></body></html>"),
	} {
		_, err := extract.Extract(context.Background(), kind, data)
		assert.True(t, extract.IsNoTextError(err), "%s: %v", kind, err)
	}
}

func TestExtract_UnknownKind(t *testing.T) {
	_, err := extract.Extract(context.Background(), extract.Kind("exe"), []byte("x"))
	assert.Equal(t, extract.UnsupportedType, reason(t, err))
}

func TestExtract_PDFIsNotInProcess(t *testing.T) {
	_, err := extract.Extract(context.Background(), extract.KindPDF, fixture(t, "text_3pages.pdf"))
	assert.Equal(t, extract.UnsupportedType, reason(t, err), "PDFs go through the child process only")
}

func docxWithBody(t *testing.T, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.Create("word/document.xml")
	require.NoError(t, err)
	_, err = f.Write([]byte(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>` +
		body + `</w:t></w:r></w:p></w:body></w:document>`))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return buf.Bytes()
}

func TestDocx_PartOverTenMiBIsRefused(t *testing.T) {
	_, err := extract.Extract(context.Background(), extract.KindDOCX, docxWithBody(t, strings.Repeat("a", 11<<20)))
	assert.Equal(t, extract.TooLarge, reason(t, err))
}

func TestBuilder_StopsJustPastTheTextCap(t *testing.T) {
	line := strings.Repeat("word ", 200) + "\n"
	sections, err := extract.Extract(context.Background(), extract.KindMarkdown, []byte("# Big\n\n"+strings.Repeat(line, 3*extract.MaxTextBytes/len(line))))
	require.NoError(t, err)
	total := 0
	for _, s := range sections {
		total += len(s.Text)
	}
	assert.Greater(t, total, extract.MaxTextBytes, "still over the cap, so the document is refused as too long")
	assert.Less(t, total, extract.MaxTextBytes+extract.MaxTextBytes/100, "but little past it is held")
}

func TestExtract_StripsNULBytes(t *testing.T) {
	for kind, data := range map[extract.Kind][]byte{
		extract.KindText:     []byte("sixteen bytes ok\x00 then more text"),
		extract.KindMarkdown: []byte("# Head\x00ing\n\nbody\x00 text"),
		extract.KindHTML:     []byte("<h1>Head\x00ing</h1><p>body\x00 text</p>"),
	} {
		sections, err := extract.Extract(context.Background(), kind, data)
		require.NoError(t, err, kind)
		for _, s := range sections {
			assert.NotContains(t, s.Text, "\x00", kind)
			assert.NotContains(t, s.Heading, "\x00", kind)
		}
	}
}
