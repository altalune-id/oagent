package legal

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
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

func latest(m map[string]*Document) time.Time {
	var out time.Time
	for _, d := range m {
		if d.UpdatedAt.After(out) {
			out = d.UpdatedAt
		}
	}
	return out
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
		return nil, fmt.Errorf("legal: content/%s.md: %w", slug, err)
	}
	if meta.Version == "" {
		return nil, fmt.Errorf("legal: content/%s.md: no updated: header; add updated: YYYY-MM-DD to the front matter", slug)
	}
	var buf bytes.Buffer
	if err := goldmark.Convert(body, &buf); err != nil {
		return nil, fmt.Errorf("legal: render %s: %w", slug, err)
	}
	sum := sha256.Sum256(raw)
	return &Document{
		Slug: slug, Title: meta.Title, Version: meta.Version, Source: string(body), SHA256: hex.EncodeToString(sum[:]),
		HTML: buf.String(), UpdatedAt: meta.Updated,
	}, nil
}

type frontmatter struct {
	Title   string
	Version string
	Updated time.Time
}

// NOTE: updated is also the version people accept; a second change on one day needs a date-time.
func parseUpdated(value string) (time.Time, bool) {
	if t, err := time.Parse(time.DateOnly, value); err == nil {
		return t, true
	}
	t, err := time.Parse(time.RFC3339, value)
	return t.UTC(), err == nil
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
			t, ok := parseUpdated(value)
			if !ok {
				return frontmatter{}, nil, fmt.Errorf("updated: %q is not a YYYY-MM-DD date or an RFC 3339 date-time", value)
			}
			fm.Version, fm.Updated = value, t
		}
	}
	return fm, body, nil
}
