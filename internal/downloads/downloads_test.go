package downloads

import (
	"bytes"
	"context"
	"errors"
	"io"
	"slices"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

const (
	adminID resource.ID = 1
	owner   resource.ID = 2
)

type fakeAdmins struct{}

func (fakeAdmins) Get(_ context.Context, id, user resource.ID) (administration.Listed, error) {
	if id != adminID || user != owner {
		return administration.Listed{}, administration.ErrNotFound
	}
	return administration.Listed{}, nil
}

type fakeStore struct{ list []Download }

func (s *fakeStore) Downloads(_ context.Context, _ resource.ID, q Query) ([]Download, int, error) {
	var out []Download
	for _, d := range slices.Backward(s.list) {
		if q.Matches(d) {
			out = append(out, d)
		}
	}
	total := len(out)
	from := min((q.Page-1)*q.PerPage, total)
	return out[from:min(from+q.PerPage, total)], total, nil
}

func (s *fakeStore) Download(_ context.Context, _, id resource.ID) (Download, error) {
	for _, d := range s.list {
		if d.ID == id {
			return d, nil
		}
	}
	return Download{}, ErrNotFound
}

func (s *fakeStore) SaveDownload(_ context.Context, d Download) error {
	for i := range s.list {
		if s.list[i].ID == d.ID {
			s.list[i] = d
			return nil
		}
	}
	s.list = append(s.list, d)
	return nil
}

type fakeFiles map[string][]byte

func (f fakeFiles) Put(_ context.Context, key, _ string, data []byte) error {
	f[key] = data
	return nil
}

func (f fakeFiles) Open(_ context.Context, key string) (io.ReadCloser, string, error) {
	data, ok := f[key]
	if !ok {
		return nil, "", errors.New("missing")
	}
	return io.NopCloser(bytes.NewReader(data)), "", nil
}

func (f fakeFiles) Delete(_ context.Context, key string) error {
	delete(f, key)
	return nil
}

func TestDownloadsLifecycle(t *testing.T) {
	ctx := context.Background()
	store, files := &fakeStore{}, fakeFiles{}
	svc := NewService(fakeAdmins{}, store, files, resource.NewGenerator(1), func() time.Time { return time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC) })
	user := owner
	audit, err := svc.Add(ctx, adminID, &user, Auditfile, "auditfile.xaf", "application/xml", []byte("<xaf/>"))
	if err != nil {
		t.Fatal(err)
	}
	if audit.Size != 6 || audit.Downloaded || audit.Failed || files[audit.Key()] == nil {
		t.Errorf("added = %+v", audit)
	}
	failed, err := svc.AddFailed(ctx, adminID, nil, ExportContacts, "")
	if err != nil || !failed.Failed || failed.Filename != "Unknown" || failed.ContentType != "application/octet-stream" {
		t.Fatalf("failed = %+v, %v", failed, err)
	}
	if _, err := svc.Add(ctx, adminID, nil, "export_everything", "x", "", nil); !errors.Is(err, ErrUnknownType) {
		t.Errorf("unknown type = %v", err)
	}

	all := Types()
	list, total, err := svc.List(ctx, adminID, owner, Query{Types: all})
	if err != nil || total != 2 || list[0].ID != failed.ID {
		t.Errorf("list = %+v, %d, %v", list, total, err)
	}
	if list, _, _ := svc.List(ctx, adminID, owner, Query{Types: []string{Auditfile}}); len(list) != 1 {
		t.Errorf("allowed types only = %+v", list)
	}
	if list, _, _ := svc.List(ctx, adminID, owner, Query{Types: all, Failed: new(true)}); len(list) != 1 || list[0].ID != failed.ID {
		t.Errorf("failed only = %+v", list)
	}
	if _, _, err := svc.List(ctx, adminID, owner, Query{Types: all, Type: "nonsense"}); !errors.Is(err, ErrUnknownType) {
		t.Errorf("unknown filter = %v", err)
	}

	d, body, err := svc.Open(ctx, adminID, audit.ID, owner)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(body)
	_ = body.Close()
	if string(data) != "<xaf/>" || !d.Downloaded {
		t.Errorf("opened %+v: %s", d, data)
	}
	if got, _ := svc.Download(ctx, adminID, audit.ID, owner); !got.Downloaded {
		t.Error("download was not marked downloaded")
	}
	if _, _, err := svc.Open(ctx, adminID, failed.ID, owner); !errors.Is(err, ErrFailed) {
		t.Errorf("open failed = %v", err)
	}
	if _, err := svc.Download(ctx, adminID, audit.ID, 77); err == nil {
		t.Error("a stranger saw a download")
	}

	RegisterType("report_profit_loss", administration.Documents, administration.SalesInvoices)
	if p, ok := Permissions("report_profit_loss"); !ok || len(p) != 2 {
		t.Errorf("registered = %v, %v", p, ok)
	}
}
