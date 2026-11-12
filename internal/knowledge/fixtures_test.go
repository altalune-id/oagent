package knowledge

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const docxReadCap = 50 << 20

func readDoc(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "docs", name))
	require.NoError(t, err)
	return raw
}

var (
	xrefEntry = regexp.MustCompile(`(?m)^(\d{10}) 00000 n `)
	startxref = regexp.MustCompile(`startxref\s+(\d+)`)
)

func assertXrefPointsAtObjects(t *testing.T, raw []byte) int {
	t.Helper()
	entries := xrefEntry.FindAllSubmatch(raw, -1)
	require.NotEmpty(t, entries)
	for i, e := range entries {
		off, err := strconv.Atoi(string(e[1]))
		require.NoError(t, err)
		assert.True(t, bytes.HasPrefix(raw[off:], []byte(strconv.Itoa(i+1)+" 0 obj")), "object %d at %d", i+1, off)
	}
	m := startxref.FindSubmatch(raw)
	require.NotNil(t, m)
	at, err := strconv.Atoi(string(m[1]))
	require.NoError(t, err)
	assert.True(t, bytes.HasPrefix(raw[at:], []byte("xref")))
	return at
}

func TestFixtures_PDFs(t *testing.T) {
	text := readDoc(t, "text_3pages.pdf")
	assert.True(t, bytes.HasPrefix(text, []byte("%PDF-")))
	assertXrefPointsAtObjects(t, text)
	assert.Contains(t, string(text), "/Count 3")
	assert.Equal(t, 3, bytes.Count(text, []byte(") Tj ET")))

	scanned := readDoc(t, "scanned_no_text.pdf")
	assertXrefPointsAtObjects(t, scanned)
	assert.Contains(t, string(scanned), "/Subtype /Image")
	assert.NotRegexp(t, `\bT[jJ]\b`, string(scanned), "no text operator: there is no text layer")

	looping := readDoc(t, "looping_xref.pdf")
	at := assertXrefPointsAtObjects(t, looping)
	assert.Contains(t, string(looping), "/Prev "+strconv.Itoa(at)+" ", "the trailer's /Prev points at its own xref section")
}

func zipEntries(t *testing.T, name string) map[string]*zip.File {
	t.Helper()
	raw := readDoc(t, name)
	assert.True(t, bytes.HasPrefix(raw, []byte("PK\x03\x04")))
	r, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	require.NoError(t, err)
	out := map[string]*zip.File{}
	for _, f := range r.File {
		out[f.Name] = f
	}
	return out
}

func TestFixtures_DOCX(t *testing.T) {
	headings := zipEntries(t, "headings.docx")
	require.Contains(t, headings, "word/document.xml")
	require.Contains(t, headings, "word/styles.xml")
	rc, err := headings["word/document.xml"].Open()
	require.NoError(t, err)
	doc, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Contains(t, string(doc), `w:pStyle w:val="Heading1"`)
	assert.Contains(t, string(doc), `w:pStyle w:val="Heading2"`)

	missing := zipEntries(t, "missing_document_xml.docx")
	assert.NotContains(t, missing, "word/document.xml")
	assert.Contains(t, missing, "word/styles.xml")

	bomb := zipEntries(t, "zip_bomb.docx")
	f := bomb["word/document.xml"]
	require.NotNil(t, f)
	assert.Less(t, f.UncompressedSize64, uint64(docxReadCap), "the header claims a small size")
	raw, err := f.OpenRaw()
	require.NoError(t, err)
	n, err := io.Copy(io.Discard, io.LimitReader(flate.NewReader(raw), docxReadCap+1))
	require.NoError(t, err)
	assert.Equal(t, int64(docxReadCap+1), n, "the real content passes the 50 MiB read cap")
	assert.Less(t, len(readDoc(t, "zip_bomb.docx")), 128<<10, "small on disk")
}

func TestFixtures_TextAndHTML(t *testing.T) {
	md := readDoc(t, "guide.md")
	assert.True(t, utf8.Valid(md))
	assert.Contains(t, string(md), "\n## ")

	latin := readDoc(t, "cp1252.txt")
	assert.False(t, utf8.Valid(latin), "not UTF-8")
	assert.NotContains(t, string(latin), "\x00", "text has no NUL byte")

	html := string(readDoc(t, "page.html"))
	for _, tag := range []string{"<nav>", "<script>", "<aside>", "<article>"} {
		assert.Contains(t, html, tag)
	}
}

func TestFixtures_RobotsVectorsHaveOneHome(t *testing.T) {
	var found []string
	require.NoError(t, filepath.WalkDir(filepath.Join("..", ".."), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "node_modules" || d.Name() == ".git") {
			return filepath.SkipDir
		}
		if d.Name() == "robots_vectors.json" {
			found = append(found, filepath.ToSlash(path))
		}
		return nil
	}))
	assert.Equal(t, []string{"../../internal/knowledge/testdata/robots_vectors.json"}, found, "one vectors file; every parser reads this one")
}
