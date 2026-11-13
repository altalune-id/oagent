package extract_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/knowledge/extract"
)

func TestPDF_ThreePagesOfText(t *testing.T) {
	data := fixture(t, "text_3pages.pdf")
	n, err := extract.PDF{}.PageCount(data)
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	sections, err := extract.PDF{}.Pages(data)
	require.NoError(t, err)
	require.Len(t, sections, 3)
	for i, want := range []string{"Refund policy", "Shipping", "Contact"} {
		assert.Equal(t, i+1, sections[i].Page)
		assert.Contains(t, sections[i].Text, want)
	}
}

func TestExtract_ScannedPDFNoText(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, extract.Serve(context.Background(), extract.KindPDF, bytes.NewReader(fixture(t, "scanned_no_text.pdf")), &out, extract.PDF{}))
	assert.JSONEq(t, `{"error":{"kind":"no_text"}}`, out.String())
}

func TestPDF_NotAPDFIsAnError(t *testing.T) {
	_, err := extract.PDF{}.PageCount([]byte("not a pdf"))
	assert.Error(t, err)
	var out bytes.Buffer
	require.NoError(t, extract.Serve(context.Background(), extract.KindPDF, strings.NewReader("not a pdf"), &out, extract.PDF{}))
	assert.JSONEq(t, `{"error":{"kind":"unreadable","reason":"corrupt"}}`, out.String())
}
