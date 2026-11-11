package reports

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/downloads"
	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Auditfile versions Moneybird offers.
var AuditfileVersions = []string{"3.2", "4.0"}

var yearPattern = regexp.MustCompile(`^\d{4}$`)

// book is what an export of a year needs: the administration, its
// categories, the year's entries and the balances at its start.
type book struct {
	admin    administration.Listed
	year     int
	period   money.Period
	accounts []Account
	index    accountIndex
	entries  []journal.Entry
	// opening are the categories' balances at the end of the year before.
	opening []Sum
	// results are the profit and loss results of the years before.
	results money.Amount
}

func (s *Service) book(ctx context.Context, admin, user resource.ID, year string) (book, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return book{}, err
	}
	if !yearPattern.MatchString(year) {
		return book{}, refuse("year does not have a valid value")
	}
	y, _ := strconv.Atoi(year)
	if y < 2000 || y > 2100 {
		return book{}, refuse("year does not have a valid value")
	}
	b := book{admin: a, year: y, period: money.Period{From: time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC), To: time.Date(y, 12, 31, 0, 0, 0, 0, time.UTC)}}
	if b.accounts, b.index, err = s.accounts(ctx, admin); err != nil {
		return book{}, err
	}
	return b, nil
}

// load reads the year's entries and opening balances; a year without
// entries is refused.
func (s *Service) load(ctx context.Context, b *book) error {
	var err error
	if b.entries, err = s.store.Entries(ctx, b.admin.ID, b.period); err != nil {
		return fmt.Errorf("list entries: %w", err)
	}
	if len(b.entries) == 0 {
		return &Refusal{Message: "Period contains no journal entries", Symbolic: map[string]string{"period": "contains_no_journal_entries"}}
	}
	sums, err := s.store.Movements(ctx, b.admin.ID, nil, dayBefore(b.period.From), nil)
	if err != nil {
		return fmt.Errorf("sum journal: %w", err)
	}
	for _, sum := range sums {
		if OnBalanceSheet(b.index[sum.LedgerAccountID].Type) {
			if sum.Amount != 0 {
				b.opening = append(b.opening, sum)
			}
		} else {
			b.results += sum.Amount
		}
	}
	b.index.byCode(b.opening)
	return nil
}

// export makes the file and keeps it as a download of the type; a failed
// export is kept as failed, so people see it did.
func (s *Service) export(ctx context.Context, admin, user resource.ID, downloadType, filename, contentType string, build func() ([]byte, error)) error {
	who := &user
	data, err := build()
	if err != nil {
		if _, ferr := s.downloads.AddFailed(ctx, admin, who, downloadType, filename); ferr != nil {
			return fmt.Errorf("record failed export: %w", ferr)
		}
		return fmt.Errorf("make %s: %w", downloadType, err)
	}
	if _, err := s.downloads.Add(ctx, admin, who, downloadType, filename, contentType, data); err != nil {
		return fmt.Errorf("store %s: %w", downloadType, err)
	}
	return nil
}

// ExportAuditfile makes the year's auditfile (XAF) and adds it to the
// downloads. Every category needs an account id of its own, and the year
// needs entries. MoneyHarpy writes the 3.2 format for both versions.
func (s *Service) ExportAuditfile(ctx context.Context, admin, user resource.ID, year, version string) error {
	if version == "" {
		version = "3.2"
	}
	if !slices.Contains(AuditfileVersions, version) {
		return refuse("version does not have a valid value")
	}
	b, err := s.book(ctx, admin, user, year)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, a := range b.accounts {
		if a.Code == "" || seen[a.Code] {
			return &Refusal{Message: "Missing or duplicate account IDs", Symbolic: map[string]string{"base": "missing_or_duplicate_account_ids"}}
		}
		seen[a.Code] = true
	}
	if err := s.load(ctx, &b); err != nil {
		return err
	}
	contacts, err := s.contactsOf(ctx, b)
	if err != nil {
		return err
	}
	return s.export(ctx, admin, user, downloads.Auditfile, fmt.Sprintf("auditfile_%d.xaf", b.year), "text/xml", func() ([]byte, error) {
		return auditfile(b, contacts, s.now())
	})
}

func (s *Service) contactsOf(ctx context.Context, b book) ([]Contact, error) {
	ids := []resource.ID{}
	for _, e := range b.entries {
		if e.ContactID != nil && !slices.Contains(ids, *e.ContactID) {
			ids = append(ids, *e.ContactID)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	list, err := s.store.Contacts(ctx, b.admin.ID, ids)
	if err != nil {
		return nil, fmt.Errorf("list contacts: %w", err)
	}
	return list, nil
}

// ExportBrugstaat makes the year's RGS brugstaat: every category with
// bookings, its RGS code and its figures; and adds it to the downloads.
// Every category needs a code of the administration's RGS scheme.
func (s *Service) ExportBrugstaat(ctx context.Context, admin, user resource.ID, year string) error {
	b, err := s.book(ctx, admin, user, year)
	if err != nil {
		return err
	}
	scheme, err := s.schemes.Scheme(ctx, admin, user)
	if err != nil {
		return fmt.Errorf("chart scheme: %w", err)
	}
	unlinked := refuse("Not all categories are linked to a RGS code. This is mandatory before downloading a RGS Brugstaat")
	if scheme == nil {
		return unlinked
	}
	for _, a := range b.accounts {
		if _, ok := scheme.Item(a.Taxonomy); !ok && a.Active {
			return unlinked
		}
	}
	if err := s.load(ctx, &b); err != nil {
		return err
	}
	return s.export(ctx, admin, user, downloads.Brugstaat, fmt.Sprintf("brugstaat_%d.xml", b.year), "text/xml", func() ([]byte, error) {
		return brugstaat(b, scheme, s.now())
	})
}

// ExportLedgerAccounts makes the year's ledger cards (grootboekkaarten) as
// an Excel workbook: per category its balance at the start, every booking
// and its balance at the end; and adds it to the downloads.
func (s *Service) ExportLedgerAccounts(ctx context.Context, admin, user resource.ID, year string) error {
	b, err := s.book(ctx, admin, user, year)
	if err != nil {
		return err
	}
	if err := s.load(ctx, &b); err != nil {
		return err
	}
	contacts, err := s.contactsOf(ctx, b)
	if err != nil {
		return err
	}
	const xlsx = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	return s.export(ctx, admin, user, downloads.ExportLedgerAccountReport, fmt.Sprintf("grootboekkaarten_%d.xlsx", b.year), xlsx, func() ([]byte, error) {
		return ledgerCards(b, contacts)
	})
}
