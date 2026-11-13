package recognize

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

var today = time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)

// textWant lists what a fixture must give; empty fields are not checked.
type textWant struct {
	kind                                         Kind
	name, vat, kvk, iban, email, website         string
	address, zip, city, country, phone           string
	ref, date, due, currency, incl, excl, payref string
	vat21                                        []string // "rate:taxable:tax"
	prices                                       string   // "incl" or "excl"
	lines                                        []string // "desc|qty|price|total|rate"
	good                                         bool
}

func TestParseTextFixtures(t *testing.T) {
	tests := map[string]textWant{
		"nl_utility.txt": {
			kind: KindPurchaseInvoice, name: "Voorbeeld Energie B.V.", vat: "NL123456782B01", kvk: "12345678",
			iban: "NL91ABNA0417164300", email: "klantenservice@voorbeeld-energie.example.com",
			website: "www.voorbeeld-energie.example.com", address: "Postbus 1234", zip: "1000 AB", city: "Amsterdam",
			country: "NL", phone: "020-1234567", ref: "2026110042", date: "2026-11-13", due: "2026-11-27",
			currency: "EUR", incl: "147.50", excl: "121.90", vat21: []string{"21:121.90:25.60"}, prices: "incl",
			lines:  []string{"Termijnbedrag stroom november|1|85.00|85.00|", "Termijnbedrag gas november|1|62.50|62.50|"},
			payref: "3000000000000047", good: true,
		},
		"nl_webshop.txt": {
			kind: KindPurchaseInvoice, name: "Example Shop B.V.", vat: "NL123456782B01", kvk: "12345678",
			email: "info@example.com", website: "www.example.com", address: "Voorbeeldstraat 1", zip: "1234 AB",
			city: "Amsterdam", country: "NL", ref: "INV-2026-0815", date: "2026-11-12", currency: "EUR",
			incl: "49.80", excl: "41.16", vat21: []string{"21:41.16:8.64"}, prices: "incl",
			lines: []string{"USB-C kabel 2m|2|9.95|19.90|", "Draadloze muis|1|24.95|24.95|", "Verzendkosten|1|4.95|4.95|"},
			good:  true,
		},
		"nl_restaurant.txt": {
			kind: KindReceipt, name: "EETCAFE DE VOORBEELD", vat: "NL123456782B01", address: "Marktplein 3",
			zip: "1234 AB", city: "Amsterdam", phone: "020-7654321", ref: "4711", date: "2026-11-13", currency: "EUR",
			incl: "21.65", excl: "19.86", vat21: []string{"9:19.86:1.79"}, prices: "incl",
			lines: []string{"Cappuccino|2|3.40|6.80|", "Tosti ham kaas|1|7.50|7.50|", "Appeltaart|1|4.25|4.25|", "Spa rood|1|3.10|3.10|"},
			good:  true,
		},
		"nl_kassabon.txt": {
			kind: KindReceipt, name: "SUPERMARKT VOORBEELD", address: "Winkelstraat 10", zip: "1234 AB",
			city: "Amsterdam", phone: "020-1112233", ref: "0042-3-4711", date: "2026-11-13", currency: "EUR",
			incl: "14.30", excl: "12.49", vat21: []string{"9:6.71:0.60", "21:5.78:1.21"}, prices: "incl",
			lines: []string{"HALFVOLLE MELK|1|1.19|1.19|", "VOLKORENBROOD|1|2.49|2.49|", "BANANEN|1|1.85|1.85|",
				"PINDAKAAS|2|0.89|1.78|", "WASMIDDEL|1|8.99|8.99|", "BONUS WASMIDDEL|1|-2.00|-2.00|"},
			good: true,
		},
		"nl_fuel.txt": {
			kind: KindReceipt, name: "Example Fuel B.V.", vat: "NL123456782B01", kvk: "12345678",
			address: "Snelweg 99", zip: "1234 AB", city: "Voorbeeldstad", ref: "2026-118-0042", date: "2026-11-13",
			currency: "EUR", incl: "89.29", excl: "73.79", vat21: []string{"21:73.79:15.50"}, prices: "incl",
			lines: []string{"Pomp 4 Euro 95 E10|1|84.30|84.30|", "Ruitenvloeistof|1|4.99|4.99|"},
			good:  true,
		},
		"nl_hardware.txt": {
			kind: KindReceipt, name: "Example Bouw B.V.", website: "www.example.com", address: "Industrieweg 20",
			zip: "1234 AB", city: "Amsterdam", ref: "7788-2026", date: "2026-11-13",
			incl: "28.44", excl: "23.50", vat21: []string{"21:23.50:4.94"}, prices: "incl",
			lines: []string{"Schroeven 4x40 200st|2|7.49|14.98|", "Houtlijm 250ml|1|5.99|5.99|", "Kwast 50mm|3|2.49|7.47|"},
			good:  true,
		},
		"en_saas.txt": {
			kind: KindPurchaseInvoice, name: "Example Cloud Ltd", vat: "GB123456789", email: "billing@example.com",
			address: "1 Example Street", zip: "EC1A 1BB", city: "London", country: "GB", ref: "EC-2026-11-0042",
			date: "2026-11-13", due: "2026-11-27", currency: "GBP", incl: "50.00", excl: "50.00",
			vat21: []string{"0:50.00:0.00"}, prices: "excl",
			lines: []string{"Pro plan (Nov 13 - Dec 13, 2026)|3|15.00|45.00|", "Extra storage 100 GB|1|5.00|5.00|"},
			good:  true,
		},
		"de_rechnung.txt": {
			kind: KindPurchaseInvoice, name: "Beispiel GmbH", vat: "DE123456789", iban: "DE89370400440532013000",
			email: "info@beispiel.example.com", address: "Musterstraße 12", zip: "10115", city: "Berlin",
			country: "DE", ref: "RE-2026-1187", date: "2026-11-13", due: "2026-11-27", currency: "EUR",
			incl: "506.35", excl: "425.50", vat21: []string{"19:425.50:80.85"}, prices: "excl",
			lines: []string{"Beratung (Stunden)|4|95.00|380.00|", "Reisekosten|1|45.50|45.50|"},
			good:  true,
		},
		"pt_fatura.txt": {
			kind: KindPurchaseInvoice, name: "Exemplo Lda", vat: "PT123456789", iban: "PT50000201231234567890154",
			address: "Rua do Exemplo, 123", zip: "1000-001", city: "Lisboa", country: "PT", ref: "FT 2026/142",
			date: "2026-11-13", due: "2026-12-13", currency: "EUR", incl: "369.00", excl: "300.00",
			vat21: []string{"23:300.00:69.00"}, prices: "excl",
			lines: []string{"Tradução de documentos|2|150.00|300.00|23"},
			good:  true,
		},
		"ocr_receipt.txt": {
			kind: KindReceipt, name: "CAFE VOORBEELD", vat: "NL123456782B01", zip: "1234 AB", city: "Amsterdam",
			ref: "0815", date: "2026-11-13", incl: "14.10", excl: "12.94", vat21: []string{"9:12.94:1.16"},
			prices: "incl", lines: []string{"Koffie verkeerd|2|3.80|7.60|", "Broodjekaas|1|6.50|6.50|"},
			good: true,
		},
		"nl_creditnota.txt": {
			kind: KindPurchaseInvoice, name: "Example Shop B.V.", vat: "NL123456782B01", kvk: "12345678",
			iban: "NL91ABNA0417164300", ref: "CN-2026-007", date: "2026-11-13", incl: "-24.95", excl: "-20.62",
			vat21: []string{"21:-20.62:-4.33"}, prices: "excl", lines: []string{"Retour draadloze muis|1|-20.62|-20.62|"},
			good: true,
		},
		"nl_verlegd.txt": {
			kind: KindPurchaseInvoice, name: "Example Bouw B.V.", vat: "NL123456782B01", kvk: "12345678",
			iban: "NL91ABNA0417164300", address: "Bouwweg 8", zip: "1234 AB", city: "Amsterdam",
			ref: "2026-0099", date: "2026-11-13", due: "2026-12-13", incl: "2450.00", excl: "2450.00",
			vat21: []string{"0:2450.00:0.00"}, prices: "excl", lines: []string{"Stukadoorswerk week 45|1|2450.00|2450.00|"},
			payref: "RF18539007547034", good: true,
		},
		"nl_two_rates.txt": {
			kind: KindPurchaseInvoice, name: "Example Catering B.V.", vat: "NL123456782B01", kvk: "12345678",
			iban: "NL91ABNA0417164300", email: "facturen@example.com", phone: "+31 20 123 4567",
			address: "Keukenstraat 4", ref: "F26-0313", date: "2026-11-13", due: "2026-11-27",
			incl: "603.88", excl: "537.50", vat21: []string{"9:387.50:34.88", "21:150.00:31.50"}, prices: "excl",
			lines: []string{"Lunchbuffet p.p.|25|12.50|312.50|9", "Drankjes p.p.|25|3.00|75.00|9", "Zaalhuur|1|150.00|150.00|21"},
			good:  true,
		},
		"en_usd.txt": {
			kind: KindPurchaseInvoice, name: "Example Software Inc.", email: "support@example.com", ref: "2026-1113",
			date: "2026-11-13", due: "2026-12-13", currency: "USD", incl: "1500.00", excl: "1500.00",
			vat21: []string{"0:1500.00:0.00"}, prices: "excl",
			lines: []string{"API usage - October 2026|1|1250.00|1250.00|", "Premium support|1|250.00|250.00|"},
			good:  true,
		},
		"nl_freelancer.txt": {
			kind: KindPurchaseInvoice, name: "Ann de Vries Fotografie", vat: "NL000099998B57", kvk: "12345678",
			iban: "NL91ABNA0417164300", address: "Fotostraat 7", zip: "1234 AB", city: "Amsterdam", ref: "2026-017",
			date: "2026-11-10", due: "2026-12-10", currency: "EUR", incl: "411.40", excl: "340.00",
			vat21: []string{"21:340.00:71.40"}, prices: "excl",
			lines:  []string{"Fotoshoot productfoto's (4 uur à € 85,00)|1|340.00|340.00|"},
			payref: "RF18539007547034", good: true,
		},
		"fr_facture.txt": {
			kind: KindPurchaseInvoice, name: "Exemple SARL", vat: "FR12345678901", address: "12 rue de l'Exemple",
			zip: "75001", city: "Paris", country: "FR", ref: "F-2026-0456", date: "2026-11-13", due: "2026-12-13",
			currency: "EUR", incl: "480.00", excl: "400.00", vat21: []string{"20:400.00:80.00"}, prices: "excl",
			lines: []string{"Conseil|2|200.00|400.00|"}, good: true,
		},
	}
	for file, w := range tests {
		t.Run(file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", file))
			if err != nil {
				t.Fatal(err)
			}
			got := ParseText(string(data), today)
			checkText(t, got, w)
		})
	}
}

