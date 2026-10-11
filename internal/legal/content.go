package legal

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/yuin/goldmark"
)

//go:embed content/terms.md content/privacy.md
var contentFS embed.FS

var docs = sync.OnceValues(loadAll) //nolint:gochecknoglobals // rendered once per process.

// BySlug returns the document for slug.
func BySlug(slug string) (*Document, error) {
	m, err := docs()
	if err != nil {
		return nil, err
	}
	d, ok := m[slug]
	if !ok {
		return nil, &UnknownDocumentError{Slug: slug}
	}
	return d, nil
}

func loadAll() (map[string]*Document, error) {
	out := make(map[string]*Document, 2)
	for _, slug := range []string{TermsSlug, PrivacySlug} {
		raw, err := contentFS.ReadFile("content/" + slug + ".md")
		if err != nil {
			return nil, fmt.Errorf("legal: read %s: %w", slug, err)
		}
		d, err := parse(slug, raw)
		if err != nil {
			return nil, err
		}
		out[slug] = d
	}
	return out, nil
}

// SECURITY: goldmark's default renderer drops raw HTML, so markdown can never inject markup.
func parse(slug string, raw []byte) (*Document, error) {
	meta, body, err := splitFrontmatter(raw)
	if err != nil {
		return nil, fmt.Errorf("legal: frontmatter %s: %w", slug, err)
	}
	var buf bytes.Buffer
	if err := goldmark.Convert(body, &buf); err != nil {
		return nil, fmt.Errorf("legal: render %s: %w", slug, err)
	}
	return &Document{Slug: slug, Title: meta.Title, HTML: buf.String(), UpdatedAt: meta.Updated}, nil
}

type frontmatter struct {
	Title   string
	Updated time.Time
}

func splitFrontmatter(raw []byte) (frontmatter, []byte, error) {
	const delim = "---"
	trimmed := bytes.TrimLeft(raw, "\uFEFF\r\n\t ")
	if !bytes.HasPrefix(trimmed, []byte(delim)) {
		return frontmatter{}, raw, nil
	}
	head, body, ok := bytes.Cut(trimmed[len(delim):], []byte("\n"+delim))
	if !ok {
		return frontmatter{}, nil, errors.New("opening --- has no closing ---")
	}
	body = bytes.TrimLeft(body, "\r\n")

	var fm frontmatter
	for line := range strings.SplitSeq(strings.TrimSpace(string(head)), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		switch strings.TrimSpace(key) {
		case "title":
			fm.Title = value
		case "updated":
			if t, err := time.Parse(time.DateOnly, value); err == nil {
				fm.Updated = t
			}
		}
	}
	return fm, body, nil
}
