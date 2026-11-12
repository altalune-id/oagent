// Package memory is an in-process storage driver for tests: presigned URLs are fake and Put stands in for the browser.
package memory

import (
	"bytes"
	"context"
	"io"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"altalune.id/oagent/internal/kernel/storage"
)

var _ storage.Driver = (*Driver)(nil)

type object struct {
	body        []byte
	contentType string
}

// Driver keeps objects in a map.
type Driver struct {
	mu      sync.Mutex
	objects map[string]object
	now     func() time.Time
}

// New returns an empty Driver.
func New() *Driver { return &Driver{objects: map[string]object{}, now: time.Now} }

// PresignPut implements storage.Driver with a memory:// URL that only Put understands.
func (d *Driver) PresignPut(_ context.Context, key, contentType string, size int64, ttl time.Duration) (storage.Upload, error) {
	return storage.Upload{
		URL:    "memory://upload/" + url.PathEscape(key),
		Method: storage.UploadMethod,
		Headers: map[string]string{
			"Content-Type":   contentType,
			"Content-Length": strconv.FormatInt(size, 10),
		},
		ExpiresAt: d.now().Add(ttl).UTC(),
	}, nil
}

// Put stores body under key as if a browser had uploaded it.
func (d *Driver) Put(key, contentType string, body []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.objects[key] = object{body: append([]byte(nil), body...), contentType: contentType}
}

// Has reports whether an object is stored under key.
func (d *Driver) Has(key string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, ok := d.objects[key]
	return ok
}

// Head implements storage.Driver.
func (d *Driver) Head(_ context.Context, key string) (storage.Object, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	o, ok := d.objects[key]
	if !ok {
		return storage.Object{}, &storage.ObjectNotFoundError{Key: key}
	}
	return storage.Object{Size: int64(len(o.body)), ContentType: o.contentType}, nil
}

// ReadPrefix implements storage.Driver.
func (d *Driver) ReadPrefix(_ context.Context, key string, n int64) ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	o, ok := d.objects[key]
	if !ok {
		return nil, &storage.ObjectNotFoundError{Key: key}
	}
	return append([]byte(nil), o.body[:min(n, int64(len(o.body)))]...), nil
}

// Open implements storage.Driver.
func (d *Driver) Open(_ context.Context, key string, maxBytes int64) (io.ReadCloser, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	o, ok := d.objects[key]
	if !ok {
		return nil, &storage.ObjectNotFoundError{Key: key}
	}
	return storage.CapReader(io.NopCloser(bytes.NewReader(append([]byte(nil), o.body...))), maxBytes), nil
}

// DeletePrefix implements storage.Driver.
func (d *Driver) DeletePrefix(_ context.Context, prefix storage.Prefix) error {
	if prefix.IsZero() {
		return &storage.InvalidPrefixError{}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for key := range d.objects {
		if strings.HasPrefix(key, prefix.String()) {
			delete(d.objects, key)
		}
	}
	return nil
}

// Delete implements storage.Driver.
func (d *Driver) Delete(_ context.Context, key string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.objects, key)
	return nil
}

// PresignGet implements storage.Driver with a memory:// URL.
func (d *Driver) PresignGet(_ context.Context, key string, ttl time.Duration) (string, error) {
	return "memory://get/" + url.PathEscape(key) + "?expires=" + strconv.FormatInt(d.now().Add(ttl).Unix(), 10), nil
}
