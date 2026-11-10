package taxonomy_test

import (
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/taxonomy"
)

func TestDutchSchemeIsBuiltIn(t *testing.T) {
	s, ok := taxonomy.Lookup("nl-rgs-3.8")
	if !ok {
		t.Fatal("Lookup(nl-rgs-3.8) found nothing")
	}
	if s.Country != "NL" || s.Version != "3.8" || s.Len() < 3000 {
		t.Errorf("scheme = %s %s with %d codes", s.Country, s.Version, s.Len())
	}
	it, ok := s.Item("WBedAlkOal")
	if !ok {
		t.Fatal("Item(WBedAlkOal) found nothing")
	}
	if it.Reference != "4215010" || it.Name != "Algemene kosten" || it.Side() != taxonomy.ProfitAndLoss || it.Parent != "WBedAlk" {
		t.Errorf("item = %+v", it)
	}
	if kas, _ := s.Item("BLimKas"); kas.Side() != taxonomy.BalanceSheet {
		t.Errorf("BLimKas side = %v", kas.Side())
	}
	if _, ok := s.Item("Nope"); ok {
		t.Error("Item(Nope) found something")
	}
}

func TestSearch(t *testing.T) {
	s, _ := taxonomy.Lookup("nl-rgs-3.8")
	got := s.Search("kasmiddelen", taxonomy.BalanceSheet, 5)
	if len(got) == 0 || got[0].Code != "BLimKas" {
		t.Fatalf("Search(kasmiddelen) = %+v", got)
	}
	for _, it := range s.Search("kosten", taxonomy.ProfitAndLoss, 20) {
		if it.Side() != taxonomy.ProfitAndLoss {
			t.Errorf("Search on profit and loss returned %s", it.Code)
		}
	}
	if got := s.Search("4215010", taxonomy.AnySide, 3); len(got) != 1 || got[0].Code != "WBedAlkOal" {
		t.Errorf("Search(reference) = %+v", got)
	}
	if got := s.Search("kosten", taxonomy.AnySide, 3); len(got) != 3 {
		t.Errorf("Search() ignored its limit: %d", len(got))
	}
}

func TestSchemesForCountry(t *testing.T) {
	if got := taxonomy.ForCountry("NL"); len(got) != 1 || got[0].ID != "nl-rgs-3.8" {
		t.Errorf("ForCountry(NL) = %v", got)
	}
	if got := taxonomy.ForCountry("XX"); len(got) != 0 {
		t.Errorf("ForCountry(XX) = %v", got)
	}
	if len(taxonomy.All()) == 0 {
		t.Error("All() is empty")
	}
}
