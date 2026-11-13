package attachments

import (
	"bytes"
	"context"
	"errors"
	"io"
	"slices"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

const admin resource.ID = 100

type memFiles map[string][]byte

func (m memFiles) Put(_ context.Context, key, _ string, data []byte) error { m[key] = data; return nil }
func (m memFiles) Open(_ context.Context, key string) (io.ReadCloser, string, error) {
	d, ok := m[key]
	if !ok {
		return nil, "", errors.New("missing")
	}
	return io.NopCloser(bytes.NewReader(d)), "", nil
}
func (m memFiles) Delete(_ context.Context, key string) error { delete(m, key); return nil }

type memStore struct{ list []Attachment }

func (m *memStore) CreateAttachment(_ context.Context, a Attachment) error {
	m.list = append(m.list, a)
	return nil
}
func (m *memStore) DeleteAttachment(_ context.Context, _, id resource.ID) error {
	m.list = slices.DeleteFunc(m.list, func(a Attachment) bool { return a.ID == id })
	return nil
}
func (m *memStore) AttachmentsOf(_ context.Context, _ resource.ID, entityType string, entities []resource.ID) ([]Attachment, error) {
	var out []Attachment
	for _, a := range m.list {
		if a.EntityType == entityType && slices.Contains(entities, a.EntityID) {
			out = append(out, a)
		}
	}
	return out, nil
}

func (m *memStore) UnhashedAttachments(_ context.Context, entityTypes []string, after resource.ID, limit int) ([]Attachment, error) {
	var out []Attachment
	for _, a := range m.list {
		if a.SHA256 == "" && a.ID > after && slices.Contains(entityTypes, a.EntityType) && len(out) < limit {
			out = append(out, a)
		}
	}
	return out, nil
}
func (m *memStore) SetAttachmentHash(_ context.Context, _, id resource.ID, sum string) error {
	for i := range m.list {
		if m.list[i].ID == id && m.list[i].SHA256 == "" {
			m.list[i].SHA256 = sum
		}
	}
	return nil
}

func newService() (*Service, memFiles) {
	files := memFiles{}
	return NewService(&memStore{}, files, resource.NewGenerator(1), func() time.Time { return time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC) }), files
}

var pdf = []byte("%PDF-1.4\n1 0 obj\n<<>>\nendobj\n")

func TestAddSniffsAndChecks(t *testing.T) {
	s, files := newService()
	ctx := context.Background()
	a, err := s.Add(ctx, admin, "SalesInvoice", 10, `C:\scans\receipt.pdf`, "application/octet-stream", pdf)
	if err != nil {
		t.Fatal(err)
	}
	if a.Filename != "receipt.pdf" || a.ContentType != "application/pdf" || a.Size != int64(len(pdf)) || files[a.Key()] == nil {
		t.Errorf("attachment = %+v", a)
	}
	if _, err := s.Add(ctx, admin, "SalesInvoice", 10, "app.exe", "", []byte{0x4d, 0x5a, 0x90, 0x00}); err == nil {
		t.Error("executable accepted")
	}
	if _, err := s.Add(ctx, admin, "SalesInvoice", 10, "big.pdf", "application/pdf", make([]byte, MaxSize+1)); err == nil {
		t.Error("oversized file accepted")
	}
	if _, err := s.Add(ctx, admin, "SalesInvoice", 10, "empty.pdf", "application/pdf", nil); err == nil {
		t.Error("empty file accepted")
	}
}

func TestOpenAndDeleteStayOnTheRecord(t *testing.T) {
	s, files := newService()
	ctx := context.Background()
	a, _ := s.Add(ctx, admin, "SalesInvoice", 10, "a.pdf", "application/pdf", pdf)
	if _, _, err := s.Open(ctx, admin, "SalesInvoice", 11, a.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("open through another record = %v", err)
	}
	got, body, err := s.Open(ctx, admin, "SalesInvoice", 10, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(body)
	_ = body.Close()
	if got.ID != a.ID || !bytes.Equal(data, pdf) {
		t.Errorf("opened %+v %q", got, data)
	}
	if err := s.DeleteAll(ctx, admin, "SalesInvoice", 10); err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Errorf("files left: %d", len(files))
	}
}

// pdfSum is the SHA-256 of pdf, in hex.
const pdfSum = "4cdcd30197a080d2f5aa3c5750a83c03d619523936b79cf0efffdb62ec667b6f"

func TestAddSumsTheContents(t *testing.T) {
	s, _ := newService()
	a, err := s.Add(context.Background(), admin, "Receipt", 10, "a.pdf", "application/pdf", pdf)
	if err != nil {
		t.Fatal(err)
	}
	if a.SHA256 != pdfSum {
		t.Errorf("sha256 = %q, want %q", a.SHA256, pdfSum)
	}
}

func TestHashMissingSumsOldAttachmentsOfTheTypes(t *testing.T) {
	store, files := &memStore{}, memFiles{}
	s := NewService(store, files, resource.NewGenerator(1), time.Now)
	s.hashBatch = 2
	ctx := context.Background()
	for i, typ := range []string{"Receipt", "SalesInvoice", "TypelessDocument", "Receipt", "Receipt"} {
		a := Attachment{ID: resource.ID(i + 1), AdministrationID: admin, EntityType: typ, EntityID: 10}
		store.list = append(store.list, a)
		if i != 3 { // The file of the fourth is gone; it stays unsummed.
			files[a.Key()] = pdf
		}
	}
	if err := s.HashMissing(ctx, []string{"Receipt", "TypelessDocument"}); err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(store.list))
	for i, a := range store.list {
		got[i] = a.SHA256
	}
	if want := []string{pdfSum, "", pdfSum, "", pdfSum}; !slices.Equal(got, want) {
		t.Errorf("sums = %q, want %q", got, want)
	}
}
