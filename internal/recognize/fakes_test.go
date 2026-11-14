package recognize

import (
	"bytes"
	"context"
	"errors"
	"io"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/attachments"
	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/ocr"
	"github.com/adirkuhn/moneyharpy/internal/purchases"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/settings"
)

type fakeStore struct {
	recs map[resource.ID]Recognition
	mems map[resource.ID]Memory
	// history holds the booked documents of each contact, oldest first.
	history      map[resource.ID][]purchases.Document
	historyCalls int
}

func newFakeStore() *fakeStore {
	return &fakeStore{recs: map[resource.ID]Recognition{}, mems: map[resource.ID]Memory{}, history: map[resource.ID][]purchases.Document{}}
}

func (s *fakeStore) History(_ context.Context, _, contact resource.ID, kind string, limit int) ([]purchases.Document, error) {
	s.historyCalls++
	var out []purchases.Document
	for _, d := range s.history[contact] {
		if d.Kind == kind {
			out = append(out, d)
		}
	}
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}

func (s *fakeStore) Recognition(_ context.Context, _, id resource.ID) (Recognition, error) {
	r, ok := s.recs[id]
	if !ok {
		return Recognition{}, ErrNotFound
	}
	r.Fields = slices.Clone(r.Fields)
	return r, nil
}

func (s *fakeStore) Recognitions(_ context.Context, _ resource.ID, ids []resource.ID) (map[resource.ID]Recognition, error) {
	out := map[resource.ID]Recognition{}
	for _, id := range ids {
		if r, ok := s.recs[id]; ok {
			out[id] = r
		}
	}
	return out, nil
}

func (s *fakeStore) SaveRecognition(_ context.Context, r Recognition) error {
	s.recs[r.DocumentID] = r
	return nil
}

func (s *fakeStore) DeleteRecognition(_ context.Context, _, id resource.ID) error {
	delete(s.recs, id)
	return nil
}

