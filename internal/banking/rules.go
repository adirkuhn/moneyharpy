package banking

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Fields and operators of rule conditions.
var (
	RuleFields        = []string{"message", "contra_account_name", "contra_account_number", "amount"}
	TextOperators     = []string{"contains", "equals", "starts_with", "ends_with"}
	AmountOperators   = []string{"equals", "greater_than", "less_than"}
	RuleDirections    = []string{"any", "debit", "credit"}
	maxRuleConditions = 10
)

// Rule books matching transactions automatically.
type Rule struct {
	ID               resource.ID
	AdministrationID resource.ID
	Name             string
	Active           bool
	Position         int
	// MatchAll needs every condition to match; otherwise one is enough.
	MatchAll   bool
	Direction  string
	AccountID  *resource.ID
	Conditions []Condition
	// LedgerAccountID is the category matching transactions are booked on.
	LedgerAccountID resource.ID
	Description     string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Ref locates the rule.
func (r Rule) Ref() resource.Ref {
	return resource.Ref{ID: r.ID, Type: resource.TypeBookingRule, AdministrationID: r.AdministrationID}
}

// Condition compares one field of a transaction. Text compares ignore case;
// amounts compare without sign.
type Condition struct{ Field, Operator, Value string }

func (c Condition) match(m Mutation) bool {
	if c.Field == "amount" {
		want, err := ParseAmount(c.Value)
		if err != nil {
			return false
		}
		have := m.Amount
		if have < 0 {
			have = -have
		}
		if want < 0 {
			want = -want
		}
		switch c.Operator {
		case "equals":
			return have == want
		case "greater_than":
			return have > want
		case "less_than":
			return have < want
		}
		return false
	}
	var have string
	switch c.Field {
	case "message":
		have = m.Message
	case "contra_account_name":
		have = m.ContraName
	case "contra_account_number":
		have = m.ContraAccount
	}
	have, want := strings.ToLower(have), strings.ToLower(strings.TrimSpace(c.Value))
	switch c.Operator {
	case "contains":
		return strings.Contains(have, want)
	case "equals":
		return have == want
	case "starts_with":
		return strings.HasPrefix(have, want)
	case "ends_with":
		return strings.HasSuffix(have, want)
	}
	return false
}

// Matches reports whether the rule applies to the transaction.
func (r Rule) Matches(m Mutation) bool {
	if !r.Active || (r.AccountID != nil && *r.AccountID != m.AccountID) ||
		(r.Direction == "debit" && m.Amount > 0) || (r.Direction == "credit" && m.Amount < 0) {
		return false
	}
	for _, c := range r.Conditions {
		hit := c.match(m)
		if r.MatchAll && !hit {
			return false
		}
		if !r.MatchAll && hit {
			return true
		}
	}
	return r.MatchAll && len(r.Conditions) > 0
}

// RuleChanges holds the fields to set; nil fields stay as they are. An
// AccountID of 0 applies the rule to every account.
type RuleChanges struct {
	Name            *string
	Active          *bool
	MatchAll        *bool
	Direction       *string
	AccountID       *resource.ID
	Conditions      *[]Condition
	LedgerAccountID *resource.ID
	Description     *string
}

// Rules lists the booking rules in the order they are tried.
func (s *Service) Rules(ctx context.Context, admin, user resource.ID) ([]Rule, error) {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return nil, err
	}
	return s.rules(ctx, admin)
}

func (s *Service) rules(ctx context.Context, admin resource.ID) ([]Rule, error) {
	list, err := s.store.Rules(ctx, admin)
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	slices.SortStableFunc(list, func(a, b Rule) int { return cmp.Or(cmp.Compare(a.Position, b.Position), cmp.Compare(a.ID, b.ID)) })
	return list, nil
}

// Rule returns one booking rule.
func (s *Service) Rule(ctx context.Context, admin, id, user resource.ID) (Rule, error) {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return Rule{}, err
	}
	return s.store.Rule(ctx, admin, id)
}

// CreateRule adds a booking rule, tried last.
func (s *Service) CreateRule(ctx context.Context, admin, user resource.ID, c RuleChanges) (Rule, error) {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return Rule{}, err
	}
	now := s.now()
	r := Rule{ID: s.ids.Next(), AdministrationID: admin, Active: true, MatchAll: true, Direction: "any", CreatedAt: now, UpdatedAt: now}
	if err := s.applyRule(ctx, admin, user, &r, c); err != nil {
		return Rule{}, err
	}
	existing, err := s.rules(ctx, admin)
	if err != nil {
		return Rule{}, err
	}
	for _, e := range existing {
		r.Position = max(r.Position, e.Position)
	}
	r.Position++
	if err := s.store.CreateRule(ctx, r); err != nil {
		return Rule{}, fmt.Errorf("store rule: %w", err)
	}
	return r, nil
}

// UpdateRule changes a booking rule.
func (s *Service) UpdateRule(ctx context.Context, admin, id, user resource.ID, c RuleChanges) (Rule, error) {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return Rule{}, err
	}
	r, err := s.store.Rule(ctx, admin, id)
	if err != nil {
		return Rule{}, err
	}
	if err := s.applyRule(ctx, admin, user, &r, c); err != nil {
		return Rule{}, err
	}
	r.UpdatedAt = s.now()
	if err := s.store.UpdateRules(ctx, []Rule{r}); err != nil {
		return Rule{}, fmt.Errorf("store rule: %w", err)
	}
	return r, nil
}

