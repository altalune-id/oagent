package ui

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
)

func TestDocument(t *testing.T) {
	doc := Document()

	assert.Contains(t, doc, "<!doctype html>")
	assert.Contains(t, doc, "oagent")
	assert.Equal(t, "ui://oagent/app", ResourceURI)
}

func TestDocument_FallbackWhenUnbuilt(t *testing.T) {
	assert.Contains(t, document(fstest.MapFS{}), "not built")
}

func TestDocument_ReadsEmbedded(t *testing.T) {
	fsys := fstest.MapFS{"assets/app.html": &fstest.MapFile{Data: []byte("<!doctype html>embedded")}}
	assert.Equal(t, "<!doctype html>embedded", document(fsys))
}
