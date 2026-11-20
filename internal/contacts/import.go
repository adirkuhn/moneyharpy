package contacts

import (
	"context"
	"fmt"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// ImportedPerson is a contact person brought in with a contact from another
// system, with the id it had there.
type ImportedPerson struct {
	Origin  resource.Origin
	Changes PersonChanges
}

// Import adds a contact brought in from another system, such as Moneybird,
// keeping the id and times it had there. It checks the fields as Create
// does; it records no event, as the contact was made elsewhere.
func (s *Service) Import(ctx context.Context, admin, user resource.ID, o resource.Origin, c Changes, people []ImportedPerson) (Contact, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Contact{}, err
	}
	created, updated := o.Or(s.now())
	contact := Contact{
		ID: o.ID, AdministrationID: admin, Country: a.Country, DeliveryMethod: "Email", SepaSequenceType: "RCUR",
		CreatedAt: created, UpdatedAt: updated,
	}
	if err := s.apply(ctx, admin, user, &contact, c); err != nil {
		return Contact{}, err
	}
	if contact.CustomerID == "" {
		if contact.CustomerID, err = s.nextCustomerID(ctx, admin); err != nil {
			return Contact{}, err
		}
	}
	for _, ip := range people {
		pc, pu := ip.Origin.Or(created)
		p := Person{ID: ip.Origin.ID, AdministrationID: admin, ContactID: contact.ID, CreatedAt: pc, UpdatedAt: pu}
		if err := applyPerson(&p, ip.Changes, "contact_person."); err != nil {
			return Contact{}, err
		}
		contact.People = append(contact.People, p)
	}
	if err := s.store.CreateContact(ctx, contact); err != nil {
		return Contact{}, fmt.Errorf("store contact: %w", err)
	}
	return contact, nil
}
