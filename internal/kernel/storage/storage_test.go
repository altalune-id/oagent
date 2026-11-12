package storage_test

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/storage"
)

func TestKey_BuildsTheObjectPath(t *testing.T) {
	p := storage.Purpose{Name: "knowledge", OwnerKind: storage.OwnerProjects, Visibility: storage.Private}
	assert.Equal(t, "private/oagent/projects/Pj1/knowledge/F1.pdf", storage.Key("oagent", p, "Pj1", "F1", "pdf"))
}

func TestExtension_KnowsTheKnowledgeTypes(t *testing.T) {
	for ct, ext := range map[string]string{
		"application/pdf": "pdf",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document": "docx",
		"text/markdown":         "md",
		"text/plain":            "txt",
		"image/png":             "png",
		"application/x-unknown": "bin",
	} {
		assert.Equal(t, ext, storage.Extension(ct), ct)
	}
}

func TestSniff_SixteenBytePrefix(t *testing.T) {
	docx := "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	assert.True(t, storage.Sniff("application/pdf", []byte("%PDF-1.4\n")))
	assert.False(t, storage.Sniff("application/pdf", []byte("PK\x03\x04")))
	assert.True(t, storage.Sniff(docx, []byte("PK\x03\x04\x14\x00")))
	assert.False(t, storage.Sniff(docx, []byte("%PDF-")))
	assert.True(t, storage.Sniff("text/markdown", []byte("# Title\n")))
	assert.False(t, storage.Sniff("text/plain", []byte("abc\x00def")), "text has no NUL byte")
	assert.True(t, storage.Sniff("image/png", []byte("\x89PNG\r\n\x1a\n")))
}

func TestStorage_MemoryRefusedInProduction(t *testing.T) {
	assert.True(t, storage.IsMemoryInProductionError(storage.Config{Driver: storage.DriverMemory, Production: true}.Check()))
	assert.NoError(t, storage.Config{Driver: storage.DriverMemory}.Check())
	assert.NoError(t, storage.Config{Driver: storage.DriverS3, Production: true}.Check())
}

func TestConfig_NoDriverMeansUploadsOff(t *testing.T) {
	cfg := storage.Config{}
	assert.False(t, cfg.Enabled())
	assert.True(t, storage.IsNotConfiguredError(cfg.Check()))
	assert.Error(t, storage.Config{Driver: "ftp"}.Check())
}

func TestCapReader_StopsPastTheCap(t *testing.T) {
	rc := storage.CapReader(io.NopCloser(strings.NewReader("abcdef")), 4)
	b, err := io.ReadAll(rc)
	assert.Equal(t, "abcd", string(b))
	assert.True(t, storage.IsObjectTooLargeError(err), "got %v", err)
	require.NoError(t, rc.Close())

	exact := storage.CapReader(io.NopCloser(strings.NewReader("abcd")), 4)
	b, err = io.ReadAll(exact)
	require.NoError(t, err)
	assert.Equal(t, "abcd", string(b))
}

func TestErrors_SayWhatHappened(t *testing.T) {
	for err, want := range map[error]string{
		&storage.NotConfiguredError{}:             "not configured",
		&storage.MemoryInProductionError{}:        "not production",
		&storage.UnknownDriverError{Driver: "x"}:  `unknown driver "x"`,
		&storage.ObjectNotFoundError{Key: "k"}:    "no object at k",
		&storage.ObjectTooLargeError{MaxBytes: 9}: "larger than 9 bytes",
	} {
		assert.Contains(t, err.Error(), want)
	}
	assert.Equal(t, "bin", storage.Extension("application/octet-stream"))
	assert.True(t, storage.Purpose{Types: []string{"text/plain"}}.Allows("text/plain"))
}

func TestOwnerPrefix_BuildsTheOwnersFolder(t *testing.T) {
	purpose := storage.Purpose{Name: "knowledge", OwnerKind: storage.OwnerProjects, Visibility: storage.Private}
	p, err := storage.OwnerPrefix("oagent", purpose, "Pj1")
	require.NoError(t, err)
	assert.Equal(t, "private/oagent/projects/Pj1/knowledge/", p.String())
}

func TestOwnerPrefix_CannotReachAboveOneOwner(t *testing.T) {
	knowledge := storage.Purpose{Name: "knowledge", OwnerKind: storage.OwnerProjects, Visibility: storage.Private}
	for name, build := range map[string]func() (storage.Prefix, error){
		"private/ (no app, owner kind, owner or purpose)": func() (storage.Prefix, error) {
			return storage.OwnerPrefix("", storage.Purpose{Visibility: storage.Private}, "")
		},
		"private/oagent/ (no owner kind, owner or purpose)": func() (storage.Prefix, error) {
			return storage.OwnerPrefix("oagent", storage.Purpose{Visibility: storage.Private}, "")
		},
		"no app":   func() (storage.Prefix, error) { return storage.OwnerPrefix("", knowledge, "Pj1") },
		"no owner": func() (storage.Prefix, error) { return storage.OwnerPrefix("oagent", knowledge, "") },
		"no purpose name": func() (storage.Prefix, error) {
			return storage.OwnerPrefix("oagent", storage.Purpose{OwnerKind: storage.OwnerProjects, Visibility: storage.Private}, "Pj1")
		},
		"no visibility": func() (storage.Prefix, error) {
			return storage.OwnerPrefix("oagent", storage.Purpose{Name: "k", OwnerKind: storage.OwnerProjects}, "Pj1")
		},
		"no owner kind": func() (storage.Prefix, error) {
			return storage.OwnerPrefix("oagent", storage.Purpose{Name: "k", Visibility: storage.Private}, "Pj1")
		},
		"slash in owner": func() (storage.Prefix, error) { return storage.OwnerPrefix("oagent", knowledge, "a/b") },
		"dot-dot owner":  func() (storage.Prefix, error) { return storage.OwnerPrefix("oagent", knowledge, "..") },
		"slash in app":   func() (storage.Prefix, error) { return storage.OwnerPrefix("oagent/x", knowledge, "Pj1") },
	} {
		p, err := build()
		assert.True(t, storage.IsInvalidPrefixError(err), "%s: %v", name, err)
		assert.True(t, p.IsZero(), name)
	}
}

func TestPrefix_SubGoesOnlyDeeper(t *testing.T) {
	purpose := storage.Purpose{Name: "knowledge", OwnerKind: storage.OwnerProjects, Visibility: storage.Private}
	owner, err := storage.OwnerPrefix("oagent", purpose, "Pj1")
	require.NoError(t, err)
	doc, err := owner.Sub("Dc1")
	require.NoError(t, err)
	assert.Equal(t, "private/oagent/projects/Pj1/knowledge/Dc1/", doc.String())
	for _, seg := range []string{"", ".", "..", "a/b", "a/"} {
		p, err := owner.Sub(seg)
		assert.True(t, storage.IsInvalidPrefixError(err), "%q", seg)
		assert.True(t, p.IsZero())
	}
	_, err = storage.Prefix{}.Sub("x")
	assert.True(t, storage.IsInvalidPrefixError(err), "the zero prefix has no children")
	assert.True(t, storage.Prefix{}.IsZero())
}
