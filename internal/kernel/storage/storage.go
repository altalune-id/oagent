// Package storage is the file storage feature: signed direct uploads, their verification, object keys and URLs.
package storage

import (
	"bytes"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Visibility decides who may read a file: anyone with its URL, or only a holder of a short-lived signed URL.
type Visibility string

// The visibilities; it is the first key segment, so one edge rule guards every private file.
const (
	Public  Visibility = "public"
	Private Visibility = "private"
)

// OwnerKind names the table a file's owner lives in; it is the third key segment.
type OwnerKind string

// The owner kinds.
const (
	OwnerOrgs     OwnerKind = "orgs"
	OwnerProjects OwnerKind = "projects"
	OwnerUsers    OwnerKind = "users"
)

// Status is where a file is in its life.
type Status string

// The file statuses: pending until verified, ready once attached, deleting until the sweep removes it.
const (
	StatusPending  Status = "pending"
	StatusReady    Status = "ready"
	StatusDeleting Status = "deleting"
)

// Purpose is one kind of file a domain accepts, declared once in code.
type Purpose struct {
	Name       string
	OwnerKind  OwnerKind
	Visibility Visibility
	Types      []string
	MaxBytes   int64
}

// Allows reports whether contentType is one of the purpose's types.
func (p Purpose) Allows(contentType string) bool { return slices.Contains(p.Types, contentType) }

// Owner is the project a file belongs to.
type Owner struct {
	OrgID    uuid.UUID
	ID       uuid.UUID
	PublicID string
}

// File is one stored object and what was declared about it; files belong to a project.
type File struct {
	ID          uuid.UUID
	PublicID    string
	OrgID       uuid.UUID
	ProjectID   uuid.UUID
	Purpose     string
	Visibility  Visibility
	ObjectKey   string
	ContentType string
	SizeBytes   int64
	Status      Status
	CreatedAt   time.Time
	CompletedAt *time.Time
	ExpiresAt   time.Time
}

// Upload is what a browser needs to PUT the file straight to storage.
type Upload struct {
	URL       string
	Method    string
	Headers   map[string]string
	ExpiresAt time.Time
}

// Object is what storage reports about a stored object.
type Object struct {
	Size        int64
	ContentType string
}

// Key builds the object key {visibility}/{app}/{owner kind}/{owner public id}/{purpose}/{file public id}.{ext}; nothing parses keys back.
func Key(app string, p Purpose, ownerPublicID, filePublicID, ext string) string {
	return strings.Join([]string{
		string(p.Visibility), app, string(p.OwnerKind), ownerPublicID, p.Name, filePublicID + "." + ext,
	}, "/")
}

//nolint:gochecknoglobals // read-only lookup table.
var extensions = map[string]string{
	"image/png":       "png",
	"image/jpeg":      "jpg",
	"image/webp":      "webp",
	"image/gif":       "gif",
	"application/pdf": "pdf",
	"text/csv":        "csv",
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": "docx",
	"text/markdown": "md",
	"text/plain":    "txt",
}

// Extension is the file extension of contentType, or "bin" when it has none on the list.
func Extension(contentType string) string {
	if ext, ok := extensions[contentType]; ok {
		return ext
	}
	return "bin"
}

// SniffLen is how many leading bytes Sniff reads.
const SniffLen = 16

// UploadMethod is the HTTP method every signed upload uses.
const UploadMethod = "PUT"

// Sniff reports whether the first SniffLen bytes fit contentType.
func Sniff(contentType string, head []byte) bool {
	switch contentType {
	case "image/png":
		return bytes.HasPrefix(head, []byte("\x89PNG\r\n\x1a\n"))
	case "image/jpeg":
		return bytes.HasPrefix(head, []byte{0xFF, 0xD8, 0xFF})
	case "image/webp":
		return len(head) >= 12 && bytes.Equal(head[:4], []byte("RIFF")) && bytes.Equal(head[8:12], []byte("WEBP"))
	case "image/gif":
		return bytes.HasPrefix(head, []byte("GIF87a")) || bytes.HasPrefix(head, []byte("GIF89a"))
	case "application/pdf":
		return bytes.HasPrefix(head, []byte("%PDF-"))
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return bytes.HasPrefix(head, []byte("PK\x03\x04"))
	case "text/plain", "text/markdown", "text/csv":
		return !bytes.Contains(head, []byte{0})
	}
	return true
}
