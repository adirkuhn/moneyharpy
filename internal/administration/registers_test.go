package administration

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
)

type stubRegister struct {
	matches []CompanyMatch
	err     error
	calls   int
}

func (s *stubRegister) SearchCompanies(context.Context, string) ([]CompanyMatch, error) {
	s.calls++
	return s.matches, s.err
}

func TestRegistersUseTheFirstEnabledOne(t *testing.T) {
	official := &stubRegister{err: ErrLookupUnavailable}
	website := &stubRegister{matches: []CompanyMatch{{Name: "Example Shop B.V.", ChamberOfCommerce: "12345678"}}}
	got, err := Registers{official, website}.SearchCompanies(context.Background(), "example")
	if err != nil {
		t.Fatalf("SearchCompanies() error = %v", err)
	}
	if diff := cmp.Diff(website.matches, got); diff != "" {
		t.Errorf("SearchCompanies() diff (-want +got):\n%s", diff)
	}

	official.err, official.matches = nil, []CompanyMatch{{Name: "Official"}}
	website.calls = 0
	if got, _ := (Registers{official, website}).SearchCompanies(context.Background(), "example"); len(got) != 1 || got[0].Name != "Official" {
		t.Errorf("SearchCompanies() = %v, want the official register's matches", got)
	}
	if website.calls != 0 {
		t.Error("the website search was called while the official register is enabled")
	}
}

func TestRegistersDoNotFallBackOnFailure(t *testing.T) {
	official := &stubRegister{err: errors.New("kvk answered 500")}
	website := &stubRegister{}
	if _, err := (Registers{official, website}).SearchCompanies(context.Background(), "example"); err == nil || errors.Is(err, ErrLookupUnavailable) {
		t.Errorf("SearchCompanies() error = %v, want the official failure", err)
	}
	if website.calls != 0 {
		t.Error("a failing official register fell back to the website search")
	}
}

func TestRegistersNoneEnabled(t *testing.T) {
	none := &stubRegister{err: ErrLookupUnavailable}
	if _, err := (Registers{none, none}).SearchCompanies(context.Background(), "x"); !errors.Is(err, ErrLookupUnavailable) {
		t.Errorf("SearchCompanies() error = %v, want ErrLookupUnavailable", err)
	}
	if _, err := (Registers{}).SearchCompanies(context.Background(), "x"); !errors.Is(err, ErrLookupUnavailable) {
		t.Errorf("SearchCompanies() with no registers error = %v, want ErrLookupUnavailable", err)
	}
}
