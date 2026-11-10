// Package diskstore keeps uploaded files in a folder on the server. It is
// used when no S3-compatible storage is configured.
package diskstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Errors returned by Store.
var (
	ErrNotFound   = errors.New("file not found")
	ErrInvalidKey = errors.New("invalid file key")
)

// Store reads and writes files under one root folder.
type Store struct {
	root string
}

// New returns a Store keeping files under root.
func New(root string) *Store {
	return &Store{root: root}
}

// file maps a key such as "a/b/c" to a path under the root. Keys are clean,
// relative and slash-separated, so no key reaches outside the root.
func (s *Store) file(key string) (string, error) {
	if key == "" || strings.Contains(key, "\\") || path.IsAbs(key) || path.Clean(key) != key || strings.HasPrefix(key, "..") {
		return "", fmt.Errorf("%w: %q", ErrInvalidKey, key)
	}
	return filepath.Join(s.root, filepath.FromSlash(key)), nil
}

// Put stores data under key, replacing any file there. The write goes to a
// temporary file first, so readers never see half a file.
func (s *Store) Put(_ context.Context, key, _ string, data []byte) error {
	name, err := s.file(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(name), 0o750); err != nil {
		return fmt.Errorf("create folder for %s: %w", key, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(name), ".upload-*")
	if err != nil {
		return fmt.Errorf("store %s: %w", key, err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // A no-op once renamed.
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("store %s: %w", key, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("store %s: %w", key, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("store %s: %w", key, err)
	}
	if err := os.Rename(tmp.Name(), name); err != nil {
		return fmt.Errorf("store %s: %w", key, err)
	}
	return nil
}

// Open returns the file under key, or ErrNotFound. The content type is not
// kept on disk, so it is empty; callers know it from their own records.
func (s *Store) Open(_ context.Context, key string) (io.ReadCloser, string, error) {
	name, err := s.file(key)
	if err != nil {
		return nil, "", err
	}
	f, err := os.Open(name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", fmt.Errorf("open %s: %w", key, err)
	}
	return f, "", nil
}

// Delete removes the file under key. Removing a missing file is no error.
func (s *Store) Delete(_ context.Context, key string) error {
	name, err := s.file(key)
	if err != nil {
		return err
	}
	if err := os.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("delete %s: %w", key, err)
	}
	return nil
}
