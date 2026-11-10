package uploads_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/diskstore"
	"github.com/adirkuhn/moneyharpy/internal/objectstore"
	"github.com/adirkuhn/moneyharpy/internal/uploads"
)

// fakeS3 behaves like objectstore.Store, enabled or not.
type fakeS3 struct {
	enabled bool
	data    map[string][]byte
}

func (f *fakeS3) Put(_ context.Context, key, _ string, data []byte) error {
	if !f.enabled {
		return objectstore.ErrDisabled
	}
	f.data[key] = data
	return nil
}

func (f *fakeS3) Open(_ context.Context, key string) (io.ReadCloser, string, error) {
	if !f.enabled {
		return nil, "", objectstore.ErrDisabled
	}
	d, ok := f.data[key]
	if !ok {
		return nil, "", objectstore.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(d)), "image/png", nil
}

func (f *fakeS3) Delete(_ context.Context, key string) error {
	if !f.enabled {
		return objectstore.ErrDisabled
	}
	delete(f.data, key)
	return nil
}

func read(t *testing.T, s *uploads.Store, key string) string {
	t.Helper()
	body, _, err := s.Open(context.Background(), key)
	if err != nil {
		t.Fatalf("Open(%s) error = %v", key, err)
	}
	defer func() { _ = body.Close() }()
	got, _ := io.ReadAll(body)
	return string(got)
}

func TestWithoutS3FilesGoToDisk(t *testing.T) {
	ctx := context.Background()
	s3 := &fakeS3{data: map[string][]byte{}}
	disk := diskstore.New(t.TempDir())
	s := uploads.New(s3, disk)
	if err := s.Put(ctx, "a/logo", "image/png", []byte("disk")); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if got := read(t, s, "a/logo"); got != "disk" {
		t.Errorf("Open() = %q", got)
	}
	if err := s.Delete(ctx, "a/logo"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, _, err := s.Open(ctx, "a/logo"); !errors.Is(err, uploads.ErrNotFound) {
		t.Errorf("Open() after delete error = %v, want ErrNotFound", err)
	}
}

func TestWithS3FilesGoThereAndOlderDiskFilesStayReadable(t *testing.T) {
	ctx := context.Background()
	s3 := &fakeS3{data: map[string][]byte{}}
	disk := diskstore.New(t.TempDir())
	s := uploads.New(s3, disk)
	if err := s.Put(ctx, "a/old", "image/png", []byte("before s3")); err != nil {
		t.Fatal(err)
	}

	s3.enabled = true
	if err := s.Put(ctx, "a/new", "image/png", []byte("in s3")); err != nil {
		t.Fatal(err)
	}
	if _, ok := s3.data["a/new"]; !ok {
		t.Error("new file did not go to S3")
	}
	if got := read(t, s, "a/new"); got != "in s3" {
		t.Errorf("Open(new) = %q", got)
	}
	if got := read(t, s, "a/old"); got != "before s3" {
		t.Errorf("Open(old) = %q", got)
	}
	if err := s.Delete(ctx, "a/old"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Open(ctx, "a/old"); !errors.Is(err, uploads.ErrNotFound) {
		t.Errorf("Open(old) after delete error = %v", err)
	}
	if err := s.Delete(ctx, "a/new"); err != nil || len(s3.data) != 0 {
		t.Errorf("Delete(new) = %v, S3 holds %d", err, len(s3.data))
	}
}
