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
			assert.Equal(t, "2026-10-11", d.Version)
			assert.Len(t, d.SHA256, 64)
			assert.Contains(t, d.Source, "## 1.", "the markdown source is kept for the snapshot")
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
	d, err := parse("x", []byte("---\ntitle: X\nupdated: 2026-01-02\n---\n\n<script>alert(1)</script>\n\nhello"))
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
	assert.Contains(t, err.Error(), "legal: content/terms.md")
}

func TestParse_NeedsAValidUpdatedHeader(t *testing.T) {
	for name, raw := range map[string]string{
		"missing":        "---\ntitle: X\n---\nbody",
		"invalid":        "---\ntitle: X\nupdated: soon\n---\nbody",
		"no frontmatter": "body",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parse("terms", []byte(raw))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "content/terms.md")
			assert.Contains(t, err.Error(), "updated:")
		})
	}
}

func TestSplitFrontmatter_SkipsUnknownAndMalformedLines(t *testing.T) {
	fm, body, err := splitFrontmatter([]byte("---\nno colon here\nauthor: someone\ntitle: 'T'\n---\nbody"))
	require.NoError(t, err)
	assert.Equal(t, "T", fm.Title)
	assert.True(t, fm.Updated.IsZero())
	assert.Equal(t, "body", string(body))
}

func TestService_TermsUpdatedAt(t *testing.T) {
	builtIn := NewService(nil).TermsUpdatedAt()
	terms, err := BySlug(TermsSlug)
	require.NoError(t, err)
	privacy, err := BySlug(PrivacySlug)
	require.NoError(t, err)
	want := terms.UpdatedAt
	if privacy.UpdatedAt.After(want) {
		want = privacy.UpdatedAt
	}
	assert.Equal(t, want, builtIn, "the newer of the two documents")
	assert.False(t, builtIn.IsZero())

	external := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, external, NewService(nil).WithExternalTerms(external).TermsUpdatedAt(), "the configured date wins for external terms")
	assert.True(t, NewService(nil).WithExternalTerms(time.Time{}).TermsUpdatedAt().IsZero(), "external terms without a date are never re-asked")
}

func TestLatest(t *testing.T) {
	a := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	b := a.AddDate(0, 1, 0)
	assert.Equal(t, b, latest(map[string]*Document{"x": {UpdatedAt: a}, "y": {UpdatedAt: b}}))
	assert.True(t, latest(nil).IsZero())
}

// SECURITY: people accept a version; its text must never change under them. Add a line here for every new version.
var publishedVersions = map[string]map[string]string{
	TermsSlug:   {"2026-10-11": "21bb08fd057d909615a76373668017d1198e20a4a3985fbfe51bc0a1adc03084"},
	PrivacySlug: {"2026-10-11": "e497fd4115918228d7a205c38ff633568f693c4a7cbe922f1275301bc8ec2b60"},
}

func TestPublishedVersionsAreFrozen(t *testing.T) {
	for _, slug := range []string{TermsSlug, PrivacySlug} {
		d, err := BySlug(slug)
		require.NoError(t, err)
		want, known := publishedVersions[slug][d.Version]
		require.True(t, known, "%s: version %q is new; add it with its SHA-256 %s to publishedVersions", slug, d.Version, d.SHA256)
		assert.Equal(t, want, d.SHA256, "%s: the text of version %q changed; set a new updated: in content/%s.md instead", slug, d.Version, slug)
	}
}

func TestSplitFrontmatter_UpdatedIsTheVersion(t *testing.T) {
	fm, _, err := splitFrontmatter([]byte("---\nupdated: 2026-10-11\n---\nbody"))
	require.NoError(t, err)
	assert.Equal(t, "2026-10-11", fm.Version)

	fm, _, err = splitFrontmatter([]byte("---\nupdated: 2026-10-11T15:00:00Z\n---\nbody"))
	require.NoError(t, err)
	assert.Equal(t, "2026-10-11T15:00:00Z", fm.Version, "a second change on one day")
	assert.Equal(t, time.Date(2026, 10, 11, 15, 0, 0, 0, time.UTC), fm.Updated)

	_, _, err = splitFrontmatter([]byte("---\nupdated: soon\n---\nbody"))
	assert.ErrorContains(t, err, `updated: "soon"`)
}
