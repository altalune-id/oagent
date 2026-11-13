package extract

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"encoding/xml"
	"errors"
	"io"
	"regexp"
	"strings"
)

// MaxDocxEntries is the most zip entries a DOCX may hold.
const MaxDocxEntries = 1000

// MaxDocxPartBytes caps the decompressed bytes read from one DOCX part.
const MaxDocxPartBytes = 10 << 20

const wordNS = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"

var errPartTooLarge = errors.New("extract: docx part over the decompressed cap")

var headingStyleID = regexp.MustCompile(`(?i)^(heading\d|title)$`)

type cappedReader struct {
	r    io.Reader
	left int64
}

func (c *cappedReader) Read(p []byte) (int, error) {
	if c.left <= 0 {
		var probe [1]byte
		if n, _ := c.r.Read(probe[:]); n > 0 {
			return 0, errPartTooLarge
		}
		return 0, io.EOF
	}
	if int64(len(p)) > c.left {
		p = p[:c.left]
	}
	n, err := c.r.Read(p)
	c.left -= int64(n)
	return n, err
}

func openPart(f *zip.File) (io.Reader, error) {
	raw, err := f.OpenRaw()
	if err != nil {
		return nil, &UnreadableError{Reason: Corrupt, Cause: err}
	}
	switch f.Method {
	case zip.Store:
		return &cappedReader{r: raw, left: MaxDocxPartBytes}, nil
	case zip.Deflate:
		return &cappedReader{r: flate.NewReader(raw), left: MaxDocxPartBytes}, nil
	default:
		return nil, &UnreadableError{Reason: Corrupt, Cause: errors.New("unsupported zip compression")}
	}
}

func partError(err error) error {
	if errors.Is(err, errPartTooLarge) {
		return &RejectedError{Reason: TooLarge}
	}
	return &UnreadableError{Reason: Corrupt, Cause: err}
}

func extractDOCX(data []byte) ([]Section, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, &UnreadableError{Reason: Corrupt, Cause: err}
	}
	if len(zr.File) > MaxDocxEntries {
		return nil, &RejectedError{Reason: TooManyEntries}
	}
	var doc, styles *zip.File
	for _, f := range zr.File {
		switch f.Name {
		case "word/document.xml":
			doc = f
		case "word/styles.xml":
			styles = f
		}
	}
	if doc == nil {
		return nil, &RejectedError{Reason: MissingDocumentXML}
	}
	names := map[string]string{}
	if styles != nil {
		if names, err = styleNames(styles); err != nil {
			return nil, err
		}
	}
	r, err := openPart(doc)
	if err != nil {
		return nil, err
	}
	return paragraphs(r, func(id string) bool {
		name := strings.ToLower(names[id])
		return strings.HasPrefix(name, "heading ") || name == "title" || (name == "" && headingStyleID.MatchString(id))
	})
}

func styleNames(f *zip.File) (map[string]string, error) {
	r, err := openPart(f)
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	dec := xml.NewDecoder(r)
	current := ""
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return names, nil
		}
		if err != nil {
			return nil, partError(err)
		}
		el, ok := tok.(xml.StartElement)
		if !ok || el.Name.Space != wordNS {
			continue
		}
		switch el.Name.Local {
		case "style":
			current = attr(el, "styleId")
		case "name":
			if current != "" {
				names[current] = attr(el, "val")
			}
		}
	}
}

func attr(el xml.StartElement, local string) string {
	for _, a := range el.Attr {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

func paragraphs(r io.Reader, isHeading func(styleID string) bool) ([]Section, error) {
	var (
		b     builder
		para  strings.Builder
		style string
		inT   bool
	)
	dec := xml.NewDecoder(r)
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return b.done(), nil
		}
		if err != nil {
			return nil, partError(err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Space != wordNS {
				continue
			}
			switch t.Name.Local {
			case "p":
				para.Reset()
				style = ""
			case "pStyle":
				style = attr(t, "val")
			case "t":
				inT = true
			case "tab":
				para.WriteByte('\t')
			case "br", "cr":
				para.WriteByte('\n')
			}
		case xml.EndElement:
			if t.Name.Space != wordNS {
				continue
			}
			switch t.Name.Local {
			case "t":
				inT = false
			case "p":
				if style != "" && isHeading(style) {
					b.addHeading(para.String())
				} else {
					b.addText(para.String())
				}
			}
		case xml.CharData:
			if inT {
				para.Write(t)
			}
		}
	}
}
