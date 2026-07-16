package provider

import (
	"encoding/json"
	"testing"
)

func TestIsMiMoEndpoint(t *testing.T) {
	for _, tc := range []struct {
		url  string
		want bool
	}{
		{"https://api.xiaomimimo.com/v1", true},
		{"https://token-plan-cn.xiaomimimo.com/v1", true},
		{"https://token-plan-sgp.xiaomimimo.com/anthropic", true},
		{"https://xiaomimimo.com/v1", false},
		{"https://api.deepseek.com", false},
		{"https://xiaomimimo.com.example.org", false},
		{"not-a-url", false},
	} {
		if got := IsMiMoEndpoint(tc.url); got != tc.want {
			t.Errorf("IsMiMoEndpoint(%q) = %v, want %v", tc.url, got, tc.want)
		}
	}
}

func TestNormalizeLegacyTupleItemsUpdatesDialectAndBoundaries(t *testing.T) {
	raw := json.RawMessage(`{
		"$schema":"https://json-schema.org/draft/2019-09/schema",
		"$defs":{
			"pair":{"type":"array","items":[{"type":"string"}],"additionalItems":false},
			"custom":{"$schema":"https://example.com/custom","type":"array","items":[{"type":"number"}]}
		}
	}`)
	var schema map[string]any
	if err := json.Unmarshal(NormalizeLegacyTupleItemsForDraft202012(raw), &schema); err != nil {
		t.Fatal(err)
	}
	if got := schema["$schema"]; got != "https://json-schema.org/draft/2020-12/schema" {
		t.Fatalf("root dialect = %v", got)
	}
	defs := schema["$defs"].(map[string]any)
	pair := defs["pair"].(map[string]any)
	if _, ok := pair["prefixItems"].([]any); !ok || pair["items"] != false {
		t.Fatalf("legacy tuple not converted: %v", pair)
	}
	custom := defs["custom"].(map[string]any)
	if _, ok := custom["items"].([]any); !ok {
		t.Fatalf("custom dialect resource changed: %v", custom)
	}
}

func TestNormalizeLegacyTupleItemsKeepsNoOpBytes(t *testing.T) {
	raw := json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}}}`)
	got := NormalizeLegacyTupleItemsForDraft202012(raw)
	if len(got) == 0 || &got[0] != &raw[0] {
		t.Fatal("schema without tuple items should return the original bytes")
	}
}
