package extract

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func dropped(a atom.Atom) bool {
	switch a {
	case atom.Head, atom.Script, atom.Style, atom.Noscript, atom.Template, atom.Nav, atom.Header, atom.Footer, atom.Aside:
		return true
	}
	return false
}

func isHeading(a atom.Atom) bool {
	switch a {
	case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
		return true
	}
	return false
}

func isBlock(a atom.Atom) bool {
	switch a {
	case atom.P, atom.Div, atom.Br, atom.Li, atom.Tr, atom.Section, atom.Article, atom.Main,
		atom.Blockquote, atom.Pre, atom.Table, atom.Ul, atom.Ol:
		return true
	}
	return false
}

type htmlWalker struct {
	b    builder
	line strings.Builder
}

func (w *htmlWalker) endLine() {
	if text := strings.Join(strings.Fields(w.line.String()), " "); text != "" {
		w.b.addText(text)
	}
	w.line.Reset()
}

func (w *htmlWalker) walk(n *html.Node) {
	if n.Type == html.ElementNode && dropped(n.DataAtom) {
		return
	}
	if n.Type == html.ElementNode && isHeading(n.DataAtom) {
		w.endLine()
		w.b.addHeading(strings.Join(strings.Fields(textContent(n)), " "))
		return
	}
	if n.Type == html.TextNode {
		w.line.WriteString(n.Data)
		w.line.WriteByte(' ')
	}
	block := n.Type == html.ElementNode && isBlock(n.DataAtom)
	if block {
		w.endLine()
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		w.walk(c)
	}
	if block {
		w.endLine()
	}
}

func textContent(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data + " "
	}
	var sb strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && dropped(c.DataAtom) {
			continue
		}
		sb.WriteString(textContent(c))
	}
	return sb.String()
}

func extractHTML(data []byte) ([]Section, error) {
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, &UnreadableError{Reason: Corrupt, Cause: err}
	}
	var w htmlWalker
	w.walk(doc)
	w.endLine()
	return w.b.done(), nil
}
