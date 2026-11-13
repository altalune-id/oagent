// Package chunk splits extracted sections into overlapping chunks sized for embedding.
package chunk

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"altalune.id/oagent/internal/knowledge/extract"
)

// The chunk sizes, at 4 characters per token: about 800 tokens per chunk and 100 tokens of overlap.
const (
	TargetChars  = 800 * 4
	OverlapChars = 100 * 4
)

// The per-document caps.
const (
	MaxTextBytes = extract.MaxTextBytes
	MaxChunks    = 2000
)

// Doc is one document's sections with the title and URL every chunk carries.
type Doc struct {
	Title    string
	URL      string
	Sections []extract.Section
}

// Chunk is one embeddable piece of a document.
type Chunk struct {
	Ordinal     int    `json:"ordinal"`
	HeadingPath string `json:"heading_path,omitempty"`
	Page        int    `json:"page,omitempty"`
	URL         string `json:"url,omitempty"`
	Text        string `json:"text"`
	EmbedText   string `json:"embed_text"`
}

// Limit names the cap a LimitError hit.
type Limit string

// The caps a LimitError can name.
const (
	LimitTextBytes Limit = "text_bytes"
	LimitChunks    Limit = "chunks"
)

// LimitError reports a document over a per-document cap.
type LimitError struct {
	Limit Limit
	Max   int
}

func (e *LimitError) Error() string {
	return "chunk: document over the " + string(e.Limit) + " cap of " + strconv.Itoa(e.Max)
}

// AsLimitError returns the *LimitError in err's tree, if any.
func AsLimitError(err error) (*LimitError, bool) {
	return errors.AsType[*LimitError](err)
}

type section struct {
	page    int
	heading string
	paras   []string
}

// Split normalizes doc and splits it by section, then paragraph (one line), then sentence; chunks never cross a section.
func Split(doc Doc) ([]Chunk, error) {
	title := oneLine(doc.Title)
	sections := make([]section, 0, len(doc.Sections))
	total := 0
	for _, s := range doc.Sections {
		ns := section{page: s.Page, heading: oneLine(s.Heading), paras: paragraphs(s.Text)}
		for _, p := range ns.paras {
			total += len(p)
		}
		if total > MaxTextBytes {
			return nil, &LimitError{Limit: LimitTextBytes, Max: MaxTextBytes}
		}
		sections = append(sections, ns)
	}
	var out []Chunk
	for _, s := range sections {
		for _, text := range pack(pieces(s.paras)) {
			if len(out) == MaxChunks {
				return nil, &LimitError{Limit: LimitChunks, Max: MaxChunks}
			}
			out = append(out, Chunk{
				Ordinal: len(out), HeadingPath: s.heading, Page: s.page, URL: doc.URL,
				Text: text, EmbedText: EmbedText(title, s.heading, text),
			})
		}
	}
	return out, nil
}

// EmbedText is the text a chunk is embedded as: its document title and heading, one line each, then the chunk text.
func EmbedText(title, heading, text string) string {
	title, heading = oneLine(title), oneLine(heading)
	label := title
	switch {
	case label == "":
		label = heading
	case heading != "":
		label += " — " + heading
	}
	if label == "" {
		return text
	}
	return label + "\n\n" + text
}

func oneLine(s string) string { return strings.Join(strings.Fields(norm.NFKC.String(s)), " ") }

func paragraphs(text string) []string {
	var out []string
	for line := range strings.SplitSeq(norm.NFKC.String(text), "\n") {
		if p := strings.Join(strings.Fields(line), " "); p != "" {
			out = append(out, p)
		}
	}
	return out
}

type piece struct {
	text  string
	runes int
	para  bool
}

var sentenceEnd = regexp.MustCompile(`[.!?。！？]["'”’)\]]*\s+`)

func pieces(paras []string) []piece {
	var out []piece
	for _, p := range paras {
		start := len(out)
		if n := utf8.RuneCountInString(p); n <= TargetChars {
			out = append(out, piece{text: p, runes: n})
		} else {
			for _, s := range sentences(p) {
				out = appendSentence(out, s)
			}
		}
		out[start].para = true
	}
	return out
}

func sentences(p string) []string {
	var out []string
	last := 0
	for _, m := range sentenceEnd.FindAllStringIndex(p, -1) {
		out = append(out, strings.TrimSpace(p[last:m[1]]))
		last = m[1]
	}
	if rest := strings.TrimSpace(p[last:]); rest != "" {
		out = append(out, rest)
	}
	return out
}

func appendSentence(out []piece, s string) []piece {
	if n := utf8.RuneCountInString(s); n <= TargetChars {
		return append(out, piece{text: s, runes: n})
	}
	for w := range strings.FieldsSeq(s) {
		for w != "" {
			cut := len(w)
			if utf8.RuneCountInString(w) > TargetChars {
				cut = runeOffset(w, TargetChars)
			}
			out = append(out, piece{text: w[:cut], runes: utf8.RuneCountInString(w[:cut])})
			w = w[cut:]
		}
	}
	return out
}

func runeOffset(s string, n int) int {
	i := 0
	for range n {
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
	}
	return i
}

func joiner(p piece) string {
	if p.para {
		return "\n"
	}
	return " "
}

func pack(ps []piece) []string {
	var (
		out   []string
		cur   []piece
		size  int
		fresh int
	)
	width := func(i int) int {
		if i == 0 {
			return cur[0].runes
		}
		return 1 + cur[i].runes
	}
	emit := func() {
		var b strings.Builder
		for i, p := range cur {
			if i > 0 {
				b.WriteString(joiner(p))
			}
			b.WriteString(p.text)
		}
		out = append(out, b.String())
		keep, kept := len(cur), 0
		for keep > 1 && kept+cur[keep-1].runes+1 <= OverlapChars {
			keep--
			kept += cur[keep].runes + 1
		}
		cur = append([]piece(nil), cur[keep:]...)
		size, fresh = 0, 0
		for i := range cur {
			size += width(i)
		}
	}
	for _, p := range ps {
		if fresh > 0 && size+1+p.runes > TargetChars {
			emit()
		}
		for len(cur) > 0 && size+1+p.runes > TargetChars {
			size -= cur[0].runes
			cur = cur[1:]
			if len(cur) > 0 {
				size--
			}
		}
		cur = append(cur, p)
		size += width(len(cur) - 1)
		fresh++
	}
	if fresh > 0 {
		emit()
	}
	return out
}
