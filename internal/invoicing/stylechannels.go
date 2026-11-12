package invoicing

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// A document style also styles the other ways a document reaches the
// customer, as Moneybird's editor does on its Email, Public views and Online
// sales tabs. Moneybird's public API has none of these fields, so MoneyHarpy
// names them after Moneybird's editor form.

// Choices for the email, public view and checkout settings.
var (
	EmailHeaderTypes = []string{"", "background_image_with_text", "background_image_with_logo", "background_image", "background_color_with_logo", "background_color_with_text"}
	EmailFooterTypes = []string{"", "contact_information", "signature", "background_image"}
	// PublicViewBackgroundColors are the page colours a public view offers,
	// light grey first as the default.
	PublicViewBackgroundColors = []string{"#F7F9FC", "#F6E4E2", "#EDE8F7", "#D5E8FA", "#DEF4F8", "#FBE0D5", "#FCF1DB", "#D7ECEA", "#E7F3E6", "#E4E9EB"}
)

const (
	maxHeaderText, maxProfileText, maxFavoriteLabel = 500, 100, 20
	minSenderWidth, maxSenderWidth                  = 100, 400
)

var hexColor = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// EmailStyle is how the emails that carry documents look.
type EmailStyle struct {
	// HeaderType is one of EmailHeaderTypes; empty means no header.
	HeaderType            string
	HeaderContentPosition string
	HeaderBackgroundColor string
	HeaderText            string
	HeaderTextColor       string
	LogoDisplayWidth      int
	// FooterType is one of EmailFooterTypes; empty means no footer.
	FooterType            string
	FooterTextColor       string
	FooterBackgroundColor string
	FooterProfileTitle    string
	FooterProfileSubtitle string
	BodyBackgroundColor   string
	FontFamily            string
	// LabelEnabled shows "Sent with MoneyHarpy" under the email.
	LabelEnabled bool
}

// PublicViewStyle is how the online view of a document looks.
type PublicViewStyle struct {
	LogoDisplaySize int
	BackgroundColor string
	LabelEnabled    bool
}

// CheckoutStyle is what the online sales checkout asks and shows.
type CheckoutStyle struct {
	ShowAddress          bool
	ShowPhoneField       bool
	ForBusiness          bool
	ShowIdentity         bool
	ShowWithdrawalButton bool
	FavoriteLabel        string
}

// DefaultEmailStyle is the email look of a new document style.
func DefaultEmailStyle() EmailStyle {
	return EmailStyle{
		HeaderContentPosition: "left", HeaderBackgroundColor: "#F7F9FC", HeaderTextColor: "#465569", LogoDisplayWidth: 50,
		FooterTextColor: "#465569", FooterBackgroundColor: "#F7FAFC", BodyBackgroundColor: "#F7F9FC", FontFamily: "lucida_grande",
		LabelEnabled: true,
	}
}

// DefaultPublicViewStyle is the public view look of a new document style.
func DefaultPublicViewStyle() PublicViewStyle {
	return PublicViewStyle{LogoDisplaySize: 50, BackgroundColor: PublicViewBackgroundColors[0], LabelEnabled: true}
}

// DefaultCheckoutStyle is the checkout of a new document style.
func DefaultCheckoutStyle() CheckoutStyle { return CheckoutStyle{ShowAddress: true} }

// EmailChanges holds the email settings to set. Nil fields stay as they are.
type EmailChanges struct {
	HeaderType, HeaderContentPosition, HeaderBackgroundColor, HeaderText, HeaderTextColor *string
	LogoDisplayWidth                                                                      *int
	FooterType, FooterTextColor, FooterBackgroundColor                                    *string
	FooterProfileTitle, FooterProfileSubtitle, BodyBackgroundColor, FontFamily            *string
	LabelEnabled                                                                          *bool
}

// PublicViewChanges holds the public view settings to set.
type PublicViewChanges struct {
	LogoDisplaySize *int
	BackgroundColor *string
	LabelEnabled    *bool
}

// CheckoutChanges holds the checkout settings to set.
type CheckoutChanges struct {
	ShowAddress, ShowPhoneField, ForBusiness, ShowIdentity, ShowWithdrawalButton *bool
	FavoriteLabel                                                                *string
}

