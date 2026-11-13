package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
)

type fakeAbandoned struct {
	files []uuid.UUID
	err   error
}

func (a *fakeAbandoned) UploadAbandoned(_ context.Context, fileID uuid.UUID) error {
	if a.err != nil {
		return a.err
	}
	a.files = append(a.files, fileID)
	return nil
}

type fakeStore struct {
	mu     sync.Mutex
	files  map[uuid.UUID]*File
	failOn string
}

var errFake = errors.New("fake store failure")

func newFakeStore() *fakeStore { return &fakeStore{files: map[uuid.UUID]*File{}} }

func (s *fakeStore) fail(op string) error {
	if s.failOn == op {
		return errFake
	}
	return nil
}

func (s *fakeStore) Insert(_ context.Context, f *File) error {
	if err := s.fail("insert"); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c := *f
	s.files[f.ID] = &c
	return nil
}

func (s *fakeStore) ByID(_ context.Context, id uuid.UUID) (*File, error) {
	if err := s.fail("byID"); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if f, ok := s.files[id]; ok {
		c := *f
		return &c, nil
	}
	return nil, &UploadNotFoundError{PublicID: id.String()}
}

func (s *fakeStore) MarkReady(_ context.Context, id uuid.UUID, at time.Time) error {
	if err := s.fail("markReady"); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.files[id]
	switch {
	case !ok, f.Status == StatusDeleting:
		return &UploadNotFoundError{PublicID: id.String()}
	case f.Status == StatusPending && !f.ExpiresAt.After(at):
		return &UploadNotFoundError{PublicID: id.String()}
	case f.Status == StatusPending:
		f.Status, f.CompletedAt = StatusReady, &at
	}
	return nil
}

func (s *fakeStore) MarkDeleting(_ context.Context, id uuid.UUID) error {
	if err := s.fail("markDeleting"); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.files[id]
	if !ok {
		return &UploadNotFoundError{PublicID: id.String()}
	}
	f.Status = StatusDeleting
	return nil
}

func (s *fakeStore) Sweepable(_ context.Context, now time.Time, limit int) ([]*File, error) {
	if err := s.fail("sweepable"); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*File
	for _, f := range s.files {
		if f.Status == StatusDeleting || (f.Status == StatusPending && f.ExpiresAt.Before(now)) {
			c := *f
			out = append(out, &c)
		}
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func (s *fakeStore) DeleteDeleting(_ context.Context, id uuid.UUID) error {
	if err := s.fail("delete"); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if f, ok := s.files[id]; ok && f.Status == StatusDeleting {
		delete(s.files, id)
	}
	return nil
}

func (s *fakeStore) LockSweepable(_ context.Context, id uuid.UUID, now time.Time) (*File, error) {
	if err := s.fail("lock"); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.files[id]
	if !ok || (f.Status != StatusDeleting && (f.Status != StatusPending || !f.ExpiresAt.Before(now))) {
		return nil, nil
	}
	c := *f
	return &c, nil
}

type directTx struct{}

func (directTx) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

func (s *fakeStore) get(id uuid.UUID) *File {
	s.mu.Lock()
	defer s.mu.Unlock()
	if f, ok := s.files[id]; ok {
		c := *f
		return &c
	}
	return nil
}

type fakeDriver struct {
	mu      sync.Mutex
	objects map[string]Object
	bodies  map[string][]byte
	deleted []string
	failOn  string
}

func newFakeDriver() *fakeDriver {
	return &fakeDriver{objects: map[string]Object{}, bodies: map[string][]byte{}}
}

func (d *fakeDriver) put(key, contentType string, body []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.objects[key] = Object{Size: int64(len(body)), ContentType: contentType}
	d.bodies[key] = body
}

func (d *fakeDriver) has(key string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, ok := d.objects[key]
	return ok
}

func (d *fakeDriver) fail(op string) error {
	if d.failOn == op {
		return errFake
	}
	return nil
}

func (d *fakeDriver) PresignPut(_ context.Context, key, contentType string, _ int64, ttl time.Duration) (Upload, error) {
	if err := d.fail("presignPut"); err != nil {
		return Upload{}, err
	}
	return Upload{URL: "https://s3.test/" + key, Method: UploadMethod, Headers: map[string]string{"Content-Type": contentType}, ExpiresAt: time.Now().Add(ttl)}, nil
}

func (d *fakeDriver) Head(_ context.Context, key string) (Object, error) {
	if err := d.fail("head"); err != nil {
		return Object{}, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	o, ok := d.objects[key]
	if !ok {
		return Object{}, &ObjectNotFoundError{Key: key}
	}
	return o, nil
}

func (d *fakeDriver) ReadPrefix(_ context.Context, key string, n int64) ([]byte, error) {
	if err := d.fail("read"); err != nil {
		return nil, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	b := d.bodies[key]
	return b[:min(n, int64(len(b)))], nil
}

func (d *fakeDriver) Delete(_ context.Context, key string) error {
	if err := d.fail("delete"); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.objects, key)
	delete(d.bodies, key)
	d.deleted = append(d.deleted, key)
	return nil
}

func (d *fakeDriver) Open(_ context.Context, key string, _ int64) (io.ReadCloser, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return io.NopCloser(bytes.NewReader(d.bodies[key])), nil
}

func (d *fakeDriver) DeletePrefix(context.Context, Prefix) error { return nil }

func (d *fakeDriver) PresignGet(_ context.Context, key string, _ time.Duration) (string, error) {
	if err := d.fail("presignGet"); err != nil {
		return "", err
	}
	return "https://s3.test/" + key + "?signed", nil
}