func checkText(t *testing.T, got Extraction, w textWant) {
	t.Helper()
	eq := func(field, want, have string) {
		t.Helper()
		if want != "" && want != have {
			t.Errorf("%s = %q, want %q", field, have, want)
		}
	}
	eq("kind", string(w.kind), string(got.Kind.V))
	s := got.Supplier
	eq("name", w.name, s.Name)
	eq("vat", w.vat, s.VATNumber)
	eq("kvk", w.kvk, s.KVK)
	eq("iban", w.iban, s.IBAN)
	eq("email", w.email, s.Email)
	eq("website", w.website, s.Website)
	eq("address", w.address, s.Address1)
	eq("zip", w.zip, s.Zipcode)
	eq("city", w.city, s.City)
	eq("country", w.country, s.Country)
	eq("phone", w.phone, s.Phone)
	eq("reference", w.ref, got.Reference.V)
	eq("date", w.date, dateString(got.Date))
	eq("due", w.due, dateString(got.DueDate))
	eq("currency", w.currency, got.Currency.V)
	eq("incl", w.incl, amountString(got.TotalInclTax.Found, got.TotalInclTax.V))
	eq("excl", w.excl, amountString(got.TotalExclTax.Found, got.TotalExclTax.V))
	eq("payref", w.payref, got.PaymentReference.V)
	if w.vat21 != nil {
		var have []string
		for _, v := range got.VAT {
			have = append(have, fmt.Sprintf("%s:%s:%s", v.Rate, amountString(true, v.Taxable), amountString(true, v.Tax)))
		}
		if diff := cmp.Diff(w.vat21, have); diff != "" {
			t.Errorf("VAT (-want +got)\n%s", diff)
		}
	}
	if w.prices != "" {
		have := "excl"
		if got.PricesInclTax.V {
			have = "incl"
		}
		eq("prices", w.prices, have)
	}
	if w.lines != nil {
		var have []string
		for _, l := range got.Lines {
			have = append(have, strings.Join([]string{l.Description, l.Quantity.V,
				amountString(l.Price.Found, l.Price.V), amountString(l.Total.Found, l.Total.V), l.TaxRate.V}, "|"))
		}
		if diff := cmp.Diff(w.lines, have); diff != "" {
			t.Errorf("lines (-want +got)\n%s", diff)
		}
	}
	if got.Good() != w.good {
		t.Errorf("Good() = %v, want %v (total %+v, date %+v)", got.Good(), w.good, got.TotalInclTax, got.Date)
	}
}

func dateString(v Value[time.Time]) string {
	if !v.Found {
		return ""
	}
	return v.V.Format(time.DateOnly)
}

func amountString(found bool, a interface{ String() string }) string {
	if !found {
		return ""
	}
	s := a.String()
	whole, frac, _ := strings.Cut(s, ".")
	return whole + "." + (frac + "00")[:2]
}
