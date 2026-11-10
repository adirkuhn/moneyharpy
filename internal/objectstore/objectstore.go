// Package objectstore keeps uploaded files in the S3-compatible storage
// configured in instance settings, such as Amazon S3, MinIO, Cloudflare R2 or
// Backblaze B2.
package objectstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/adirkuhn/moneyharpy/internal/settings"
)

// Errors returned by Store.
var (
	// ErrDisabled reports that no file storage is configured.
	ErrDisabled = errors.New("file storage is disabled")
	// ErrNotFound reports a key with no object.
	ErrNotFound = errors.New("object not found")
)

// checkKey is written and removed by Check.
const checkKey = ".moneyharpy-check"

// Settings provides the configured storage.
type Settings interface {
	Instance(ctx context.Context) (settings.Instance, error)
}

// Object is a stored file. The caller closes Body.
type Object struct {
	Body        io.ReadCloser
	ContentType string
	Size        int64
}

// Store reads and writes objects. It reads the settings on every call, so
// changes apply without a restart.
type Store struct {
	settings Settings
}

// New returns a Store using the storage configured in s.
func New(s Settings) *Store {
	return &Store{settings: s}
}

// Put stores data under key.
func (s *Store) Put(ctx context.Context, key, contentType string, data []byte) error {
	c, bucket, err := s.client(ctx)
	if err != nil {
		return err
	}
	_, err = c.PutObject(ctx, bucket, key, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return fmt.Errorf("store %s: %w", key, err)
	}
	return nil
}

// Get returns the object stored under key, or ErrNotFound.
func (s *Store) Get(ctx context.Context, key string) (Object, error) {
	c, bucket, err := s.client(ctx)
	if err != nil {
		return Object{}, err
	}
	obj, err := c.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return Object{}, fmt.Errorf("read %s: %w", key, err)
	}
	info, err := obj.Stat()
	if err != nil {
		_ = obj.Close()
		if minio.ToErrorResponse(err).Code == minio.NoSuchKey {
			return Object{}, ErrNotFound
		}
		return Object{}, fmt.Errorf("read %s: %w", key, err)
	}
	return Object{Body: obj, ContentType: info.ContentType, Size: info.Size}, nil
}

// Open returns the body and content type of the object under key. The caller
// closes the body.
func (s *Store) Open(ctx context.Context, key string) (io.ReadCloser, string, error) {
	obj, err := s.Get(ctx, key)
	if err != nil {
		return nil, "", err
	}
	return obj.Body, obj.ContentType, nil
}

// Delete removes the object under key. Removing a missing key is no error.
func (s *Store) Delete(ctx context.Context, key string) error {
	c, bucket, err := s.client(ctx)
	if err != nil {
		return err
	}
	if err := c.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("delete %s: %w", key, err)
	}
	return nil
}

// Check proves the settings work by writing and removing a small object. It
// creates the bucket first when it does not exist yet.
func (s *Store) Check(ctx context.Context) error {
	c, bucket, err := s.client(ctx)
	if err != nil {
		return err
	}
	exists, err := c.BucketExists(ctx, bucket)
	if err != nil {
		return fmt.Errorf("check bucket %s: %w", bucket, err)
	}
	if !exists {
		if err := c.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			return fmt.Errorf("create bucket %s: %w", bucket, err)
		}
	}
	if err := s.Put(ctx, checkKey, "text/plain", []byte("ok")); err != nil {
		return err
	}
	return s.Delete(ctx, checkKey)
}

func (s *Store) client(ctx context.Context) (*minio.Client, string, error) {
	inst, err := s.settings.Instance(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("load storage settings: %w", err)
	}
	cfg := inst.Storage
	if !cfg.Enabled {
		return nil, "", ErrDisabled
	}
	endpoint, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return nil, "", fmt.Errorf("storage endpoint: %w", err)
	}
	lookup := minio.BucketLookupDNS
	if cfg.PathStyle {
		lookup = minio.BucketLookupPath
	}
	c, err := minio.New(endpoint.Host, &minio.Options{
		Creds:        credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure:       endpoint.Scheme == "https",
		Region:       cfg.Region,
		BucketLookup: lookup,
	})
	if err != nil {
		return nil, "", fmt.Errorf("storage client: %w", err)
	}
	return c, cfg.Bucket, nil
}
