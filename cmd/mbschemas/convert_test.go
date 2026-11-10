package main

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"gopkg.in/yaml.v3"
)

func parse(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := yaml.Unmarshal([]byte(s), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestConvertSchema(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{
			name: "nullable type list",
			in:   `{type: [string, "null"], description: theirs, example: x}`,
			want: `{type: string, nullable: true}`,
		},
		{
			name: "several types become anyOf, keeping string rules on the string branch",
			in:   `{type: [string, integer], pattern: '^\d+$'}`,
			want: `{anyOf: [{type: string, pattern: '^\d+$'}, {type: integer}]}`,
		},
		{
			name: "array branches get items, object branches keep their properties",
			in:   `{type: [string, object, array], additionalProperties: {type: string}}`,
			want: `{anyOf: [{type: string}, {type: object, additionalProperties: {type: string}}, {type: array, items: {}}]}`,
		},
		{
			name: "null branch of oneOf becomes nullable",
			in:   `{oneOf: [{type: string}, {type: number}, {type: "null"}]}`,
			want: `{oneOf: [{type: string}, {type: number}], nullable: true}`,
		},
		{
			name: "null in enum becomes nullable",
			in:   `{type: [string, "null"], enum: [a, b, null]}`,
			want: `{type: string, enum: [a, b], nullable: true}`,
		},
		{
			name: "3.1-only keywords and defaults go",
			in:   `{type: object, unevaluatedProperties: false, properties: {a: {const: x}, b: {type: boolean, default: 'default'}}}`,
			want: `{type: object, properties: {a: {enum: [x]}, b: {type: boolean}}}`,
		},
		{
			name: "ref siblings go and identifier maps to ours",
			in:   `{$ref: '#/components/schemas/identifier', description: theirs}`,
			want: `{$ref: '#/components/schemas/Identifier'}`,
		},
		{
			name: "nested additionalProperties and items convert too",
			in:   `{type: array, items: {type: object, additionalProperties: {type: [string, boolean]}}}`,
			want: `{type: array, items: {type: object, additionalProperties: {anyOf: [{type: string}, {type: boolean}]}}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convert(parse(t, tt.in))
			if diff := cmp.Diff(parse(t, tt.want), got); diff != "" {
				t.Errorf("convert() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSchemasSkipsOursAndDescribesTheirs(t *testing.T) {
	spec := parse(t, `
components:
  schemas:
    identifier: {type: [string, integer]}
    workflow_response: {type: object, properties: {id: {$ref: '#/components/schemas/identifier'}}}
`)
	got, err := schemas(spec)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["identifier"]; ok {
		t.Error("schemas() kept identifier, want it mapped to Identifier")
	}
	w := got["workflow_response"].(map[string]any)
	if w["description"] != "Moneybird-compatible workflow_response." {
		t.Errorf("description = %v", w["description"])
	}
}

func TestReplaceBlock(t *testing.T) {
	doc := "a: 1\n" + beginMarker + "\n    old: {}\n" + endMarker + "\n"
	got, err := replaceBlock(doc, "    new: {}\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "a: 1\n" + beginMarker + "\n    new: {}\n" + endMarker + "\n"
	if got != want {
		t.Errorf("replaceBlock() = %q, want %q", got, want)
	}
	if _, err := replaceBlock("a: 1\n", "x"); err == nil {
		t.Error("replaceBlock() without markers error = nil")
	}
}
