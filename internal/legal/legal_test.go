package legal

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDocuments(t *testing.T) {
	for slug, title := range map[string]string{TermsSlug: "Terms of Service", PrivacySlug: "Privacy Policy"} {
		t.Run(slug, func(t *testing.T) {
			d, err := BySlug(slug)
			require.NoError(t, err)
			assert.Equal(t, slug, d.Slug)
			assert.Equal(t, title, d.Title)
			assert.Equal(t, time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC), d.UpdatedAt)
			assert.Contains(t, d.HTML, "<h2>")
			assert.NotContains(t, d.HTML, "---")
			assert.NotContains(t, d.HTML, "altempl")
		})
	}
}

func TestBySlug_Unknown(t *testing.T) {
	_, err := BySlug("cookies")
	assert.True(t, IsUnknownDocumentError(err))
}

func TestRender_DropsRawHTML(t *testing.T) {
	d, err := parse("x", []byte("---\ntitle: X\n---\n\n<script>alert(1)</script>\n\nhello"))
	require.NoError(t, err)
	assert.NotContains(t, d.HTML, "<script>")
	assert.Contains(t, d.HTML, "hello")
}

func TestSplitFrontmatter(t *testing.T) {
	fm, body, err := splitFrontmatter([]byte("---\ntitle: \"Hi\"\nupdated: 2026-01-02\n---\nbody"))
	require.NoError(t, err)
	assert.Equal(t, "Hi", fm.Title)
	assert.Equal(t, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), fm.Updated)
	assert.Equal(t, "body", string(body))

	_, body, err = splitFrontmatter([]byte("no frontmatter"))
	require.NoError(t, err)
	assert.Equal(t, "no frontmatter", string(body))

	_, _, err = splitFrontmatter([]byte("---\ntitle: X\nnever closed"))
	assert.Error(t, err)
}

func TestUnknownDocumentError(t *testing.T) {
	err := &UnknownDocumentError{Slug: "cookies"}
	assert.Equal(t, `legal: unknown document "cookies"`, err.Error())
	assert.True(t, IsUnknownDocumentError(fmt.Errorf("wrapped: %w", err)))
	assert.False(t, IsUnknownDocumentError(errors.New("other")))
}

func TestParse_UnclosedFrontmatter(t *testing.T) {
	_, err := parse("terms", []byte("---\ntitle: X\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "legal: frontmatter terms")
}

func TestSplitFrontmatter_SkipsUnknownAndMalformedLines(t *testing.T) {
	fm, body, err := splitFrontmatter([]byte("---\nno colon here\nauthor: someone\nupdated: not-a-date\ntitle: 'T'\n---\nbody"))
	require.NoError(t, err)
	assert.Equal(t, "T", fm.Title)
	assert.True(t, fm.Updated.IsZero())
	assert.Equal(t, "body", string(body))
}
