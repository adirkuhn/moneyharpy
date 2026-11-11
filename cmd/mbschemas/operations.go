package main

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// An operation is what a client of one endpoint relies on: the query
// parameters it may send, the status codes it may get, the request fields
// it may send (nested one level, as "contact.firstname") and the fields a
// successful response carries.
type operation struct {
	path     string // as the description writes it, without {format}
	query    set
	codes    set
	request  set
	response set
}

type set map[string]bool

// missing returns the members of want that s lacks, sorted.
func (s set) missing(want set) []string {
	var out []string
	for k := range want {
		if !s[k] {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return out
}

var methods = []string{"get", "post", "put", "patch", "delete"}

// compareOperations lists, per Moneybird operation, what the contract lacks
// of it, and how many operations it checked. Extra parameters, codes and
// fields of MoneyHarpy's are fine; only missing ones are gaps.
func compareOperations(theirs, ours map[string]any) (gaps []string, checked int) {
	want, have := operations(theirs), operations(ours)
	for _, key := range slices.Sorted(maps.Keys(want)) {
		checked++
		w := want[key]
		method, _, _ := strings.Cut(key, " ")
		name := method + " " + w.path
		h, ok := have[key]
		if !ok {
			gaps = append(gaps, name+": missing")
			continue
		}
		var parts []string
		for _, c := range []struct {
			label      string
			have, want set
		}{
			{"query parameters", h.query, w.query},
			{"status codes", h.codes, w.codes},
			{"request fields", h.request, w.request},
			{"response fields", h.response, w.response},
		} {
			if miss := c.have.missing(c.want); len(miss) > 0 {
				parts = append(parts, c.label+" "+strings.Join(miss, ", "))
			}
		}
		if len(parts) > 0 {
			gaps = append(gaps, name+": "+strings.Join(parts, "; "))
		}
	}
	return gaps, checked
}

var pathParam = regexp.MustCompile(`\{[^}]+\}`)

// pathKey makes Moneybird's and MoneyHarpy's way of writing a path
// comparable: no /api/v2 prefix, no {format} suffix, and path parameters
// unnamed, since the two name them differently.
func pathKey(p string) string {
	p = strings.TrimPrefix(strings.ReplaceAll(p, "{format}", ""), "/api/v2")
	return pathParam.ReplaceAllString(p, "{}")
}

// operations indexes a description's operations by "METHOD path key".
func operations(spec map[string]any) map[string]operation {
	r := resolver{spec}
	out := map[string]operation{}
	for p, v := range asMap(spec["paths"]) {
		item := asMap(r.resolve(v))
		shared := asList(item["parameters"])
		for _, m := range methods {
			op := asMap(item[m])
			if op == nil {
				continue
			}
			o := operation{
				path:  strings.ReplaceAll(p, "{format}", ""),
				query: set{}, codes: set{}, request: set{}, response: set{},
			}
			for _, param := range append(slices.Clone(shared), asList(op["parameters"])...) {
				param := asMap(r.resolve(param))
				if param["in"] == "query" {
					o.query[fmt.Sprint(param["name"])] = true
				}
			}
			for code, resp := range asMap(op["responses"]) {
				o.codes[code] = true
				if strings.HasPrefix(code, "2") {
					for _, media := range asMap(asMap(r.resolve(resp))["content"]) {
						for f := range r.fields(asMap(media)["schema"], 0) {
							o.response[f] = true
						}
					}
				}
			}
			for _, media := range asMap(asMap(r.resolve(op["requestBody"]))["content"]) {
				r.requestFields(asMap(media)["schema"], o.request)
			}
			out[strings.ToUpper(m)+" "+pathKey(p)] = o
		}
	}
	return out
}

type resolver struct{ spec map[string]any }

// resolve follows local references ("#/components/...").
func (r resolver) resolve(v any) any {
	for range 20 {
		ref, ok := asMap(v)["$ref"].(string)
		if !ok {
			return v
		}
		v = r.spec
		for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
			v = asMap(v)[part]
		}
	}
	return v
}

// properties returns a schema's own properties, merged over allOf, anyOf
// and oneOf, and looking through arrays to their items.
func (r resolver) properties(schema any, depth int) map[string]any {
	s := asMap(r.resolve(schema))
	if s == nil || depth > 8 {
		return nil
	}
	if items, ok := s["items"]; ok {
		return r.properties(items, depth+1)
	}
	out := map[string]any{}
	for _, k := range []string{"allOf", "anyOf", "oneOf"} {
		for _, sub := range asList(s[k]) {
			maps.Copy(out, r.properties(sub, depth+1))
		}
	}
	maps.Copy(out, asMap(s["properties"]))
	return out
}

func (r resolver) fields(schema any, depth int) set {
	out := set{}
	for k := range r.properties(schema, depth) {
		out[k] = true
	}
	return out
}

// requestFields adds a request body's fields: nested ones as
// "wrapper.field", as Moneybird wraps them ({"contact": {...}}).
func (r resolver) requestFields(schema any, into set) {
	for name, sub := range r.properties(schema, 0) {
		inner := r.fields(sub, 1)
		if len(inner) == 0 {
			into[name] = true
		}
		for f := range inner {
			into[name+"."+f] = true
		}
	}
}

// asMap reads a YAML mapping, whose keys may be strings or, unquoted,
// numbers such as response codes.
func asMap(v any) map[string]any {
	switch m := v.(type) {
	case map[string]any:
		return m
	case map[any]any:
		out := make(map[string]any, len(m))
		for k, v := range m {
			out[fmt.Sprint(k)] = v
		}
		return out
	}
	return nil
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}
