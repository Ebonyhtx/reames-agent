package provider

import (
	"bytes"
	"encoding/json"
	"net/url"
	"strings"
)

// IsMiMoEndpoint reports whether rawURL points at an official Xiaomi MiMo API
// host, including regional token-plan subdomains. The bare apex is rejected
// because it is not an API endpoint.
func IsMiMoEndpoint(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host != "xiaomimimo.com" && strings.HasSuffix(host, ".xiaomimimo.com")
}

// NormalizeLegacyTupleItemsForDraft202012 rewrites only the pre-2020-12 tuple
// keywords in a JSON Schema. Provider implementations opt in only when the
// endpoint dialect is known, so other vendors keep byte-identical tool schemas
// and cache prefixes. The common no-op path does not parse or allocate.
func NormalizeLegacyTupleItemsForDraft202012(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || !bytes.Contains(raw, []byte(`"items"`)) {
		return raw
	}
	var schema any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return raw
	}
	if _, changed := normalizeDraft202012Schema(schema); !changed {
		return raw
	}
	out, err := json.Marshal(schema)
	if err != nil {
		return raw
	}
	return json.RawMessage(out)
}

// normalizeDraft202012Schema returns changes within the current schema
// resource and anywhere in the document separately. A nested $schema starts a
// resource boundary, so its conversion must not upgrade the parent's dialect.
func normalizeDraft202012Schema(value any) (resourceChanged, documentChanged bool) {
	schema, ok := value.(map[string]any)
	if !ok {
		return false, false
	}
	decl, hasDecl := schema["$schema"].(string)
	if hasDecl && !isLegacyJSONSchemaDialect(decl) && !isDraft202012Dialect(decl) {
		return false, false
	}
	changed := false
	document := false
	visit := func(child any) {
		childResource, childDocument := normalizeDraft202012Schema(child)
		changed = changed || childResource
		document = document || childDocument
	}

	for _, keyword := range []string{
		"additionalItems", "additionalProperties", "contains", "contentSchema",
		"else", "if", "items", "not", "propertyNames", "then",
		"unevaluatedItems", "unevaluatedProperties",
	} {
		visit(schema[keyword])
	}
	for _, keyword := range []string{"allOf", "anyOf", "oneOf", "prefixItems"} {
		if children, ok := schema[keyword].([]any); ok {
			for _, child := range children {
				visit(child)
			}
		}
	}
	for _, keyword := range []string{
		"$defs", "definitions", "dependentSchemas", "patternProperties", "properties",
	} {
		if children, ok := schema[keyword].(map[string]any); ok {
			for _, child := range children {
				visit(child)
			}
		}
	}
	if dependencies, ok := schema["dependencies"].(map[string]any); ok {
		for _, child := range dependencies {
			visit(child)
		}
	}

	if legacyItems, ok := schema["items"].([]any); ok {
		for _, child := range legacyItems {
			visit(child)
		}
		changed = true
		delete(schema, "items")
		if len(legacyItems) > 0 {
			if _, exists := schema["prefixItems"]; !exists {
				schema["prefixItems"] = legacyItems
			}
		}
		if additional, exists := schema["additionalItems"]; exists {
			delete(schema, "additionalItems")
			if isSchemaObjectOrBool(additional) {
				schema["items"] = additional
			}
		}
	}

	if changed {
		document = true
		if hasDecl && isLegacyJSONSchemaDialect(decl) {
			schema["$schema"] = "https://json-schema.org/draft/2020-12/schema"
		}
	}
	if hasDecl {
		return false, document
	}
	return changed, document
}

func isLegacyJSONSchemaDialect(decl string) bool {
	switch normalizeDialectURI(decl) {
	case "json-schema.org/schema",
		"json-schema.org/draft-03/schema",
		"json-schema.org/draft-04/schema",
		"json-schema.org/draft-06/schema",
		"json-schema.org/draft-07/schema",
		"json-schema.org/draft/2019-09/schema":
		return true
	}
	return false
}

func isDraft202012Dialect(decl string) bool {
	return normalizeDialectURI(decl) == "json-schema.org/draft/2020-12/schema"
}

func normalizeDialectURI(decl string) string {
	d := strings.TrimSuffix(strings.TrimSpace(decl), "#")
	d = strings.TrimPrefix(d, "http://")
	return strings.TrimPrefix(d, "https://")
}

func isSchemaObjectOrBool(value any) bool {
	switch value.(type) {
	case map[string]any, bool:
		return true
	default:
		return false
	}
}
