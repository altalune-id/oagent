// Package assets serves embedded files with strong ETags and their build-time brotli and gzip siblings.
package assets

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"
)

const minCompressSize = 1024

// Files maps a slash-separated path to its asset; compressed siblings are variants, never entries.
type Files map[string]*Asset

// Asset is one file held in memory with its content type and any compressed variants.
type Asset struct {
	name  string
	ctype string
	plain variant
	br    *variant
	gzip  *variant
}

type variant struct {
	encoding string
	etag     string
	data     []byte
}

// Load reads every file of fsys once, computing each ETag and pairing it with its .br and .gz siblings.
func Load(fsys fs.FS) (Files, error) {
	files := Files{}
	var encoded []variant
	var bases []string
	err := fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		ext := path.Ext(name)
		if enc := siblingEncoding(ext); enc != "" {
			encoded = append(encoded, newVariant(enc, b))
			bases = append(bases, strings.TrimSuffix(name, ext))
			return nil
		}
		files[name] = &Asset{name: name, ctype: mime.TypeByExtension(ext), plain: newVariant("", b)}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for i, v := range encoded {
		a, ok := files[bases[i]]
		if !ok || a.ctype == "" {
			continue
		}
		if v.encoding == "br" {
			a.br = &v
		} else {
			a.gzip = &v
		}
	}
	return files, nil
}

// New builds an asset from bytes, adding an in-memory gzip variant when b is over 1 KiB and gzip shrinks it.
func New(name string, b []byte) *Asset {
	a := &Asset{name: name, ctype: mime.TypeByExtension(path.Ext(name)), plain: newVariant("", b)}
	if len(b) <= minCompressSize {
		return a
	}
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	_, _ = zw.Write(b)
	_ = zw.Close()
	if buf.Len() < len(b) {
		gz := newVariant("gzip", buf.Bytes())
		a.gzip = &gz
	}
	return a
}

func siblingEncoding(ext string) string {
	switch ext {
	case ".br":
		return "br"
	case ".gz":
		return "gzip"
	}
	return ""
}

func newVariant(encoding string, b []byte) variant {
	sum := sha256.Sum256(b)
	return variant{encoding: encoding, etag: `"` + hex.EncodeToString(sum[:16]) + `"`, data: b}
}

// Sibling reports whether name is the .br or .gz sibling of a file in f.
func (f Files) Sibling(name string) bool {
	ext := path.Ext(name)
	if siblingEncoding(ext) == "" {
		return false
	}
	_, ok := f[strings.TrimSuffix(name, ext)]
	return ok
}

// Handler serves f by URL path with cacheControl, answering 404 for anything that is not a file in f.
func (f Files) Handler(cacheControl string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a, ok := f[strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		a.Serve(w, r, cacheControl)
	})
}

// Serve writes a with cacheControl and its ETag, as brotli or gzip when the request accepts it.
func (a *Asset) Serve(w http.ResponseWriter, r *http.Request, cacheControl string) {
	v := a.pick(r)
	h := w.Header()
	if a.br != nil || a.gzip != nil {
		h.Add("Vary", "Accept-Encoding")
	}
	h.Set("Cache-Control", cacheControl)
	h.Set("ETag", v.etag)
	if a.ctype != "" {
		h.Set("Content-Type", a.ctype)
	}
	if v.encoding != "" {
		h.Set("Content-Encoding", v.encoding)
		h.Set("Content-Length", strconv.Itoa(len(v.data)))
	}
	http.ServeContent(w, r, a.name, time.Time{}, bytes.NewReader(v.data))
}

func (a *Asset) pick(r *http.Request) variant {
	ae := r.Header.Get("Accept-Encoding")
	if ae == "" || r.Header.Get("Range") != "" {
		return a.plain
	}
	if a.br != nil && accepts(ae, "br") {
		return *a.br
	}
	if a.gzip != nil && accepts(ae, "gzip") {
		return *a.gzip
	}
	return a.plain
}

func accepts(header, coding string) bool {
	star := 0.0
	for part := range strings.SplitSeq(header, ",") {
		name, params, _ := strings.Cut(part, ";")
		name = strings.ToLower(strings.TrimSpace(name))
		switch name {
		case coding:
			return qvalue(params) > 0
		case "*":
			star = qvalue(params)
		}
	}
	return star > 0
}

func qvalue(params string) float64 {
	for p := range strings.SplitSeq(params, ";") {
		k, v, ok := strings.Cut(p, "=")
		if ok && strings.EqualFold(strings.TrimSpace(k), "q") {
			q, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
			if err != nil {
				return 0
			}
			return q
		}
	}
	return 1
}
