package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/invoicing"
	"github.com/adirkuhn/moneyharpy/internal/postgres"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// newAdministrationWithIdentity stores an administration owned by a new user.
func newAdministrationWithIdentity(t *testing.T) (administration.Administration, administration.Identity, resource.ID) {
	t.Helper()
	owner := newUser(t)
	a := newAdministration()
	def := newDefaultIdentity(a)
	if err := postgres.NewAdministrationStore(testPool).Create(context.Background(), a, owner.ID, def); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	return a, def, owner.ID
}

var approxTime = cmpopts.EquateApproxTime(time.Millisecond)

func TestInvoiceSettingsRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := postgres.NewInvoicingStore(testPool)
	a, _, _ := newAdministrationWithIdentity(t)

	got, err := s.Settings(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := invoicing.Settings{
		InvoiceIDTemplate: "{year}-{id}", InvoiceSequenceID: "0001", EstimateIDTemplate: "{year}-{id}", EstimateSequenceID: "0001",
		DefaultEmailUBL: true, PaymentsWithoutProof: true,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Settings() defaults diff (-want +got):\n%s", diff)
	}
	want = invoicing.Settings{
		InvoiceIDTemplate: "F{id}", InvoiceSequenceID: "1462", EstimateIDTemplate: "O{id}", EstimateSequenceID: "0027",
		InvoiceBCC: "a@example.com", InvoiceRemindersBCC: true, EstimateBCC: "b@example.com", OrderBCC: "c@example.com",
	}
	if err := s.SaveSettings(ctx, a.ID, want); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.Settings(ctx, a.ID); !cmp.Equal(want, got) {
		t.Errorf("Settings() after save = %+v, want %+v", got, want)
	}
}

func newWorkflow(a administration.Administration, def bool, name string) invoicing.Workflow {
	now := time.Now().UTC().Truncate(time.Microsecond)
	return invoicing.Workflow{
		ID: ids.Next(), AdministrationID: a.ID, Type: invoicing.InvoiceWorkflow, Name: name, Default: def,
		Currency: "EUR", Language: "nl", Active: true, ShowQRCode: true, CreatedAt: now, UpdatedAt: now,
		Steps: []invoicing.WorkflowStep{
			{ID: ids.Next(), StepOrder: 1, DueInterval: 14, CreatedAt: now, UpdatedAt: now},
			{ID: ids.Next(), StepOrder: 2, ReminderDelay: 7, ReminderText: "Pay", ReminderAutoSend: true, CreatedAt: now, UpdatedAt: now},
		},
	}
}