// applyChannels sets the email, public view and checkout changes, reporting
// each refused field under its API name.
func applyChannels(d *DocumentStyle, c DocumentStyleChanges, verr *resource.ValidationError) {
	e, p, k := c.Email, c.PublicView, c.Checkout
	choice := func(field string, dst, src *string, choices []string) {
		if src == nil {
			return
		}
		if !slices.Contains(choices, *src) {
			verr.Add(field, "is not included in the list")
			return
		}
		*dst = *src
	}
	color := func(field string, dst, src *string) {
		if src == nil {
			return
		}
		if !hexColor.MatchString(*src) {
			verr.Add(field, "must be a colour such as #F7F9FC")
			return
		}
		*dst = strings.ToUpper(*src)
	}
	text := func(field string, dst, src *string, limit int) {
		if src == nil {
			return
		}
		if utf8.RuneCountInString(*src) > limit {
			verr.Add(field, fmt.Sprintf("is too long (maximum is %d characters)", limit))
			return
		}
		*dst = strings.TrimSpace(*src)
	}
	number := func(field string, dst, src *int, lo, hi int) {
		if src == nil {
			return
		}
		if *src < lo || *src > hi {
			verr.Add(field, fmt.Sprintf("must be between %d and %d", lo, hi))
			return
		}
		*dst = *src
	}
	flag := func(dst, src *bool) {
		if src != nil {
			*dst = *src
		}
	}

	choice("email_header_type", &d.Email.HeaderType, e.HeaderType, EmailHeaderTypes)
	choice("email_header_content_position", &d.Email.HeaderContentPosition, e.HeaderContentPosition, LogoPositions)
	color("email_header_background_color", &d.Email.HeaderBackgroundColor, e.HeaderBackgroundColor)
	text("email_header_text", &d.Email.HeaderText, e.HeaderText, maxHeaderText)
	color("email_header_text_color", &d.Email.HeaderTextColor, e.HeaderTextColor)
	number("email_logo_display_width", &d.Email.LogoDisplayWidth, e.LogoDisplayWidth, minLogoWidth, maxLogoWidth)
	choice("email_footer_type", &d.Email.FooterType, e.FooterType, EmailFooterTypes)
	color("email_footer_text_color", &d.Email.FooterTextColor, e.FooterTextColor)
	color("email_footer_background_color", &d.Email.FooterBackgroundColor, e.FooterBackgroundColor)
	text("email_footer_profile_title", &d.Email.FooterProfileTitle, e.FooterProfileTitle, maxProfileText)
	text("email_footer_profile_subtitle", &d.Email.FooterProfileSubtitle, e.FooterProfileSubtitle, maxProfileText)
	color("body_background_color", &d.Email.BodyBackgroundColor, e.BodyBackgroundColor)
	choice("email_font_family", &d.Email.FontFamily, e.FontFamily, FontFamilies)
	flag(&d.Email.LabelEnabled, e.LabelEnabled)

	number("public_view_logo_display_size", &d.PublicView.LogoDisplaySize, p.LogoDisplaySize, minLogoWidth, maxLogoWidth)
	if p.BackgroundColor != nil {
		up := strings.ToUpper(*p.BackgroundColor)
		choice("public_view_background_color", &d.PublicView.BackgroundColor, &up, PublicViewBackgroundColors)
	}
	flag(&d.PublicView.LabelEnabled, p.LabelEnabled)

	flag(&d.Checkout.ShowAddress, k.ShowAddress)
	flag(&d.Checkout.ShowPhoneField, k.ShowPhoneField)
	flag(&d.Checkout.ForBusiness, k.ForBusiness)
	flag(&d.Checkout.ShowIdentity, k.ShowIdentity)
	flag(&d.Checkout.ShowWithdrawalButton, k.ShowWithdrawalButton)
	text("checkout_favorite_label", &d.Checkout.FavoriteLabel, k.FavoriteLabel, maxFavoriteLabel)

	if c.HideSenderAddress != nil {
		d.HideSenderAddress = *c.HideSenderAddress
	}
	if c.ShowEPCCode != nil {
		d.ShowEPCCode = *c.ShowEPCCode
	}
	number("sender_address_width", &d.SenderAddressWidth, c.SenderAddressWidth, minSenderWidth, maxSenderWidth)
}
