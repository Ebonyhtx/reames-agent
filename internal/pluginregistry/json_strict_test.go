package pluginregistry

import (
	"strings"
	"testing"
)

func TestDecodeAndValidateIndexRejectsDuplicateObjectKeys(t *testing.T) {
	validEntry := `{
  "name":"demo","description":"demo","version":"1.0.0",
  "source":"https://github.com/example/demo",
  "revision":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "digest":"sha256-git-tree-v1:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
  "permissions":["skills.load"],
  "provenance":{
    "source":"https://github.com/example/demo",
    "revision":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "digest":"sha256-git-tree-v1:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
  }
}`
	for name, body := range map[string]string{
		"top level":         `{"schemaVersion":1,"registry":"one","registry":"two","updated":"2026-07-16T00:00:00Z","plugins":[]}`,
		"nested provenance": strings.Replace(validEntry, `"source":"https://github.com/example/demo",`, `"source":"https://github.com/example/demo","source":"https://github.com/attacker/demo",`, 2),
	} {
		t.Run(name, func(t *testing.T) {
			if name == "nested provenance" {
				body = `{"schemaVersion":1,"registry":"test","updated":"2026-07-16T00:00:00Z","plugins":[` + body + `]}`
			}
			_, err := decodeAndValidateIndex([]byte(body))
			if err == nil || !strings.Contains(err.Error(), "duplicate object key") {
				t.Fatalf("duplicate key error = %v", err)
			}
		})
	}
}
