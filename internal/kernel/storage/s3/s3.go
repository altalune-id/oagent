// Package s3 is the storage driver on an S3-compatible bucket: presigned PUT and GET, HEAD, ranged GET and DELETE.
package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"

	"altalune.id/oagent/internal/kernel/storage"
	s3conn "altalune.id/oagent/s3"
)

var _ storage.Driver = (*Driver)(nil)

// Driver runs storage on the one S3 connection.
type Driver struct {
	c   *s3conn.Client
	now func() time.Time
}

// New returns a Driver over c.
func New(c *s3conn.Client) *Driver { return &Driver{c: c, now: time.Now} }

// PresignPut implements storage.Driver; the signature covers the content type and the exact length.
func (d *Driver) PresignPut(ctx context.Context, key, contentType string, size int64, ttl time.Duration) (storage.Upload, error) {
	req, err := d.c.Presign.PresignPutObject(ctx, &awss3.PutObjectInput{
		Bucket: aws.String(d.c.Bucket), Key: aws.String(key),
		ContentType: aws.String(contentType), ContentLength: aws.Int64(size),
	}, awss3.WithPresignExpires(ttl))
	if err != nil {
		return storage.Upload{}, fmt.Errorf("storage.s3: presign put: %w", err)
	}
	return storage.Upload{
		URL:    req.URL,
		Method: storage.UploadMethod,
		Headers: map[string]string{
			"Content-Type":   contentType,
			"Content-Length": strconv.FormatInt(size, 10),
		},
		ExpiresAt: d.now().Add(ttl).UTC(),
	}, nil
}

// Head implements storage.Driver.
func (d *Driver) Head(ctx context.Context, key string) (storage.Object, error) {
	out, err := d.c.S3.HeadObject(ctx, &awss3.HeadObjectInput{Bucket: aws.String(d.c.Bucket), Key: aws.String(key)})
	if err != nil {
		return storage.Object{}, notFound(key, err, "head")
	}
	return storage.Object{Size: aws.ToInt64(out.ContentLength), ContentType: aws.ToString(out.ContentType)}, nil
}

// ReadPrefix implements storage.Driver with a ranged GET.
func (d *Driver) ReadPrefix(ctx context.Context, key string, n int64) ([]byte, error) {
	out, err := d.c.S3.GetObject(ctx, &awss3.GetObjectInput{
		Bucket: aws.String(d.c.Bucket), Key: aws.String(key), Range: aws.String("bytes=0-" + strconv.FormatInt(n-1, 10)),
	})
	if err != nil {
		return nil, notFound(key, err, "read")
	}
	defer func() { _ = out.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(out.Body, n))
	if err != nil {
		return nil, fmt.Errorf("storage.s3: read: %w", err)
	}
	return b, nil
}

// Open implements storage.Driver with a plain GET, capped as it is read.
func (d *Driver) Open(ctx context.Context, key string, maxBytes int64) (io.ReadCloser, error) {
	out, err := d.c.S3.GetObject(ctx, &awss3.GetObjectInput{Bucket: aws.String(d.c.Bucket), Key: aws.String(key)})
	if err != nil {
		return nil, notFound(key, err, "open")
	}
	return storage.CapReader(out.Body, maxBytes), nil
}

// DeletePrefix implements storage.Driver with ListObjectsV2 and batched DeleteObjects.
func (d *Driver) DeletePrefix(ctx context.Context, prefix storage.Prefix) error {
	if prefix.IsZero() {
		return &storage.InvalidPrefixError{}
	}
	list := &awss3.ListObjectsV2Input{Bucket: aws.String(d.c.Bucket), Prefix: aws.String(prefix.String())}
	pages := awss3.NewListObjectsV2Paginator(d.c.S3, list)
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("storage.s3: list %s: %w", prefix, err)
		}
		if len(page.Contents) == 0 {
			continue
		}
		ids := make([]types.ObjectIdentifier, 0, len(page.Contents))
		for _, o := range page.Contents {
			ids = append(ids, types.ObjectIdentifier{Key: o.Key})
		}
		out, err := d.c.S3.DeleteObjects(ctx, &awss3.DeleteObjectsInput{
			Bucket: aws.String(d.c.Bucket), Delete: &types.Delete{Objects: ids, Quiet: aws.Bool(true)},
		})
		if err != nil {
			return fmt.Errorf("storage.s3: delete prefix %s: %w", prefix, err)
		}
		if len(out.Errors) > 0 {
			return fmt.Errorf("storage.s3: delete prefix %s: %d objects not deleted", prefix, len(out.Errors))
		}
	}
	return nil
}

// Delete implements storage.Driver; S3 answers a missing key with success.
func (d *Driver) Delete(ctx context.Context, key string) error {
	if _, err := d.c.S3.DeleteObject(ctx, &awss3.DeleteObjectInput{Bucket: aws.String(d.c.Bucket), Key: aws.String(key)}); err != nil {
		return fmt.Errorf("storage.s3: delete: %w", err)
	}
	return nil
}

// PresignGet implements storage.Driver.
func (d *Driver) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	req, err := d.c.Presign.PresignGetObject(ctx, &awss3.GetObjectInput{Bucket: aws.String(d.c.Bucket), Key: aws.String(key)},
		awss3.WithPresignExpires(ttl))
	if err != nil {
		return "", fmt.Errorf("storage.s3: presign get: %w", err)
	}
	return req.URL, nil
}

func notFound(key string, err error, op string) error {
	if apiErr, ok := errors.AsType[smithy.APIError](err); ok {
		switch apiErr.ErrorCode() {
		case "NotFound", "NoSuchKey":
			return &storage.ObjectNotFoundError{Key: key}
		}
	}
	return fmt.Errorf("storage.s3: %s: %w", op, err)
}
