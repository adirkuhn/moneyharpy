package contacts

import (
	"context"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

func TestImportKeepsTheIDTimesAndPeople(t *testing.T) {
	s, store := newService()
	created := time.Date(2024, 3, 1, 10, 0, 0, 0, time.UTC)
	origin := resource.Origin{ID: 504315055158854794, CreatedAt: created, UpdatedAt: created.Add(time.Hour)}
	people := []ImportedPerson{{Origin: resource.Origin{ID: 504315055158854795}, Changes: PersonChanges{Firstname: ptr("Ann"), Lastname: ptr("de Vries")}}}

	c, err := s.Import(context.Background(), adminID, owner, origin, Changes{CompanyName: ptr("Example Shop B.V."), CustomerID: ptr("42")}, people)
	if err != nil {
		t.Fatal(err)
	}
	got := store.contacts[origin.ID]
	if c.ID != origin.ID || got.CustomerID != "42" || !got.CreatedAt.Equal(created) || !got.UpdatedAt.Equal(origin.UpdatedAt) {
		t.Errorf("stored %+v", got)
	}
	if len(got.People) != 1 || got.People[0].ID != 504315055158854795 || got.People[0].ContactID != origin.ID {
		t.Errorf("people %+v", got.People)
	}
}

func TestImportChecksLikeCreate(t *testing.T) {
	s, _ := newService()
	_, err := s.Import(context.Background(), adminID, owner, resource.Origin{ID: 7}, Changes{SendInvoicesToEmail: ptr("not an address")}, nil)
	if err == nil {
		t.Fatal("an invalid contact was imported")
	}
}
