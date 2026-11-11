package main

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

// theirs is a small Moneybird-style description: paths carry {format},
// parameters and schemas sit behind references, and response codes are
// unquoted keys.
const theirs = `
paths:
  /{administration_id}/contacts/{id}{format}:
    parameters:
      - $ref: '#/components/parameters/query'
    patch:
      parameters:
        - {name: id, in: path}
        - {name: include_archived, in: query}
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties:
                contact:
                  $ref: '#/components/schemas/contact_input'
                remove_note: {type: boolean}
      responses:
        200:
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/contact'
        404: {}
  /{administration_id}/contacts{format}:
    get:
      responses:
        200:
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: '#/components/schemas/contact'
components:
  parameters:
    query: {name: query, in: query}
  schemas:
    contact_input:
      type: object
      properties:
        company_name: {type: string}
        firstname: {type: string}
    contact:
      allOf:
        - type: object
          properties:
            id: {type: string}
        - type: object
          properties:
            company_name: {type: string}
`

// matching is the same API as MoneyHarpy describes it.
const matching = `
paths:
  /api/v2/{administration_id}/contacts/{contact_id}:
    patch:
      parameters:
        - {name: query, in: query}
        - {name: include_archived, in: query}
      requestBody:
        content:
          application/json:
            schema:
              properties:
                contact:
                  properties:
                    company_name: {}
                    firstname: {}
                remove_note: {}
      responses:
        '200':
          content:
            application/json:
              schema:
                properties:
                  id: {}
                  company_name: {}
                  extra_of_ours: {}
        '404': {}
        '429': {}
  /api/v2/{administration_id}/contacts:
    get:
      responses:
        '200':
          content:
            application/json:
              schema:
                items:
                  properties:
                    id: {}
                    company_name: {}
`

func TestCompareOperationsMatching(t *testing.T) {
	gaps, checked := compareOperations(parse(t, theirs), parse(t, matching))
	if checked != 2 || len(gaps) != 0 {
		t.Errorf("checked %d, gaps %v", checked, gaps)
	}
}

func TestCompareOperationsReportsGaps(t *testing.T) {
	ours := `
paths:
  /api/v2/{administration_id}/contacts/{id}:
    patch:
      parameters:
        - {name: query, in: query}
      requestBody:
        content:
          application/json:
            schema:
              properties:
                contact:
                  properties:
                    company_name: {}
      responses:
        '200':
          content:
            application/json:
              schema:
                properties:
                  id: {}
`
	gaps, checked := compareOperations(parse(t, theirs), parse(t, ours))
	want := []string{
		"GET /{administration_id}/contacts: missing",
		"PATCH /{administration_id}/contacts/{id}: query parameters include_archived; status codes 404; request fields contact.firstname, remove_note; response fields company_name",
	}
	if checked != 2 {
		t.Errorf("checked = %d", checked)
	}
	if diff := cmp.Diff(want, gaps); diff != "" {
		t.Errorf("gaps (-want +got):\n%s", diff)
	}
}

func TestOperationKey(t *testing.T) {
	for in, want := range map[string]string{
		"/{administration_id}/contacts/{id}{format}":            "/{}/contacts/{}",
		"/api/v2/{administration_id}/contacts/{contact_id}":     "/{}/contacts/{}",
		"/api/v2/{administration_id}/contacts/{id}/notes/{nid}": "/{}/contacts/{}/notes/{}",
		"/administrations{format}":                              "/administrations",
	} {
		if got := pathKey(in); got != want {
			t.Errorf("pathKey(%q) = %q, want %q", in, got, want)
		}
	}
}
