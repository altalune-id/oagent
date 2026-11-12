package extract

import (
	"bytes"
	"strings"

	"github.com/ledongthuc/pdf"
)

// PDF is the PDFReader on github.com/ledongthuc/pdf; it runs only inside the child process, which contains its panics and loops.
type PDF struct{}

// PageCount reads the page tree only.
func (PDF) PageCount(data []byte) (int, error) {
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return 0, err
	}
	return r.NumPage(), nil
}

// Pages returns one section per page with text.
func (PDF) Pages(data []byte) ([]Section, error) {
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	var out []Section
	for i := 1; i <= r.NumPage(); i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			return nil, err
		}
		if text = strings.TrimSpace(text); text != "" {
			out = append(out, Section{Page: i, Text: text})
		}
	}
	return out, nil
}
