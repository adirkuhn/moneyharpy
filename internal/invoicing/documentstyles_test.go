package invoicing

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

var png = append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)

func TestDefaultDocumentStyle(t *testing.T) {
	svc, _, _ := newTestService()
	got, err := svc.DocumentStyles(context.Background(), adminID, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !got[0].Default || got[0].Name != "Standaard" || got[0].IdentityID != 500 || got[0].PaperSize != "A4" {
		t.Fatalf("DocumentStyles() = %+v", got)
	}
	if got[0].InvoiceSenderAddress[0] != (Line{Field: "company_name", Bold: true}) {
		t.Errorf("sender address = %+v", got[0].InvoiceSenderAddress)
	}
}

func TestDocumentStyleLifecycle(t *testing.T) {
	svc, _, files := newTestService()
	ctx := context.Background()
	d, err := svc.CreateDocumentStyle(ctx, adminID, owner, DocumentStyleChanges{
		Name: ptr("The Best"), IdentityID: ptr(resource.ID(501)), PaperSize: ptr("letter"),
		InvoiceMetadataRight: &[]Line{{Field: "invoice_id", Bold: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.Default || d.IdentityID != 501 || d.PaperSize != "letter" || d.InvoiceMetadataRight[0].Field != "invoice_id" {
		t.Errorf("CreateDocumentStyle() = %+v", d)
	}
	if d.Ref().URI() != "/11/document_styles/"+d.ID.String() {
		t.Errorf("URI = %s", d.Ref().URI())
	}

	d, err = svc.UploadFile(ctx, adminID, d.ID, owner, Logo, png)
	if err != nil {
		t.Fatal(err)
	}
	if d.Logo == nil || d.Logo.ContentType != "image/png" || len(files.data) != 1 {
		t.Fatalf("UploadFile() logo = %+v, files %d", d.Logo, len(files.data))
	}
	body, contentType, err := svc.OpenFile(ctx, adminID, d.ID, owner, Logo)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(body)
	if !bytes.Equal(got, png) || contentType != "image/png" {
		t.Errorf("OpenFile() = %d bytes, %s", len(got), contentType)
	}

	other := append(bytes.Clone(png), 1)
	if d, err = svc.UploadFile(ctx, adminID, d.ID, owner, Logo, other); err != nil {
		t.Fatal(err)
	}
	if len(files.data) != 1 {
		t.Errorf("replacing the logo kept %d files, want the old one removed", len(files.data))
	}

	if _, err := svc.RemoveFile(ctx, adminID, d.ID, owner, Logo); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.OpenFile(ctx, adminID, d.ID, owner, Logo); !errors.Is(err, ErrFileNotFound) {
		t.Errorf("OpenFile() after remove error = %v", err)
	}
	if len(files.data) != 0 {
		t.Errorf("removing the logo kept %d files", len(files.data))
	}

	if err := svc.DeleteDocumentStyle(ctx, adminID, d.ID, owner); err != nil {
		t.Errorf("DeleteDocumentStyle() error = %v", err)
	}
	list, _ := svc.DocumentStyles(ctx, adminID, owner)
	if err := svc.DeleteDocumentStyle(ctx, adminID, list[0].ID, owner); !errors.Is(err, ErrDefaultInUse) {
		t.Errorf("DeleteDocumentStyle(default) error = %v", err)
	}
}

func TestDocumentStyleValidation(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	_, err := svc.CreateDocumentStyle(ctx, adminID, owner, DocumentStyleChanges{
		Name: ptr(""), IdentityID: ptr(resource.ID(999)), FontSize: ptr(30), FontFamily: ptr("comic_sans"),
		LogoPosition: ptr("top"), InvoiceSenderAddress: &[]Line{{Field: "password"}},
	})
	fields := fieldsOf(t, err)
	for _, f := range []string{"name", "identity_id", "font_size", "font_family", "logo_position", "invoice_sender_address"} {
		if fields[f] == nil {
			t.Errorf("fields = %v, want %s", fields, f)
		}
	}
}

func TestUploadRejectsWrongFiles(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	list, err := svc.DocumentStyles(ctx, adminID, owner)
	if err != nil {
		t.Fatal(err)
	}
	id := list[0].ID
	for name, data := range map[string][]byte{
		"html":     []byte("<html><script>alert(1)</script></html>"),
		"empty":    nil,
		"too big":  append(bytes.Clone(png), make([]byte, 3<<20)...),
		"pdf logo": []byte("%PDF-1.7 ..."),
	} {
		if _, err := svc.UploadFile(ctx, adminID, id, owner, Logo, data); fieldsOf(t, err)["logo"] == nil {
			t.Errorf("%s: accepted as a logo", name)
		}
	}
	if _, err := svc.UploadFile(ctx, adminID, id, owner, Background, []byte("%PDF-1.7 ...")); err != nil {
		t.Errorf("PDF background error = %v", err)
	}
}
