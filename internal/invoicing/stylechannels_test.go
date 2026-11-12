package invoicing

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

func defaultStyle(t *testing.T, svc *Service) DocumentStyle {
	t.Helper()
	list, err := svc.DocumentStyles(context.Background(), adminID, owner)
	if err != nil {
		t.Fatal(err)
	}
	return list[0]
}

func TestDefaultStyleChannels(t *testing.T) {
	svc, _, _ := newTestService()
	d := defaultStyle(t, svc)
	if d.Email != DefaultEmailStyle() || d.PublicView != DefaultPublicViewStyle() || d.Checkout != DefaultCheckoutStyle() {
		t.Errorf("channels = %+v %+v %+v", d.Email, d.PublicView, d.Checkout)
	}
	if !d.Email.LabelEnabled || d.Email.HeaderBackgroundColor != "#F7F9FC" || d.Email.FontFamily != "lucida_grande" || d.Email.LogoDisplayWidth != 50 {
		t.Errorf("email = %+v", d.Email)
	}
	if !d.Checkout.ShowAddress || d.PublicView.BackgroundColor != "#F7F9FC" || d.PublicView.LogoDisplaySize != 50 || !d.PublicView.LabelEnabled {
		t.Errorf("public view %+v, checkout %+v", d.PublicView, d.Checkout)
	}
	if !d.ShowEPCCode || d.SenderAddressWidth != 200 || d.HideSenderAddress || d.InvoiceRecipientAddress == nil {
		t.Errorf("layout = %+v", d.Layout)
	}
}

