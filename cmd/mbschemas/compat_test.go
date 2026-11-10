//go:build moneybird

// The tests in this file compare the contract with Moneybird's live API
// description. They need the network, so they run only with -tags moneybird:
//
//	go test -tags moneybird ./cmd/mbschemas
package main

import (
	"maps"
	"os"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"
)

// ownSchemas pairs MoneyHarpy response schemas with the Moneybird schema they
// must stay compatible with.
var ownSchemas = map[string]string{
	"Administration": "administration_response",
	"Identity":       "identity_response",
	"CustomField":    "custom_field_response",
}

func TestContractMatchesLiveMoneybirdSchemas(t *testing.T) {
	if err := run(SpecURL, "../../api/openapi.yaml", true); err != nil {
		t.Fatal(err)
	}
}

func TestOwnSchemasHaveEveryMoneybirdField(t *testing.T) {
	raw, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var contract map[string]any
	if err := yaml.Unmarshal(raw, &contract); err != nil {
		t.Fatal(err)
	}
	all := contract["components"].(map[string]any)["schemas"].(map[string]any)
	for ours, theirs := range ownSchemas {
		have := all[ours].(map[string]any)["properties"].(map[string]any)
		want := all[theirs].(map[string]any)["properties"].(map[string]any)
		for _, field := range slices.Sorted(maps.Keys(want)) {
			if _, ok := have[field]; !ok {
				t.Errorf("%s lacks %s.%s", ours, theirs, field)
			}
		}
	}
}
