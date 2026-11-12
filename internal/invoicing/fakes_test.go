package invoicing

import (
	"bytes"
	"context"
	"io"
	"maps"
	"slices"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// fakeAdmins knows one administration, owned by user 1, with two identities.
type fakeAdmins struct{ admin administration.Administration }

const owner, stranger resource.ID = 1, 2

func (f fakeAdmins) Get(_ context.Context, id, user resource.ID) (administration.Listed, error) {
	if id != f.admin.ID || user != owner {
		return administration.Listed{}, administration.ErrNotFound
	}
	return administration.Listed{Administration: f.admin}, nil
}

func (f fakeAdmins) DefaultIdentity(ctx context.Context, admin, user resource.ID) (administration.Identity, error) {
	return f.Identity(ctx, admin, 500, user)
}

func (f fakeAdmins) Identity(ctx context.Context, admin, id, user resource.ID) (administration.Identity, error) {
	if _, err := f.Get(ctx, admin, user); err != nil {
		return administration.Identity{}, err
	}
	if id != 500 && id != 501 {
		return administration.Identity{}, administration.ErrIdentityNotFound
	}
	return administration.Identity{ID: id, AdministrationID: admin, Default: id == 500}, nil
}

func (f fakeAdmins) CustomFields(ctx context.Context, admin, user resource.ID) ([]administration.CustomField, error) {
	if _, err := f.Get(ctx, admin, user); err != nil {
		return nil, err
	}
	return []administration.CustomField{
		{ID: 700, AdministrationID: admin, Name: "Brand", Source: administration.SourceIdentity},
		{ID: 701, AdministrationID: admin, Name: "Project", Source: administration.SourceSalesInvoice},
		{ID: 702, AdministrationID: admin, Name: "Site", Source: administration.SourceContact},
		{ID: 703, AdministrationID: admin, Name: "Offer", Source: administration.SourceEstimate},
	}, nil
}

type memStore struct {
	settings  Settings
	workflows map[resource.ID]Workflow
	styles    map[resource.ID]DocumentStyle
}

func newMemStore() *memStore {
	return &memStore{
		settings:  Settings{InvoiceIDTemplate: "{year}-{id}", InvoiceSequenceID: "0001", EstimateIDTemplate: "{year}-{id}", EstimateSequenceID: "0001"},
		workflows: map[resource.ID]Workflow{},
		styles:    map[resource.ID]DocumentStyle{},
	}
}

func (m *memStore) Settings(context.Context, resource.ID) (Settings, error) { return m.settings, nil }
func (m *memStore) SaveSettings(_ context.Context, _ resource.ID, s Settings) error {
	m.settings = s
	return nil
}

func (m *memStore) Workflows(context.Context, resource.ID) ([]Workflow, error) {
	return slices.Collect(maps.Values(m.workflows)), nil
}

func (m *memStore) Workflow(_ context.Context, _ resource.ID, id resource.ID) (Workflow, error) {
	w, ok := m.workflows[id]
	if !ok {
		return Workflow{}, ErrWorkflowNotFound
	}
	return w, nil
}

func (m *memStore) CreateWorkflow(_ context.Context, w Workflow) (bool, error) {
	if w.Default && slices.ContainsFunc(slices.Collect(maps.Values(m.workflows)), func(x Workflow) bool { return x.Default && x.Type == w.Type }) {
		return false, nil
	}
	m.workflows[w.ID] = w
	return true, nil
}

func (m *memStore) UpdateWorkflow(_ context.Context, w Workflow) error {
	if w.Default {
		for id, x := range m.workflows {
			if x.Type == w.Type && x.Default && id != w.ID {
				x.Default = false
				m.workflows[id] = x
			}
		}
	}
	m.workflows[w.ID] = w
	return nil
}

func (m *memStore) DeleteWorkflow(_ context.Context, _ resource.ID, id resource.ID) error {
	delete(m.workflows, id)
	return nil
}

func (m *memStore) DocumentStyles(context.Context, resource.ID) ([]DocumentStyle, error) {
	return slices.Collect(maps.Values(m.styles)), nil
}

func (m *memStore) DocumentStyle(_ context.Context, _ resource.ID, id resource.ID) (DocumentStyle, error) {
	d, ok := m.styles[id]
	if !ok {
		return DocumentStyle{}, ErrDocumentStyleNotFound
	}
	return d, nil
}

func (m *memStore) CreateDocumentStyle(_ context.Context, d DocumentStyle) (bool, error) {
	if d.Default && slices.ContainsFunc(slices.Collect(maps.Values(m.styles)), func(x DocumentStyle) bool { return x.Default }) {
		return false, nil
	}
	m.styles[d.ID] = d
	return true, nil
}

func (m *memStore) UpdateDocumentStyle(_ context.Context, d DocumentStyle) error {
	if d.Default {
		for id, x := range m.styles {
			if x.Default && id != d.ID {
				x.Default = false
				m.styles[id] = x
			}
		}
	}
	m.styles[d.ID] = d
	return nil
}

func (m *memStore) DeleteDocumentStyle(_ context.Context, _ resource.ID, id resource.ID) error {
	delete(m.styles, id)
	return nil
}

func (m *memStore) FileInUse(_ context.Context, _ resource.ID, hash string) (bool, error) {
	for _, d := range m.styles {
		for _, f := range d.StoredFiles() {
			if f.Hash == hash {
				return true, nil
			}
		}
	}
	return false, nil
}

type memFiles struct{ data map[string][]byte }

func (f *memFiles) Put(_ context.Context, key, _ string, data []byte) error {
	f.data[key] = data
	return nil
}

func (f *memFiles) Open(_ context.Context, key string) (io.ReadCloser, string, error) {
	return io.NopCloser(bytes.NewReader(f.data[key])), "", nil
}

func (f *memFiles) Delete(_ context.Context, key string) error {
	delete(f.data, key)
	return nil
}

const adminID resource.ID = 11

func newTestService() (*Service, *memStore, *memFiles) {
	store, files := newMemStore(), &memFiles{data: map[string][]byte{}}
	admins := fakeAdmins{admin: administration.Administration{ID: adminID, Language: "nl", Currency: "EUR", TimeZone: "Europe/Amsterdam"}}
	svc := NewService(admins, store, files, resource.NewGenerator(3), func() time.Time {
		return time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	})
	return svc, store, files
}

func ptr[T any](v T) *T { return &v }
