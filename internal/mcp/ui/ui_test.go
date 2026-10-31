package ui

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
)

func TestDocument(t *testing.T) {
	doc := Document()

	assert.Contains(t, doc, "<!doctype html>")
	assert.Contains(t, strings.ToLower(doc), "oagent")
	assert.Equal(t, "ui://oagent/app", ResourceURI)
}

func TestDocument_FallbackWhenUnbuilt(t *testing.T) {
	doc := document(fstest.MapFS{})

	assert.Contains(t, doc, "<title>OAgent</title>")
	assert.Contains(t, doc, "not built")
}

func TestDocument_ReadsEmbedded(t *testing.T) {
	fsys := fstest.MapFS{"assets/app.html": &fstest.MapFile{Data: []byte("<!doctype html>embedded")}}
	assert.Equal(t, "<!doctype html>embedded", document(fsys))
}
