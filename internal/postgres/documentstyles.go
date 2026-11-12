package postgres

import (
	"encoding/json"
	"fmt"

	"github.com/adirkuhn/moneyharpy/internal/invoicing"
)

// line and layout are how Layout is stored, in Moneybird's JSON shape.
type line struct {
	Field  string `json:"field"`
	Bold   bool   `json:"bold,omitempty"`
	Italic bool   `json:"italic,omitempty"`
	Label  bool   `json:"label,omitempty"`
}

type layout struct {
	InvoiceSenderAddress     []line `json:"invoice_sender_address"`
	InvoiceMetadataLeft      []line `json:"invoice_metadata_left"`
	InvoiceMetadataRight     []line `json:"invoice_metadata_right"`
	EstimateSenderAddress    []line `json:"estimate_sender_address"`
	EstimateMetadataLeft     []line `json:"estimate_metadata_left"`
	EstimateMetadataRight    []line `json:"estimate_metadata_right"`
	InvoiceRecipientAddress  []line `json:"invoice_recipient_address"`
	EstimateRecipientAddress []line `json:"estimate_recipient_address"`
	HideSenderAddress        bool   `json:"hide_sender_address"`
	// SenderAddressWidth and ShowEPCCode are pointers so styles stored before
	// they existed take the defaults.
	SenderAddressWidth *int  `json:"sender_address_width"`
	ShowEPCCode        *bool `json:"show_epc_code"`
}

func toLines(in []invoicing.Line) []line {
	out := make([]line, len(in))
	for i, l := range in {
		out[i] = line(l)
	}
	return out
}

func fromLines(in []line) []invoicing.Line {
	out := make([]invoicing.Line, len(in))
	for i, l := range in {
		out[i] = invoicing.Line(l)
	}
	return out
}

func encodeLayout(l invoicing.Layout) ([]byte, error) {
	return json.Marshal(layout{
		toLines(l.InvoiceSenderAddress), toLines(l.InvoiceMetadataLeft), toLines(l.InvoiceMetadataRight),
		toLines(l.EstimateSenderAddress), toLines(l.EstimateMetadataLeft), toLines(l.EstimateMetadataRight),
		toLines(l.InvoiceRecipientAddress), toLines(l.EstimateRecipientAddress), l.HideSenderAddress,
		&l.SenderAddressWidth, &l.ShowEPCCode,
	})
}

func decodeLayout(raw []byte) (invoicing.Layout, error) {
	var l layout
	if err := json.Unmarshal(raw, &l); err != nil {
		return invoicing.Layout{}, fmt.Errorf("decode layout: %w", err)
	}
	width, epc := 200, true
	if l.SenderAddressWidth != nil {
		width = *l.SenderAddressWidth
	}
	if l.ShowEPCCode != nil {
		epc = *l.ShowEPCCode
	}
	return invoicing.Layout{
		InvoiceSenderAddress: fromLines(l.InvoiceSenderAddress), InvoiceMetadataLeft: fromLines(l.InvoiceMetadataLeft),
		InvoiceMetadataRight: fromLines(l.InvoiceMetadataRight), EstimateSenderAddress: fromLines(l.EstimateSenderAddress),
		EstimateMetadataLeft: fromLines(l.EstimateMetadataLeft), EstimateMetadataRight: fromLines(l.EstimateMetadataRight),
		InvoiceRecipientAddress: fromLines(l.InvoiceRecipientAddress), EstimateRecipientAddress: fromLines(l.EstimateRecipientAddress),
		HideSenderAddress: l.HideSenderAddress, SenderAddressWidth: width, ShowEPCCode: epc,
	}, nil
}

// channels is how the email, public view and checkout settings are stored,
// under the names of the API.
type channels struct {
	EmailHeaderType              string `json:"email_header_type"`
	EmailHeaderContentPosition   string `json:"email_header_content_position"`
	EmailHeaderBackgroundColor   string `json:"email_header_background_color"`
	EmailHeaderText              string `json:"email_header_text"`
	EmailHeaderTextColor         string `json:"email_header_text_color"`
	EmailLogoDisplayWidth        int    `json:"email_logo_display_width"`
	EmailFooterType              string `json:"email_footer_type"`
	EmailFooterTextColor         string `json:"email_footer_text_color"`
	EmailFooterBackgroundColor   string `json:"email_footer_background_color"`
	EmailFooterProfileTitle      string `json:"email_footer_profile_title"`
	EmailFooterProfileSubtitle   string `json:"email_footer_profile_subtitle"`
	BodyBackgroundColor          string `json:"body_background_color"`
	EmailFontFamily              string `json:"email_font_family"`
	EmailLabelEnabled            bool   `json:"email_moneybird_label_enabled"`
	PublicViewLogoDisplaySize    int    `json:"public_view_logo_display_size"`
	PublicViewBackgroundColor    string `json:"public_view_background_color"`
	PublicViewLabelEnabled       bool   `json:"public_view_moneybird_label_enabled"`
	CheckoutShowAddress          bool   `json:"checkout_show_address"`
	CheckoutShowPhoneField       bool   `json:"checkout_show_phone_field"`
	CheckoutForBusiness          bool   `json:"checkout_for_business"`
	CheckoutShowIdentity         bool   `json:"checkout_show_identity"`
	CheckoutShowWithdrawalButton bool   `json:"checkout_show_withdrawal_button"`
	CheckoutFavoriteLabel        string `json:"checkout_favorite_label"`
}

