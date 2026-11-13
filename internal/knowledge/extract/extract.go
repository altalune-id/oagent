// Package extract turns an uploaded or crawled document into text sections with headings and page numbers.
package extract

import (
	"bytes"
	"context"
	"strings"
	"unicode/utf8"
)

// MaxTextBytes caps one document's text; a builder stops collecting just past it.
const MaxTextBytes = 2 << 20

// Kind is the document format.
type Kind string

// The supported formats.
const (
	KindPDF      Kind = "pdf"
	KindDOCX     Kind = "docx"
	KindHTML     Kind = "html"
	KindMarkdown Kind = "markdown"
	KindText     Kind = "text"
)

// Section is a run of text under one heading, on one page when the format has pages.
type Section struct {
	Page    int    `json:"page,omitempty"`
	Heading string `json:"heading,omitempty"`
	Text    string `json:"text"`
}

// Extract reads a non-PDF document; the server runs it only inside the extract child, and PDFs only through Serve.
func Extract(ctx context.Context, kind Kind, data []byte) ([]Section, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var (
		sections []Section
		err      error
	)
	switch kind {
	case KindDOCX:
		sections, err = extractDOCX(data)
	case KindHTML:
		sections, err = extractHTML(data)
	case KindMarkdown:
		sections, err = extractMarkdown(data)
	case KindText:
		sections, err = extractText(data)
	default:
		return nil, &RejectedError{Reason: UnsupportedType}
	}
	if err != nil {
		return nil, err
	}
	return nonEmpty(sections)
}

// NOTE: Postgres text cannot hold U+0000, so every format drops it here, the PDF child included.
func nonEmpty(sections []Section) ([]Section, error) {
	found := false
	for i := range sections {
		s := &sections[i]
		s.Text, s.Heading = strings.ReplaceAll(s.Text, "\x00", ""), strings.ReplaceAll(s.Heading, "\x00", "")
		found = found || s.Text != "" || s.Heading != ""
	}
	if !found {
		return nil, &NoTextError{}
	}
	return sections, nil
}

type builder struct {
	sections []Section
	lines    []string
	heading  string
	started  bool
	size     int
}

func (b *builder) flush() {
	if b.started || len(b.lines) > 0 {
		b.sections = append(b.sections, Section{Heading: b.heading, Text: tidy(strings.Join(b.lines, "\n"))})
	}
	b.lines = nil
}

func (b *builder) addHeading(h string) {
	b.flush()
	b.heading = strings.TrimSpace(h)
	b.started = true
}

func (b *builder) addText(t string) {
	if b.size > MaxTextBytes {
		return
	}
	b.lines = append(b.lines, t)
	b.size += len(strings.TrimSpace(t))
}

func (b *builder) done() []Section {
	b.flush()
	return b.sections
}

func tidy(text string) string {
	lines := strings.Split(text, "\n")
	out := lines[:0]
	for _, l := range lines {
		l = strings.TrimRight(l, " \t\r")
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

func utf8Text(data []byte) (string, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if !utf8.Valid(data) {
		return "", &RejectedError{Reason: InvalidUTF8}
	}
	return string(data), nil
}

func extractText(data []byte) ([]Section, error) {
	text, err := utf8Text(data)
	if err != nil {
		return nil, err
	}
	return []Section{{Text: tidy(text)}}, nil
}
