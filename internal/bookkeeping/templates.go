package bookkeeping

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/bookkeeping/countries"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Template is a theme of Moneybird's Add category, named in the
// administration's language: categories to pick from, or one category the
// user names.
type Template struct {
	Key, Name, Description string
	// Intro and Warning head the theme's page; Warning may be empty.
	Intro, Warning string
	// Choice decides which accounts show, such as how the car is financed.
	Choice *TemplateChoice
	// NameLabel asks for the name the accounts' names are completed with.
	NameLabel string
	// Single is set when the theme adds one category the user names.
	Single   *TemplateSingle
	Accounts []TemplateAccount
	// Applied reports that the administration has all of its categories.
	Applied bool
}

// TemplateChoice is a question whose answer decides which accounts show.
type TemplateChoice struct {
	Label, Prompt string
	Options       []TemplateOption
}

// TemplateSingle is the one category a template adds.
type TemplateSingle struct {
	Type, NameLabel, OptionsLabel, OptionsHelp string
	// Parent offers a parent category of the same type.
	Parent  bool
	Options []TemplateOption
}

// TemplateOption is an answer of a choice or a kind of single category, with
// its chart scheme code.
type TemplateOption struct{ Key, Name, Taxonomy string }

// TemplateAccount is a category a template can add.
type TemplateAccount struct {
	// Key names the category; Parent the key of the category it goes under.
	Key, Parent                             string
	Name, Type, Code, Taxonomy, Description string
	// Exists reports that the administration has the category already.
	Exists bool
	// Selected ticks the category when the page opens.
	Selected bool
	// Choices are the choice's options the category shows for; empty for all.
	Choices []string
}

// TemplateInput is what the user filled in on a template's page.
type TemplateInput struct {
	// Accounts are the keys of the categories to add; their parents in the
	// template are added too. Nil adds all of them.
	Accounts []string
	// Name completes the accounts' names, or names the single category.
	Name string
	// Option is the kind of single category.
	Option string
	// ParentID is the single category's parent, if any.
	ParentID *resource.ID
}

// Templates lists the templates of the administration's country. Without
// the country's chart scheme, the codes are left off.
func (s *Service) Templates(ctx context.Context, admin, user resource.ID) ([]Template, error) {
	a, err := s.ensure(ctx, admin, user)
	if err != nil {
		return nil, err
	}
	all, err := s.store.LedgerAccounts(ctx, admin)
	if err != nil {
		return nil, fmt.Errorf("list ledger accounts: %w", err)
	}
	st, err := s.store.Settings(ctx, admin)
	if err != nil {
		return nil, fmt.Errorf("load settings: %w", err)
	}
	keys := keysOf(all)
	seed := countries.For(a.Country)
	lang := a.Language
	code := func(c string) string {
		if st.Scheme != seed.Scheme {
			return ""
		}
		return c
	}
	options := func(in []countries.Option) []TemplateOption {
		out := make([]TemplateOption, len(in))
		for i, o := range in {
			out[i] = TemplateOption{Key: o.Key, Name: o.Name.In(lang), Taxonomy: code(o.Taxonomy)}
		}
		return out
	}
	out := make([]Template, len(seed.Templates))
	for i, t := range seed.Templates {
		named := t.NameLabel.EN != ""
		tpl := Template{
			Key: t.Key, Name: t.Name.In(lang), Description: t.Description.In(lang), Intro: t.Intro.In(lang), Warning: t.Warning.In(lang),
			NameLabel: t.NameLabel.In(lang), Applied: t.Single == nil && !named,
		}
		if t.Choice != nil {
			tpl.Choice = &TemplateChoice{Label: t.Choice.Label.In(lang), Prompt: t.Choice.Prompt.In(lang), Options: options(t.Choice.Options)}
		}
		if t.Single != nil {
			tpl.Single = &TemplateSingle{
				Type: t.Single.Type, NameLabel: t.Single.NameLabel.In(lang), OptionsLabel: t.Single.OptionsLabel.In(lang),
				OptionsHelp: t.Single.OptionsHelp.In(lang), Parent: t.Single.Parent, Options: options(t.Single.Options),
			}
		}
		for _, sa := range seed.TemplateAccounts(t) {
			// Accounts completed with a name are added once per name, so none exists before the name is known.
			exists := !named && keys[sa.Key]
			tpl.Applied = tpl.Applied && exists
			tpl.Accounts = append(tpl.Accounts, TemplateAccount{
				Key: sa.Key, Parent: sa.Parent, Name: sa.Name.In(lang), Type: sa.Type, Code: sa.Code, Taxonomy: code(sa.Taxonomy),
				Description: sa.Description.In(lang), Exists: exists, Selected: sa.Selected, Choices: slices.Clone(sa.Choices),
			})
		}
		out[i] = tpl
	}
	return out, nil
}

