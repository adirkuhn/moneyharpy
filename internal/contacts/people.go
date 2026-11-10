package contacts

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// PersonChanges holds the fields of a contact person to set; nil fields
// stay as they are.
type PersonChanges struct {
	Firstname, Lastname, Phone, Email, Department *string
}

// Person returns one person of a contact.
func (s *Service) Person(ctx context.Context, admin, contact, id, user resource.ID) (Person, error) {
	c, err := s.Contact(ctx, admin, contact, user)
	if err != nil {
		return Person{}, err
	}
	return findPerson(c, id)
}

func findPerson(c Contact, id resource.ID) (Person, error) {
	i := slices.IndexFunc(c.People, func(p Person) bool { return p.ID == id })
	if i < 0 {
		return Person{}, ErrPersonNotFound
	}
	return c.People[i], nil
}

// AddPerson adds a contact person.
func (s *Service) AddPerson(ctx context.Context, admin, contact, user resource.ID, ch PersonChanges) (Person, error) {
	if _, err := s.Contact(ctx, admin, contact, user); err != nil {
		return Person{}, err
	}
	now := s.now()
	p := Person{ID: s.ids.Next(), AdministrationID: admin, ContactID: contact, CreatedAt: now, UpdatedAt: now}
	if err := applyPerson(&p, ch, ""); err != nil {
		return Person{}, err
	}
	if err := s.store.SavePerson(ctx, p); err != nil {
		return Person{}, fmt.Errorf("store contact person: %w", err)
	}
	return p, nil
}

// UpdatePerson changes a contact person.
func (s *Service) UpdatePerson(ctx context.Context, admin, contact, id, user resource.ID, ch PersonChanges) (Person, error) {
	p, err := s.Person(ctx, admin, contact, id, user)
	if err != nil {
		return Person{}, err
	}
	if err := applyPerson(&p, ch, ""); err != nil {
		return Person{}, err
	}
	p.UpdatedAt = s.now()
	if err := s.store.SavePerson(ctx, p); err != nil {
		return Person{}, fmt.Errorf("store contact person: %w", err)
	}
	return p, nil
}

// DeletePerson removes a contact person.
func (s *Service) DeletePerson(ctx context.Context, admin, contact, id, user resource.ID) error {
	if _, err := s.Person(ctx, admin, contact, id, user); err != nil {
		return err
	}
	return s.store.DeletePerson(ctx, admin, contact, id, s.now())
}

// applyPerson sets the changes; prefix names the fields in errors, for a
// person nested in a contact.
func applyPerson(p *Person, ch PersonChanges, prefix string) error {
	for dst, src := range map[*string]*string{&p.Firstname: ch.Firstname, &p.Lastname: ch.Lastname, &p.Phone: ch.Phone, &p.Email: ch.Email, &p.Department: ch.Department} {
		if src != nil {
			*dst = strings.TrimSpace(*src)
		}
	}
	var verr resource.ValidationError
	if p.Firstname == "" && p.Lastname == "" {
		verr.Add(prefix+"base", "First name or last name can't be blank")
	}
	for field, v := range map[string]string{"firstname": p.Firstname, "lastname": p.Lastname, "phone": p.Phone, "email": p.Email, "department": p.Department} {
		if len(v) > maxText {
			verr.Add(prefix+field, fmt.Sprintf("is too long (maximum is %d characters)", maxText))
		}
	}
	if p.Email != "" && (strings.Contains(p.Email, ",") || !validEmails(p.Email)) {
		verr.Add(prefix+"email", "is not a valid email address")
	}
	return verr.OrNil()
}