// DeleteRule removes a booking rule. Bookings it made stay.
func (s *Service) DeleteRule(ctx context.Context, admin, id, user resource.ID) error {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return err
	}
	if _, err := s.store.Rule(ctx, admin, id); err != nil {
		return err
	}
	return s.store.DeleteRule(ctx, admin, id)
}

func (s *Service) applyRule(ctx context.Context, admin, user resource.ID, r *Rule, c RuleChanges) error {
	if c.Name != nil {
		r.Name = strings.TrimSpace(*c.Name)
	}
	if c.Active != nil {
		r.Active = *c.Active
	}
	if c.MatchAll != nil {
		r.MatchAll = *c.MatchAll
	}
	if c.Direction != nil {
		r.Direction = *c.Direction
	}
	if c.AccountID != nil {
		r.AccountID = nil
		if *c.AccountID != 0 {
			id := *c.AccountID
			r.AccountID = &id
		}
	}
	if c.Conditions != nil {
		r.Conditions = slices.Clone(*c.Conditions)
	}
	if c.LedgerAccountID != nil {
		r.LedgerAccountID = *c.LedgerAccountID
	}
	if c.Description != nil {
		r.Description = strings.TrimSpace(*c.Description)
	}
	var verr resource.ValidationError
	if r.Name == "" || len(r.Name) > 255 {
		verr.Add("name", "can't be blank")
	}
	if !slices.Contains(RuleDirections, r.Direction) {
		verr.Add("direction", "is not included in the list")
	}
	if len(r.Description) > 1000 {
		verr.Add("description", "is too long (maximum is 1000 characters)")
	}
	if len(r.Conditions) == 0 || len(r.Conditions) > maxRuleConditions {
		verr.Add("conditions", fmt.Sprintf("must hold 1 to %d conditions", maxRuleConditions))
	}
	for i, cond := range r.Conditions {
		field := fmt.Sprintf("conditions.%d", i)
		r.Conditions[i].Value = strings.TrimSpace(cond.Value)
		operators := TextOperators
		if cond.Field == "amount" {
			operators = AmountOperators
			if _, err := ParseAmount(cond.Value); err != nil {
				verr.Add(field, "value is not an amount")
			}
		}
		switch {
		case !slices.Contains(RuleFields, cond.Field):
			verr.Add(field, "field is not included in the list")
		case !slices.Contains(operators, cond.Operator):
			verr.Add(field, "operator does not fit the field")
		case r.Conditions[i].Value == "" || len(cond.Value) > 255:
			verr.Add(field, "value can't be blank")
		}
	}
	if r.AccountID != nil {
		if _, err := s.store.Account(ctx, admin, *r.AccountID); err != nil {
			verr.Add("financial_account_id", "does not exist")
		}
	}
	if r.LedgerAccountID == 0 {
		verr.Add("ledger_account_id", "can't be blank")
	} else if err := s.ledger.CheckBookable(ctx, admin, user, r.LedgerAccountID); err != nil {
		verr.Add("ledger_account_id", "can't be booked on")
	}
	return verr.OrNil()
}

// autoBook books the transactions the first matching rule fits, when
// automatic booking is on. It returns the transactions as they are now.
func (s *Service) autoBook(ctx context.Context, admin, user resource.ID, mutations []Mutation) ([]Mutation, error) {
	if len(mutations) == 0 {
		return mutations, nil
	}
	st, err := s.store.Settings(ctx, admin)
	if err != nil || !st.AutoLink {
		return mutations, err
	}
	rules, err := s.rules(ctx, admin)
	if err != nil {
		return nil, err
	}
	locked, err := s.locked(ctx, admin, user)
	if err != nil {
		return nil, err
	}
	bookable := map[resource.ID]bool{}
	for i, m := range mutations {
		if len(m.Bookings) > 0 || (locked != nil && !m.Date.After(*locked)) {
			continue
		}
		for _, r := range rules {
			if !r.Matches(m) {
				continue
			}
			ok, seen := bookable[r.LedgerAccountID]
			if !seen {
				ok = s.ledger.CheckBookable(ctx, admin, user, r.LedgerAccountID) == nil
				bookable[r.LedgerAccountID] = ok
			}
			if !ok {
				continue
			}
			now := s.now()
			rule := r.ID
			b := Booking{ID: s.ids.Next(), AdministrationID: admin, MutationID: m.ID, LedgerAccountID: r.LedgerAccountID, Description: r.Description, Price: m.Open() * m.direction(), RuleID: &rule, CreatedAt: now, UpdatedAt: now}
			m.Bookings = append(m.Bookings, b)
			m.settle(now)
			if err := s.store.SaveBookings(ctx, m, []Booking{b}, nil); err != nil {
				return nil, fmt.Errorf("store booking: %w", err)
			}
			mutations[i] = m
			break
		}
	}
	return mutations, nil
}
