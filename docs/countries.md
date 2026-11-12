# Adding a country

A new administration starts its books from its country's **seed**: the VAT
rates, the boxes of the VAT return they count in, a standard set of
categories (ledger accounts) and templates that add more, such as a company
car. Categories can map to a national **chart scheme**, such as the Dutch
Referentie Grootboekschema (RGS).

Countries without a seed get the generic one: the categories the books need,
without VAT rates or a chart scheme. People then add their own rates.

The Netherlands is the only country with a seed so far. This page describes
how to add another one, say Belgium (`BE`).

## 1. The chart scheme (optional)

Skip this step if the country has no standard chart of accounts.

1. Check the scheme's licence allows redistribution, and note the source.
2. Convert it to CSV in `internal/taxonomy/data/<country>-<scheme>-<version>.csv`
   with these columns:

   | Column | Meaning |
   |---|---|
   | `code` | Unique code, such as `WBedKanSof` |
   | `reference` | The scheme's own number, if it has one |
   | `name` | Name in the country's language |
   | `description` | Longer explanation, may be empty |
   | `debit_credit` | `D` or `C` |
   | `level` | Depth, from 1 |
   | `parent` | Code of the parent, empty at level 1 |
   | `basic` | `true` for the codes small businesses usually need |

   Codes on the balance sheet must start with `B`, profit and loss codes with
   `W`; `taxonomy.Item.Side` relies on that. A scheme with other conventions
   needs its own rule in `Side`.

   For RGS, `cmd/rgsimport` reads the official spreadsheet. Write a similar
   importer for another source, with tests, rather than editing the CSV by
   hand.
3. Register it in `schemes` in `internal/taxonomy/taxonomy.go`:

   ```go
   {ID: "be-pcmn-2024", Country: "BE", Name: "Minimum Normalized Chart of Accounts (PCMN)", Version: "2024", Source: "https://…", file: "data/be-pcmn-2024.csv"},
   ```
4. Add the source and licence to `internal/taxonomy/data/README.md`.

## 2. The seed

Create `internal/bookkeeping/countries/<country>.go`, after `nl.go`:

```go
var belgium = Seed{
	Country:   "BE",
	Scheme:    "be-pcmn-2024", // or "" without a scheme
	Boxes:     []Box{{Code: "BE/00", Name: Text{EN: "…", NL: "…"}}},
	TaxRates:  []TaxRate{{Name: Text{EN: "VAT 21%", NL: "Btw 21%"}, Percentage: "21", Type: "sales_invoice", ShowTax: true, Boxes: []string{"BE/03"}}},
	Accounts:  []Account{…},
	Templates: []Template{…},
}
```

and add it to `seeds` in `countries.go`:

```go
var seeds = map[string]Seed{"NL": netherlands, "BE": belgium}
```

What goes in:

- **Boxes**: every box of the VAT return, coded `<country>/<box>`.
- **TaxRates**: the rates for sales, purchases (`purchase_invoice`) and
  journal entries (`general_journal_document`), each with the boxes it counts
  in. Set `ShowTax: false` for reverse-charged and exempt rates.
- **Accounts**: at least every account with a `System` role in `nl.go`, with
  the same role names. The books rely on them, for example
  `accounts_receivable` and `sales_tax`. Children name their parent by `Key`
  and have the same `Type`.
- **Templates**: the themes of Add category, such as a company car (see
  `nl_themes.go`). Use the same keys as the Dutch themes where the situation
  is the same, so people recognise them across countries. A theme either
  lists categories to tick or adds one category the user names (`Single`,
  with the kinds to choose from and their chart scheme codes). In a list,
  an account with only a `Key` refers to a category of the seed or of
  another theme; `Selected` ticks it when the page opens and `Choices` shows
  it only for some answers of the theme's `Choice` (such as how the car is
  financed). A theme with a `NameLabel` asks for a name that replaces
  `{name}` in its categories' names, such as the entrepreneur's; those
  categories are added once per name and have no fixed code.
- **Text**: names in English and Dutch. A language MoneyHarpy does not speak
  yet falls back to English.

## 3. Tests

`go test ./internal/bookkeeping/countries/` checks every registered seed:
account types, parents of the same type, unique keys, names unique per type
in each language, every system role present, chart scheme codes that exist
and sit on the right side of the books, tax rate boxes that exist, and
themes whose references resolve, whose parents are on the same page and
whose choices and options exist. New
seeds are picked up from `seeds`, so there is no list of countries to update
in the tests. Then run `go test ./...`.

## 4. Existing administrations

Seeds apply once, the first time an administration opens its bookkeeping.
Changing a seed does not change administrations that were seeded already;
add a migration for that if it is needed.