// ApplyTemplate adds what the user picked on a template's page and returns
// the categories it added: the ticked categories the administration does not
// have yet, with their parents, or the one category a single template adds.
// Without the country's chart scheme, the codes are left off.
func (s *Service) ApplyTemplate(ctx context.Context, admin, user resource.ID, key string, in TemplateInput) ([]LedgerAccount, error) {
	a, err := s.ensure(ctx, admin, user)
	if err != nil {
		return nil, err
	}
	seed := countries.For(a.Country)
	i := slices.IndexFunc(seed.Templates, func(t countries.Template) bool { return t.Key == key })
	if i < 0 {
		return nil, ErrTemplateNotFound
	}
	tpl := seed.Templates[i]
	st, err := s.store.Settings(ctx, admin)
	if err != nil {
		return nil, fmt.Errorf("load settings: %w", err)
	}
	ownScheme := st.Scheme == seed.Scheme
	if tpl.Single != nil {
		return s.applySingle(ctx, a.Administration, tpl, in, ownScheme)
	}
	all, err := s.store.LedgerAccounts(ctx, admin)
	if err != nil {
		return nil, fmt.Errorf("list ledger accounts: %w", err)
	}

	var verr resource.ValidationError
	name := strings.TrimSpace(in.Name)
	named := tpl.NameLabel.EN != ""
	switch {
	case named && name == "":
		verr.Add("name", "can't be blank")
	case len(name) > 200:
		verr.Add("name", "is too long (maximum is 200 characters)")
	}
	accounts := seed.TemplateAccounts(tpl)
	byKey := map[string]countries.Account{}
	for _, sa := range accounts {
		byKey[sa.Key] = sa
	}
	wanted := map[string]bool{}
	for _, sa := range accounts {
		wanted[sa.Key] = in.Accounts == nil
	}
	for _, k := range in.Accounts {
		if _, ok := byKey[k]; !ok {
			verr.Add("ledger_accounts", k+" is not a category of this theme")
			continue
		}
		// A category goes under its parent, so the parents come along.
		for at := k; at != "" && !wanted[at]; at = byKey[at].Parent {
			wanted[at] = true
		}
	}
	if err := verr.OrNil(); err != nil {
		return nil, err
	}

	keys := keysOf(all)
	var missing []countries.Account
	for _, sa := range accounts {
		if !wanted[sa.Key] {
			continue
		}
		if named {
			sa = withName(sa, name, byKey)
			// Another entrepreneur's categories, or ones made by hand, may carry the name already.
			if slices.ContainsFunc(all, func(o LedgerAccount) bool {
				return o.Type == sa.Type && strings.EqualFold(o.Name, sa.Name.In(a.Language))
			}) {
				continue
			}
		}
		if keys[sa.Key] {
			continue
		}
		if !ownScheme {
			sa.Taxonomy = ""
		}
		missing = append(missing, sa)
	}
	if len(missing) == 0 {
		return nil, nil
	}
	added := s.accountsFrom(a.Administration, parentsFirst(missing), all, key)
	// A name taken by a category of the same type gets the template's name
	// added, so both stay recognisable.
	for j := range added {
		for _, o := range all {
			if o.Type == added[j].Type && strings.EqualFold(o.Name, added[j].Name) {
				added[j].Name += " (" + tpl.Name.In(a.Language) + ")"
				break
			}
		}
	}
	if err := s.store.CreateLedgerAccounts(ctx, added); err != nil {
		return nil, fmt.Errorf("store ledger accounts: %w", err)
	}
	return added, nil
}

// withName completes an account's name with the name the template asks for.
// Its key, and its parent's within the template, get the name too, so each
// name has its own categories.
func withName(sa countries.Account, name string, inTemplate map[string]countries.Account) countries.Account {
	suffix := ":" + strings.ToLower(name)
	sa.Key += suffix
	if _, ok := inTemplate[sa.Parent]; ok {
		sa.Parent += suffix
	}
	sa.Name = countries.Text{EN: strings.ReplaceAll(sa.Name.EN, countries.Placeholder, name), NL: strings.ReplaceAll(sa.Name.NL, countries.Placeholder, name)}
	return sa
}

// parentsFirst orders accounts so each comes after its parent among them.
func parentsFirst(in []countries.Account) []countries.Account {
	byKey := map[string]countries.Account{}
	for _, sa := range in {
		byKey[sa.Key] = sa
	}
	depth := func(sa countries.Account) int {
		d := 0
		for p, ok := byKey[sa.Parent]; ok && d <= len(in); p, ok = byKey[p.Parent] {
			d++
		}
		return d
	}
	out := slices.Clone(in)
	slices.SortStableFunc(out, func(x, y countries.Account) int { return depth(x) - depth(y) })
	return out
}

// applySingle adds the one category a single template adds: the name the
// user gave, of the kind they chose, under the parent they chose.
func (s *Service) applySingle(ctx context.Context, a administration.Administration, tpl countries.Template, in TemplateInput, ownScheme bool) ([]LedgerAccount, error) {
	single := tpl.Single
	i := slices.IndexFunc(single.Options, func(o countries.Option) bool { return o.Key == in.Option })
	c := LedgerAccountChanges{Name: &in.Name, Type: &single.Type, ParentID: in.ParentID}
	if i >= 0 && ownScheme {
		c.Taxonomy = &single.Options[i].Taxonomy
	}
	if !single.Parent {
		c.ParentID = nil
	}
	now := s.now()
	l := LedgerAccount{
		ID: s.ids.Next(), AdministrationID: a.ID, Active: true, AllowedDocumentTypes: DefaultDocumentTypes(single.Type),
		Template: tpl.Key, CreatedAt: now, UpdatedAt: now,
	}
	err := s.applyLedgerAccount(ctx, &l, c, true)
	if i < 0 {
		verr := &resource.ValidationError{}
		if err != nil && !errors.As(err, &verr) {
			return nil, err
		}
		verr.Add("option", "is not included in the list")
		err = verr
	}
	if err != nil {
		return nil, err
	}
	if err := s.store.CreateLedgerAccounts(ctx, []LedgerAccount{l}); err != nil {
		return nil, fmt.Errorf("store ledger account: %w", err)
	}
	return []LedgerAccount{l}, nil
}