func toChannels(d invoicing.DocumentStyle) channels {
	e, p, c := d.Email, d.PublicView, d.Checkout
	return channels{
		e.HeaderType, e.HeaderContentPosition, e.HeaderBackgroundColor, e.HeaderText, e.HeaderTextColor, e.LogoDisplayWidth,
		e.FooterType, e.FooterTextColor, e.FooterBackgroundColor, e.FooterProfileTitle, e.FooterProfileSubtitle,
		e.BodyBackgroundColor, e.FontFamily, e.LabelEnabled,
		p.LogoDisplaySize, p.BackgroundColor, p.LabelEnabled,
		c.ShowAddress, c.ShowPhoneField, c.ForBusiness, c.ShowIdentity, c.ShowWithdrawalButton, c.FavoriteLabel,
	}
}

func encodeChannels(d invoicing.DocumentStyle) ([]byte, error) {
	return json.Marshal(toChannels(d))
}

// decodeChannels fills d's settings; keys a style was stored without keep
// their defaults.
func decodeChannels(raw []byte, d *invoicing.DocumentStyle) error {
	d.Email, d.PublicView, d.Checkout = invoicing.DefaultEmailStyle(), invoicing.DefaultPublicViewStyle(), invoicing.DefaultCheckoutStyle()
	c := toChannels(*d)
	if err := json.Unmarshal(raw, &c); err != nil {
		return fmt.Errorf("decode channels: %w", err)
	}
	d.Email = invoicing.EmailStyle{
		HeaderType: c.EmailHeaderType, HeaderContentPosition: c.EmailHeaderContentPosition, HeaderBackgroundColor: c.EmailHeaderBackgroundColor,
		HeaderText: c.EmailHeaderText, HeaderTextColor: c.EmailHeaderTextColor, LogoDisplayWidth: c.EmailLogoDisplayWidth,
		FooterType: c.EmailFooterType, FooterTextColor: c.EmailFooterTextColor, FooterBackgroundColor: c.EmailFooterBackgroundColor,
		FooterProfileTitle: c.EmailFooterProfileTitle, FooterProfileSubtitle: c.EmailFooterProfileSubtitle,
		BodyBackgroundColor: c.BodyBackgroundColor, FontFamily: c.EmailFontFamily, LabelEnabled: c.EmailLabelEnabled,
	}
	d.PublicView = invoicing.PublicViewStyle{LogoDisplaySize: c.PublicViewLogoDisplaySize, BackgroundColor: c.PublicViewBackgroundColor, LabelEnabled: c.PublicViewLabelEnabled}
	d.Checkout = invoicing.CheckoutStyle{
		ShowAddress: c.CheckoutShowAddress, ShowPhoneField: c.CheckoutShowPhoneField, ForBusiness: c.CheckoutForBusiness,
		ShowIdentity: c.CheckoutShowIdentity, ShowWithdrawalButton: c.CheckoutShowWithdrawalButton, FavoriteLabel: c.CheckoutFavoriteLabel,
	}
	return nil
}

// styleFile and styleFiles are how the files beyond the logo and background
// are stored. FileInUse finds them by their "hash" keys.
type styleFile struct {
	Hash        string `json:"hash"`
	ContentType string `json:"content_type"`
}

type styleFiles struct {
	Kinds  map[string]styleFile `json:"kinds,omitempty"`
	Images []styleFile          `json:"images,omitempty"`
}

func encodeFiles(d invoicing.DocumentStyle) ([]byte, error) {
	out := styleFiles{Kinds: map[string]styleFile{}}
	for kind, f := range d.Files {
		out.Kinds[string(kind)] = styleFile(f)
	}
	for _, f := range d.Images {
		out.Images = append(out.Images, styleFile(f))
	}
	return json.Marshal(out)
}

func decodeFiles(raw []byte, d *invoicing.DocumentStyle) error {
	var in styleFiles
	if err := json.Unmarshal(raw, &in); err != nil {
		return fmt.Errorf("decode files: %w", err)
	}
	if len(in.Kinds) > 0 {
		d.Files = make(map[invoicing.FileKind]invoicing.StoredFile, len(in.Kinds))
		for kind, f := range in.Kinds {
			d.Files[invoicing.FileKind(kind)] = invoicing.StoredFile(f)
		}
	}
	for _, f := range in.Images {
		d.Images = append(d.Images, invoicing.StoredFile(f))
	}
	return nil
}