func (s *fakeStore) LeaseDue(_ context.Context, now, lease time.Time, maxCount int) ([]Recognition, error) {
	var out []Recognition
	for _, id := range slices.Sorted(maps.Keys(s.recs)) {
		r := s.recs[id]
		if r.State == StatePending && r.NextAttemptAt != nil && !r.NextAttemptAt.After(now) && len(out) < maxCount {
			r.NextAttemptAt = &lease
			s.recs[id] = r
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *fakeStore) Memory(_ context.Context, admin, contact resource.ID) (Memory, error) {
	if m, ok := s.mems[contact]; ok {
		m.Invoices.Lines = maps.Clone(m.Invoices.Lines)
		m.Receipts.Lines = maps.Clone(m.Receipts.Lines)
		return m, nil
	}
	return Memory{AdministrationID: admin, ContactID: contact}, nil
}

func (s *fakeStore) SaveMemory(_ context.Context, m Memory) error {
	s.mems[m.ContactID] = m
	return nil
}

type proposal struct {
	id      resource.ID
	kind    string
	changes purchases.Changes
}

// fakeDocuments holds one administration's incoming documents.
type fakeDocuments struct {
	incoming  map[resource.ID]purchases.Incoming
	typeless  map[resource.ID]purchases.Typeless
	documents map[resource.ID]purchases.Document
	files     map[resource.ID][]byte // by attachment id
	proposed  []proposal
	// refuse fails proposals while it holds errors.
	refuse []error
}

func newFakeDocuments() *fakeDocuments {
	return &fakeDocuments{
		incoming: map[resource.ID]purchases.Incoming{}, typeless: map[resource.ID]purchases.Typeless{},
		documents: map[resource.ID]purchases.Document{}, files: map[resource.ID][]byte{},
	}
}

// upload adds a typeless document holding files, named by extension.
func (d *fakeDocuments) upload(id resource.ID, files map[string][]byte) {
	t := purchases.Typeless{ID: id, Date: day("2026-11-13")}
	for _, name := range slices.Sorted(maps.Keys(files)) {
		aid := id*100 + resource.ID(len(t.Attachments)+1)
		ct := "application/pdf"
		if strings.HasSuffix(name, ".xml") {
			ct = "text/xml"
		}
		t.Attachments = append(t.Attachments, attachments.Attachment{ID: aid, Filename: name, ContentType: ct})
		d.files[aid] = files[name]
	}
	d.typeless[id] = t
	d.incoming[id] = purchases.Incoming{ID: id, Kind: purchases.TypelessDocument, State: purchases.New, AttachmentsCount: len(t.Attachments)}
}

func (d *fakeDocuments) IncomingDocument(_ context.Context, _, id, _ resource.ID) (purchases.Incoming, error) {
	inc, ok := d.incoming[id]
	if !ok {
		return purchases.Incoming{}, purchases.ErrNotFound
	}
	return inc, nil
}

func (d *fakeDocuments) TypelessDocument(_ context.Context, _, id, _ resource.ID) (purchases.Typeless, error) {
	t, ok := d.typeless[id]
	if !ok {
		return purchases.Typeless{}, purchases.ErrNotFound
	}
	return t, nil
}

func (d *fakeDocuments) open(id resource.ID) (attachments.Attachment, io.ReadCloser, error) {
	data, ok := d.files[id]
	if !ok {
		return attachments.Attachment{}, nil, attachments.ErrNotFound
	}
	return attachments.Attachment{ID: id}, io.NopCloser(bytes.NewReader(data)), nil
}

func (d *fakeDocuments) OpenTypelessAttachment(_ context.Context, _, _, attachment, _ resource.ID) (attachments.Attachment, io.ReadCloser, error) {
	return d.open(attachment)
}

func (d *fakeDocuments) Document(_ context.Context, _ resource.ID, _ string, id, _ resource.ID) (purchases.Document, error) {
	doc, ok := d.documents[id]
	if !ok {
		return purchases.Document{}, purchases.ErrNotFound
	}
	return doc, nil
}

func (d *fakeDocuments) OpenAttachment(_ context.Context, _, _ resource.ID, _ string, _, attachment resource.ID) (attachments.Attachment, io.ReadCloser, error) {
	return d.open(attachment)
}

func (d *fakeDocuments) Propose(_ context.Context, _, id, _ resource.ID, kind string, c purchases.Changes) (purchases.Document, error) {
	d.proposed = append(d.proposed, proposal{id, kind, c})
	if len(d.refuse) > 0 {
		err := d.refuse[0]
		d.refuse = d.refuse[1:]
		return purchases.Document{}, err
	}
	doc := purchases.Document{ID: id, Kind: kind, State: purchases.New}
	if c.ContactID != nil {
		doc.ContactID = *c.ContactID
	}
	return doc, nil
}

type fakeContacts struct {
	list    []contacts.Contact
	created []contacts.Changes
	searchs []string
}

func (c *fakeContacts) List(_ context.Context, _, _ resource.ID, q contacts.Query) (contacts.Page, error) {
	c.searchs = append(c.searchs, q.Search)
	return contacts.Page{Contacts: c.list}, nil
}

func (c *fakeContacts) Create(_ context.Context, _, _ resource.ID, ch contacts.Changes) (contacts.Contact, error) {
	c.created = append(c.created, ch)
	if ch.TaxNumber != nil && *ch.TaxNumber == "BAD" {
		var verr resource.ValidationError
		verr.Add("tax_number", "is invalid")
		return contacts.Contact{}, &verr
	}
	contact := contacts.Contact{ID: resource.ID(900 + len(c.created)), CompanyName: *ch.CompanyName}
	c.list = append(c.list, contact)
	return contact, nil
}

// fakeRegistry answers VAT number lookups from a table, and records them.
type fakeRegistry struct {
	companies map[string]RegisteredCompany
	err       error
	asked     []string
}

func (r *fakeRegistry) LookupVAT(_ context.Context, vat string) (RegisteredCompany, error) {
	r.asked = append(r.asked, vat)
	if r.err != nil {
		return RegisteredCompany{}, r.err
	}
	return r.companies[vat], nil
}

// uncategorized is the category of expenses still to categorise.
const uncategorized resource.ID = 999

type fakeLedger struct{}

func (fakeLedger) SystemLedgerAccount(_ context.Context, _, _ resource.ID, role string) (resource.ID, error) {
	if role != "uncategorized_expenses" {
		return 0, errors.New("unknown role " + role)
	}
	return uncategorized, nil
}

func (fakeLedger) TaxRates(context.Context, resource.ID, resource.ID, bookkeeping.TaxRateFilter) ([]bookkeeping.TaxRate, error) {
	return testRates, nil
}

type fakeSettings struct {
	rec  settings.Recognition
	vies bool
}

func (s *fakeSettings) Instance(context.Context) (settings.Instance, error) {
	return settings.Instance{Recognition: s.rec, VIESEnabled: s.vies}, nil
}

type fakeOCR struct {
	text    ocr.Text
	err     error
	reads   int
	renders int
	pages   int
	// sides are the sizes pages were rendered at.
	sides []int
}

func (o *fakeOCR) Read(context.Context, ocr.File) (ocr.Text, error) {
	o.reads++
	return o.text, o.err
}

func (o *fakeOCR) Render(_ context.Context, _ ocr.File, page int) (ocr.Page, error) {
	o.renders++
	if page > o.pages {
		return ocr.Page{}, errors.New("no such page")
	}
	return ocr.Page{PNG: []byte{byte(page)}, Count: o.pages}, nil
}

func (o *fakeOCR) RenderAt(ctx context.Context, f ocr.File, page, maxSide int) (ocr.Page, error) {
	o.sides = append(o.sides, maxSide)
	return o.Render(ctx, f, page)
}

func (o *fakeOCR) Ping(context.Context) error { return o.err }

type fakeModel struct {
	got   string
	pages int
	out   Extraction
	err   error
}

func (m *fakeModel) Extract(_ context.Context, text string, pages [][]byte) (Extraction, error) {
	m.got, m.pages = text, len(pages)
	return m.out, m.err
}

func (m *fakeModel) Ping(context.Context) error { return m.err }

type fakeNotifier struct {
	documents []resource.ID
	contacts  []resource.ID
}

func (n *fakeNotifier) DocumentUpdated(_ context.Context, _ resource.ID, d purchases.Document) {
	n.documents = append(n.documents, d.ID)
}

func (n *fakeNotifier) ContactCreated(_ context.Context, _ resource.ID, c contacts.Contact) {
	n.contacts = append(n.contacts, c.ID)
}

// clock is a time that tests move.
type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }
