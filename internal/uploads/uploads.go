// Package uploads decides where uploaded files live: in the S3-compatible
// storage when the installation has one configured, and on the server's disk
// otherwise. Reading checks the disk first, so files stored before S3 was
// set up stay available.
package uploads

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/adirkuhn/moneyharpy/internal/diskstore"
	"github.com/adirkuhn/moneyharpy/internal/invoicing"
	"github.com/adirkuhn/moneyharpy/internal/objectstore"
)

// ErrNotFound reports a key stored nowhere. It is invoicing.ErrFileNotFound,
// so callers there need no knowledge of this package.
var ErrNotFound = invoicing.ErrFileNotFound

// Backend stores files. objectstore.Store and diskstore.Store satisfy it.
type Backend interface {
	Put(ctx context.Context, key, contentType string, data []byte) error
	Open(ctx context.Context, key string) (io.ReadCloser, string, error)
	Delete(ctx context.Context, key string) error
}

// Store routes files between S3 and the disk.
type Store struct {
	s3   Backend
	disk Backend
}

// New returns a Store using s3 when it is configured and disk otherwise.
// s3 must report objectstore.ErrDisabled while it is not configured.
func New(s3, disk Backend) *Store {
	return &Store{s3: s3, disk: disk}
}

// Put stores data in S3, or on disk when S3 is not configured.
func (s *Store) Put(ctx context.Context, key, contentType string, data []byte) error {
	err := s.s3.Put(ctx, key, contentType, data)
	if errors.Is(err, objectstore.ErrDisabled) {
		return s.disk.Put(ctx, key, contentType, data)
	}
	return err
}

// Open returns the file from disk, or else from S3.
func (s *Store) Open(ctx context.Context, key string) (io.ReadCloser, string, error) {
	body, contentType, err := s.disk.Open(ctx, key)
	if !errors.Is(err, diskstore.ErrNotFound) {
		return body, contentType, err
	}
	body, contentType, err = s.s3.Open(ctx, key)
	if errors.Is(err, objectstore.ErrDisabled) || errors.Is(err, objectstore.ErrNotFound) {
		return nil, "", fmt.Errorf("%w: %s", ErrNotFound, key)
	}
	return body, contentType, err
}

// Delete removes the file wherever it is.
func (s *Store) Delete(ctx context.Context, key string) error {
	if err := s.disk.Delete(ctx, key); err != nil {
		return err
	}
	if err := s.s3.Delete(ctx, key); err != nil && !errors.Is(err, objectstore.ErrDisabled) {
		return err
	}
	return nil
}