func TestUpdateStyleChannels(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	d := defaultStyle(t, svc)
	got, err := svc.UpdateDocumentStyle(ctx, adminID, d.ID, owner, DocumentStyleChanges{
		HideSenderAddress: ptr(true), SenderAddressWidth: ptr(260), ShowEPCCode: ptr(false),
		InvoiceRecipientAddress: &[]Line{{Field: "customer_tax_number", Label: true}, {Field: "custom_field_702"}},
		InvoiceSenderAddress:    &[]Line{{Field: "custom_field_700", Bold: true, Italic: true}, {Field: "address"}},
		InvoiceMetadataLeft:     &[]Line{{Field: "customer_email"}, {Field: "custom_field_701"}},
		EstimateMetadataRight:   &[]Line{{Field: "custom_field_703"}, {Field: "customer_phone"}},
		Email: EmailChanges{
			HeaderType: ptr("background_color_with_text"), HeaderContentPosition: ptr("center"), HeaderBackgroundColor: ptr("#112233"),
			HeaderText: ptr("Thanks for your order"), HeaderTextColor: ptr("#ffffff"), LogoDisplayWidth: ptr(30),
			FooterType: ptr("signature"), FooterTextColor: ptr("#000000"), FooterBackgroundColor: ptr("#FFFFFF"),
			FooterProfileTitle: ptr("Ann de Vries"), FooterProfileSubtitle: ptr("Owner"), BodyBackgroundColor: ptr("#EEEEEE"),
			FontFamily: ptr("georgia"), LabelEnabled: ptr(false),
		},
		PublicView: PublicViewChanges{LogoDisplaySize: ptr(70), BackgroundColor: ptr("#D5E8FA"), LabelEnabled: ptr(false)},
		Checkout: CheckoutChanges{
			ShowAddress: ptr(false), ShowPhoneField: ptr(true), ForBusiness: ptr(true), ShowIdentity: ptr(true),
			ShowWithdrawalButton: ptr(true), FavoriteLabel: ptr("Most chosen"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !got.HideSenderAddress || got.SenderAddressWidth != 260 || got.ShowEPCCode || got.InvoiceRecipientAddress[1].Field != "custom_field_702" || !got.InvoiceSenderAddress[0].Italic {
		t.Errorf("layout = %+v", got.Layout)
	}
	if got.Email.HeaderType != "background_color_with_text" || got.Email.HeaderTextColor != "#FFFFFF" || got.Email.FontFamily != "georgia" || got.Email.LabelEnabled || got.Email.FooterProfileTitle != "Ann de Vries" {
		t.Errorf("email = %+v", got.Email)
	}
	if got.PublicView != (PublicViewStyle{LogoDisplaySize: 70, BackgroundColor: "#D5E8FA"}) {
		t.Errorf("public view = %+v", got.PublicView)
	}
	if got.Checkout != (CheckoutStyle{ShowPhoneField: true, ForBusiness: true, ShowIdentity: true, ShowWithdrawalButton: true, FavoriteLabel: "Most chosen"}) {
		t.Errorf("checkout = %+v", got.Checkout)
	}
}

func TestStyleChannelValidation(t *testing.T) {
	svc, _, _ := newTestService()
	d := defaultStyle(t, svc)
	_, err := svc.UpdateDocumentStyle(context.Background(), adminID, d.ID, owner, DocumentStyleChanges{
		SenderAddressWidth:       ptr(20),
		InvoiceRecipientAddress:  &[]Line{{Field: "invoice_id"}},
		EstimateSenderAddress:    &[]Line{{Field: "custom_field_701"}},
		EstimateRecipientAddress: &[]Line{{Field: "custom_field_999"}},
		InvoiceMetadataLeft:      &[]Line{{Field: "custom_field_703"}},
		Email: EmailChanges{
			HeaderType: ptr("video"), HeaderContentPosition: ptr("top"), HeaderBackgroundColor: ptr("red"),
			HeaderTextColor: ptr("#12345"), LogoDisplayWidth: ptr(500), FooterType: ptr("banner"),
			FooterTextColor: ptr("url(x)"), FooterBackgroundColor: ptr("#GGGGGG"), BodyBackgroundColor: ptr(""),
			FontFamily: ptr("comic_sans"), HeaderText: ptr(strings.Repeat("x", 501)),
			FooterProfileTitle: ptr(strings.Repeat("x", 101)), FooterProfileSubtitle: ptr(strings.Repeat("x", 101)),
		},
		PublicView: PublicViewChanges{LogoDisplaySize: ptr(5), BackgroundColor: ptr("#000000")},
		Checkout:   CheckoutChanges{FavoriteLabel: ptr("a label that is far too long")},
	})
	fields := fieldsOf(t, err)
	for _, f := range []string{
		"sender_address_width", "invoice_recipient_address", "estimate_sender_address", "estimate_recipient_address", "invoice_metadata_left",
		"email_header_type", "email_header_content_position", "email_header_background_color", "email_header_text_color",
		"email_logo_display_width", "email_footer_type", "email_footer_text_color", "email_footer_background_color",
		"body_background_color", "email_font_family", "email_header_text", "email_footer_profile_title", "email_footer_profile_subtitle",
		"public_view_logo_display_size", "public_view_background_color", "checkout_favorite_label",
	} {
		if fields[f] == nil {
			t.Errorf("no error for %s; fields = %v", f, fields)
		}
	}
}

func TestChannelFiles(t *testing.T) {
	svc, store, files := newTestService()
	ctx := context.Background()
	d := defaultStyle(t, svc)
	for _, kind := range []FileKind{EmailLogo, EmailHeaderBackground, EmailFooterProfilePicture, EmailFooterBackground, PublicViewLogo} {
		got, err := svc.UploadFile(ctx, adminID, d.ID, owner, kind, png)
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if f := got.File(kind); f == nil || f.ContentType != "image/png" {
			t.Errorf("%s: file = %+v", kind, f)
		}
		if _, err := svc.UploadFile(ctx, adminID, d.ID, owner, kind, []byte("%PDF-1.7")); fieldsOf(t, err)[string(kind)] == nil {
			t.Errorf("%s: accepted a PDF", kind)
		}
	}
	if len(files.data) != 1 {
		t.Errorf("one image used five times is stored %d times", len(files.data))
	}
	if used, _ := store.FileInUse(ctx, adminID, store.styles[d.ID].File(EmailLogo).Hash); !used {
		t.Error("FileInUse() = false for an email logo")
	}
	for _, kind := range []FileKind{EmailLogo, EmailHeaderBackground, EmailFooterProfilePicture, EmailFooterBackground} {
		if _, err := svc.RemoveFile(ctx, adminID, d.ID, owner, kind); err != nil {
			t.Fatal(err)
		}
	}
	if len(files.data) != 1 {
		t.Error("removed a file the public view still uses")
	}
	if _, err := svc.RemoveFile(ctx, adminID, d.ID, owner, PublicViewLogo); err != nil {
		t.Fatal(err)
	}
	if len(files.data) != 0 {
		t.Errorf("files left: %d", len(files.data))
	}
	if _, err := svc.UploadFile(ctx, adminID, d.ID, owner, "passwords", png); err == nil {
		t.Error("accepted an unknown file kind")
	}
}

func TestCSSImages(t *testing.T) {
	svc, _, files := newTestService()
	ctx := context.Background()
	d := defaultStyle(t, svc)
	got, err := svc.AddImage(ctx, adminID, d.ID, owner, png)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Images) != 1 || got.Images[0].ContentType != "image/png" {
		t.Fatalf("Images = %+v", got.Images)
	}
	if again, err := svc.AddImage(ctx, adminID, d.ID, owner, png); err != nil || len(again.Images) != 1 {
		t.Errorf("adding the same image twice: %+v, %v", again.Images, err)
	}
	hash := got.Images[0].Hash
	body, contentType, err := svc.OpenImage(ctx, adminID, d.ID, owner, hash)
	if err != nil || contentType != "image/png" {
		t.Fatalf("OpenImage() = %s, %v", contentType, err)
	}
	_ = body.Close()
	if _, _, err := svc.OpenImage(ctx, adminID, d.ID, owner, "0000"); !errors.Is(err, ErrFileNotFound) {
		t.Errorf("OpenImage(unknown) error = %v", err)
	}
	if _, err := svc.AddImage(ctx, adminID, d.ID, owner, []byte("<svg onload=alert(1)>")); fieldsOf(t, err)["image"] == nil {
		t.Error("accepted a non-image")
	}
	if got, err = svc.RemoveImage(ctx, adminID, d.ID, owner, hash); err != nil || len(got.Images) != 0 {
		t.Errorf("RemoveImage() = %+v, %v", got.Images, err)
	}
	if len(files.data) != 0 {
		t.Errorf("files left: %d", len(files.data))
	}
	if _, err := svc.RemoveImage(ctx, adminID, d.ID, owner, hash); !errors.Is(err, ErrFileNotFound) {
		t.Errorf("RemoveImage(gone) error = %v", err)
	}
	for i := range maxImages {
		if _, err := svc.AddImage(ctx, adminID, d.ID, owner, append(bytes.Clone(png), byte(i))); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.AddImage(ctx, adminID, d.ID, owner, append(bytes.Clone(png), 0xff)); fieldsOf(t, err)["image"] == nil {
		t.Error("accepted one image too many")
	}
	if _, err := svc.AddImage(ctx, adminID, resource.ID(1), owner, png); !errors.Is(err, ErrDocumentStyleNotFound) {
		t.Errorf("AddImage(unknown style) error = %v", err)
	}
}