func TestWorkflowStore(t *testing.T) {
	ctx := context.Background()
	s := postgres.NewInvoicingStore(testPool)
	a, _, owner := newAdministrationWithIdentity(t)

	def := newWorkflow(a, true, "Standaard (NL)")
	if created, err := s.CreateWorkflow(ctx, def); err != nil || !created {
		t.Fatalf("CreateWorkflow() = %v, %v", created, err)
	}
	if created, err := s.CreateWorkflow(ctx, newWorkflow(a, true, "Second default")); err != nil || created {
		t.Errorf("CreateWorkflow(second default) = %v, %v; want false", created, err)
	}
	got, err := s.Workflow(ctx, a.ID, def.ID)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(def, got, approxTime); diff != "" {
		t.Errorf("Workflow() diff (-want +got):\n%s", diff)
	}

	other := newWorkflow(a, false, "Export")
	if _, err := s.CreateWorkflow(ctx, other); err != nil {
		t.Fatal(err)
	}
	other.Default = true
	other.Steps = other.Steps[:1]
	if err := s.UpdateWorkflow(ctx, other); err != nil {
		t.Fatal(err)
	}
	list, err := s.Workflows(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range list {
		if w.ID == def.ID && w.Default {
			t.Error("old default kept its default flag")
		}
		if w.ID == other.ID && (!w.Default || len(w.Steps) != 1) {
			t.Errorf("updated workflow = %+v", w)
		}
	}

	if ref, err := postgres.NewResourceLocator(testPool).Locate(ctx, other.ID, owner); err != nil || ref != other.Ref() {
		t.Errorf("Locate() = %+v, %v", ref, err)
	}
	if err := s.DeleteWorkflow(ctx, a.ID, def.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Workflow(ctx, a.ID, def.ID); !errors.Is(err, invoicing.ErrWorkflowNotFound) {
		t.Errorf("Workflow() after delete error = %v", err)
	}
}

func TestDocumentStyleStore(t *testing.T) {
	ctx := context.Background()
	s := postgres.NewInvoicingStore(testPool)
	admins := postgres.NewAdministrationStore(testPool)
	a, def, _ := newAdministrationWithIdentity(t)
	sender := def
	sender.ID, sender.Default, sender.CompanyName = ids.Next(), false, "Trade"
	if err := admins.CreateIdentity(ctx, sender); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	d := invoicing.DocumentStyle{
		ID: ids.Next(), AdministrationID: a.ID, Name: "The Best", IdentityID: sender.ID, Default: true,
		Logo: &invoicing.StoredFile{Hash: "abc", ContentType: "image/png"}, LogoDisplayWidth: 40, LogoPosition: "center",
		PaperSize: "letter", AddressPosition: "right", FontSize: 11, FontFamily: "georgia", CustomCSS: "p{}",
		Layout: invoicing.Layout{
			InvoiceSenderAddress: []invoicing.Line{{Field: "company_name", Bold: true}, {Field: "tax_number", Label: true}},
			InvoiceMetadataLeft:  []invoicing.Line{}, InvoiceMetadataRight: []invoicing.Line{{Field: "invoice_date"}},
			EstimateSenderAddress: []invoicing.Line{}, EstimateMetadataLeft: []invoicing.Line{}, EstimateMetadataRight: []invoicing.Line{},
			InvoiceRecipientAddress:  []invoicing.Line{{Field: "customer_tax_number", Italic: true, Label: true}},
			EstimateRecipientAddress: []invoicing.Line{}, HideSenderAddress: true, SenderAddressWidth: 240, ShowEPCCode: false,
		},
		Email: invoicing.EmailStyle{
			HeaderType: "background_color_with_logo", HeaderContentPosition: "right", HeaderBackgroundColor: "#112233", HeaderText: "Hi",
			HeaderTextColor: "#FFFFFF", LogoDisplayWidth: 30, FooterType: "signature", FooterTextColor: "#000000",
			FooterBackgroundColor: "#EEEEEE", FooterProfileTitle: "Ann", FooterProfileSubtitle: "Owner", BodyBackgroundColor: "#FAFAFA",
			FontFamily: "arial",
		},
		PublicView: invoicing.PublicViewStyle{LogoDisplaySize: 80, BackgroundColor: "#D5E8FA"},
		Checkout:   invoicing.CheckoutStyle{ShowPhoneField: true, FavoriteLabel: "Best"},
		Files:      map[invoicing.FileKind]invoicing.StoredFile{invoicing.EmailLogo: {Hash: "def", ContentType: "image/gif"}},
		Images:     []invoicing.StoredFile{{Hash: "ghi", ContentType: "image/jpeg"}},
		CreatedAt:  now, UpdatedAt: now,
	}
	if created, err := s.CreateDocumentStyle(ctx, d); err != nil || !created {
		t.Fatalf("CreateDocumentStyle() = %v, %v", created, err)
	}
	got, err := s.DocumentStyle(ctx, a.ID, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(d, got, approxTime); diff != "" {
		t.Errorf("DocumentStyle() diff (-want +got):\n%s", diff)
	}
	for _, hash := range []string{"abc", "def", "ghi"} {
		if used, err := s.FileInUse(ctx, a.ID, hash); err != nil || !used {
			t.Errorf("FileInUse(%s) = %v, %v", hash, used, err)
		}
	}
	if _, err := testPool.Exec(ctx, `UPDATE document_styles SET layout = '{}', channels = '{}' WHERE id = $1`, int64(d.ID)); err != nil {
		t.Fatal(err)
	}
	old, err := s.DocumentStyle(ctx, a.ID, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if old.Email != invoicing.DefaultEmailStyle() || old.PublicView != invoicing.DefaultPublicViewStyle() || old.Checkout != invoicing.DefaultCheckoutStyle() || !old.ShowEPCCode || old.SenderAddressWidth != 200 {
		t.Errorf("a style stored before the settings existed = %+v", old)
	}
	if used, _ := s.FileInUse(ctx, a.ID, "other"); used {
		t.Error("FileInUse(other) = true")
	}

	if err := admins.DeleteIdentity(ctx, a.ID, sender.ID); err != nil {
		t.Fatalf("DeleteIdentity() of a sender used by a style error = %v", err)
	}
	if got, _ := s.DocumentStyle(ctx, a.ID, d.ID); got.IdentityID != def.ID {
		t.Errorf("style identity after deleting its sender = %v, want the default %v", got.IdentityID, def.ID)
	}

	if err := s.DeleteDocumentStyle(ctx, a.ID, d.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DocumentStyle(ctx, a.ID, d.ID); !errors.Is(err, invoicing.ErrDocumentStyleNotFound) {
		t.Errorf("DocumentStyle() after delete error = %v", err)
	}
}

func TestCustomFieldUpdateAndDelete(t *testing.T) {
	ctx := context.Background()
	s := postgres.NewAdministrationStore(testPool)
	a, _, _ := newAdministrationWithIdentity(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	f := administration.CustomField{ID: ids.Next(), AdministrationID: a.ID, Name: "Slogan", Source: administration.SourceIdentity, Position: 1, CreatedAt: now, UpdatedAt: now}
	if err := s.CreateCustomField(ctx, f); err != nil {
		t.Fatal(err)
	}
	f.Name, f.Position = "Motto", 3
	if err := s.UpdateCustomField(ctx, f); err != nil {
		t.Fatal(err)
	}
	if got, err := s.CustomField(ctx, a.ID, f.ID); err != nil || got.Name != "Motto" || got.Position != 3 {
		t.Errorf("CustomField() = %+v, %v", got, err)
	}
	if err := s.DeleteCustomField(ctx, a.ID, f.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteCustomField(ctx, a.ID, f.ID); !errors.Is(err, administration.ErrCustomFieldNotFound) {
		t.Errorf("DeleteCustomField() twice error = %v", err)
	}
}
