package diskstore_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/diskstore"
)

func TestPutOpenDelete(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	s := diskstore.New(root)
	if err := s.Put(ctx, "administrations/1/document_styles/abc", "image/png", []byte("\x89PNG data")); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	body, _, err := s.Open(ctx, "administrations/1/document_styles/abc")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	got, _ := io.ReadAll(body)
	_ = body.Close()
	if string(got) != "\x89PNG data" {
		t.Errorf("Open() = %q", got)
	}
	if _, err := os.Stat(filepath.Join(root, "administrations", "1", "document_styles", "abc")); err != nil {
		t.Errorf("file not under the root: %v", err)
	}
	if err := s.Delete(ctx, "administrations/1/document_styles/abc"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, _, err := s.Open(ctx, "administrations/1/document_styles/abc"); !errors.Is(err, diskstore.ErrNotFound) {
		t.Errorf("Open() after delete error = %v, want ErrNotFound", err)
	}
	if err := s.Delete(ctx, "administrations/1/document_styles/abc"); err != nil {
		t.Errorf("Delete() of a missing file error = %v", err)
	}
}

func TestRejectsKeysOutsideTheRoot(t *testing.T) {
	s := diskstore.New(t.TempDir())
	for _, key := range []string{"../escape", "a/../../escape", "/etc/passwd", "a\\b", "", "a//b", "a/./b"} {
		if err := s.Put(context.Background(), key, "text/plain", []byte("x")); !errors.Is(err, diskstore.ErrInvalidKey) {
			t.Errorf("Put(%q) error = %v, want ErrInvalidKey", key, err)
		}
	}
}
