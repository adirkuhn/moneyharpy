// Package api embeds the OpenAPI contract of MoneyHarpy.
package api

import _ "embed"

// Spec is the OpenAPI document in YAML form.
//
//go:embed openapi.yaml
var Spec []byte
