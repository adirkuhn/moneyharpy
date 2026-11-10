package banking

import (
	"context"
	"testing"
)

func hostingRule() RuleChanges {
	return RuleChanges{
		Name: ptr("Hosting"), Direction: ptr("debit"), LedgerAccountID: ptr(rentCategory), Description: ptr("Auto"),
		Conditions: &[]Condition{{Field: "contra_account_name", Operator: "contains", Value: "hosting"}, {Field: "amount", Operator: "less_than", Value: "100"}},
	}
}

func TestRulesBookNewTransactionsWhenOn(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	acc := bank(t, svc)
	if _, err := svc.CreateRule(ctx, adminID, owner, hostingRule()); err != nil {
		t.Fatal(err)
	}
	off := transactions(t, svc, acc, "-20")
	if len(off[0].Bookings) != 0 {
		t.Error("booked while automatic booking is off")
	}
	if _, err := svc.SetAutoLink(ctx, adminID, owner, true); err != nil {
		t.Fatal(err)
	}
	in := StatementInput{AccountID: &acc.ID, Reference: ptr("S2"), Mutations: []MutationInput{
		{Date: ptr("2026-11-02"), Amount: ptr("-20"), ContraName: ptr("Hosting Company")},
		{Date: ptr("2026-11-02"), Amount: ptr("-200"), ContraName: ptr("Hosting Company")},
		{Date: ptr("2026-11-02"), Amount: ptr("20"), ContraName: ptr("Hosting Company")},
	}}
	st, err := svc.CreateStatement(ctx, adminID, owner, in)
	if err != nil {
		t.Fatal(err)
	}
	booked := 0
	for _, m := range st.Mutations {
		if m.AutoBooked() {
			booked++
			if m.State != Processed || m.Bookings[0].Description != "Auto" {
				t.Errorf("auto booked = %+v", m)
			}
		}
	}
	if booked != 1 {
		t.Errorf("auto booked %d, want 1", booked)
	}
	f, _ := ParseMutationFilter("state:auto_booked", now)
	if got, _ := svc.Mutations(ctx, adminID, owner, f); len(got) != 1 {
		t.Errorf("auto_booked filter = %d", len(got))
	}
}

func TestRuleMatching(t *testing.T) {
	m := Mutation{AccountID: 5, Amount: -2550, Message: "Monthly hosting plan", ContraName: "Hosting Company", ContraAccount: "NL44RABO0123456789"}
	cases := []struct {
		rule Rule
		want bool
	}{
		{Rule{Active: true, MatchAll: true, Direction: "any", Conditions: []Condition{{"message", "starts_with", "monthly"}}}, true},
		{Rule{Active: true, MatchAll: true, Direction: "credit", Conditions: []Condition{{"message", "contains", "hosting"}}}, false},
		{Rule{Active: true, MatchAll: true, Direction: "any", Conditions: []Condition{{"contra_account_number", "equals", "nl44rabo0123456789"}, {"amount", "equals", "25.50"}}}, true},
		{Rule{Active: true, MatchAll: true, Direction: "any", Conditions: []Condition{{"message", "ends_with", "plan"}, {"amount", "greater_than", "30"}}}, false},
		{Rule{Active: true, MatchAll: false, Direction: "any", Conditions: []Condition{{"message", "ends_with", "plan"}, {"amount", "greater_than", "30"}}}, true},
		{Rule{Active: false, MatchAll: true, Direction: "any", Conditions: []Condition{{"message", "contains", "hosting"}}}, false},
		{Rule{Active: true, MatchAll: true, Direction: "any", AccountID: ptr(resourceID(6)), Conditions: []Condition{{"message", "contains", "hosting"}}}, false},
	}
	for i, c := range cases {
		if got := c.rule.Matches(m); got != c.want {
			t.Errorf("case %d: Matches() = %v, want %v", i, got, c.want)
		}
	}
}

func TestRuleValidationAndEditing(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	_, err := svc.CreateRule(ctx, adminID, owner, RuleChanges{
		Name: ptr(""), Direction: ptr("sideways"), LedgerAccountID: ptr(closedCategory), AccountID: ptr(resourceID(1)),
		Conditions: &[]Condition{{Field: "amount", Operator: "contains", Value: "x"}, {Field: "colour", Operator: "equals", Value: "red"}, {Field: "message", Operator: "contains", Value: " "}},
	})
	fields := fieldsOf(t, err)
	for _, f := range []string{"name", "direction", "ledger_account_id", "financial_account_id", "conditions.0", "conditions.1", "conditions.2"} {
		if fields[f] == nil {
			t.Errorf("fields = %v, want %s", fields, f)
		}
	}
	if _, err := svc.CreateRule(ctx, adminID, owner, RuleChanges{Name: ptr("x"), LedgerAccountID: ptr(rentCategory), Conditions: &[]Condition{}}); fieldsOf(t, err)["conditions"] == nil {
		t.Error("accepted a rule without conditions")
	}
	r, err := svc.CreateRule(ctx, adminID, owner, hostingRule())
	if err != nil {
		t.Fatal(err)
	}
	r2, _ := svc.CreateRule(ctx, adminID, owner, hostingRule())
	if r.Position != 1 || r2.Position != 2 {
		t.Errorf("positions %d, %d", r.Position, r2.Position)
	}
	if got, err := svc.UpdateRule(ctx, adminID, r.ID, owner, RuleChanges{Active: ptr(false), MatchAll: ptr(false)}); err != nil || got.Active || got.MatchAll {
		t.Errorf("update = %+v, %v", got, err)
	}
	if err := svc.DeleteRule(ctx, adminID, r.ID, owner); err != nil {
		t.Fatal(err)
	}
	if list, _ := svc.Rules(ctx, adminID, owner); len(list) != 1 {
		t.Errorf("rules = %d", len(list))
	}
}
