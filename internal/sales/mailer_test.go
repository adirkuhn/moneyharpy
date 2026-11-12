package sales

import (
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/invoicing"
)

func TestMailLookCarriesTheStyle(t *testing.T) {
	if mailLook(nil) != nil {
		t.Error("mailLook(nil) != nil")
	}
	style := invoicing.DefaultEmailStyle()
	style.HeaderType, style.FontFamily, style.FooterType = "background_image_with_logo", "courier", "contact_information"
	got := mailLook(&EmailLook{
		Style: style, Language: "nl", Details: [][2]string{{"Factuur", "2026-0001"}},
		Sender: administration.Identity{CompanyName: "Example Shop B.V.", Address1: "Main Street 1", Zipcode: "1234 AB", City: "Amsterdam", Email: "info@example.com"},
		Images: map[invoicing.FileKind]File{invoicing.EmailLogo: {Name: "logo", ContentType: "image/png", Data: []byte("png")}},
	})
	if got.HeaderType != "background_image_with_logo" || got.FontStack != FontStacks["courier"] || got.Label != "Verzonden met MoneyHarpy" || got.PageBackground != "#F7F9FC" {
		t.Errorf("look = %+v", got)
	}
	if got.Logo == nil || string(got.Logo.Data) != "png" || got.HeaderImage != nil {
		t.Errorf("images: logo %+v, header %+v", got.Logo, got.HeaderImage)
	}
	if len(got.ContactLines) != 4 || got.ContactLines[2] != "1234 AB Amsterdam" || got.Details[0][1] != "2026-0001" {
		t.Errorf("contact %v, details %v", got.ContactLines, got.Details)
	}
	style.LabelEnabled = false
	if l := mailLook(&EmailLook{Style: style}); l.Label != "" {
		t.Errorf("label = %q with the label off", l.Label)
	}
}
