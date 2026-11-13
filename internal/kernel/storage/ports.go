package storage

import (
	"context"
	"io"
	"time"
)

// Driver is the object storage behind the feature: an S3-compatible bucket, or memory in tests.
type Driver interface {
	// PresignPut signs a PUT of exactly size bytes of contentType to key, valid for ttl.
	PresignPut(ctx context.Context, key, contentType string, size int64, ttl time.Duration) (Upload, error)
	// Head reports the object at key, or *ObjectNotFoundError.
	Head(ctx context.Context, key string) (Object, error)
	// ReadPrefix returns up to the first n bytes of the object at key, or *ObjectNotFoundError.
	ReadPrefix(ctx context.Context, key string, n int64) ([]byte, error)
	// Open streams the object at key; reading past maxBytes fails with *ObjectTooLargeError.
	Open(ctx context.Context, key string, maxBytes int64) (io.ReadCloser, error)
	// Delete removes the object at key; a missing object is not an error.
	Delete(ctx context.Context, key string) error
	// DeletePrefix removes every object under prefix; the zero Prefix fails with *InvalidPrefixError.
	DeletePrefix(ctx context.Context, prefix Prefix) error
	// PresignGet signs a GET of key, valid for ttl.
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
}
