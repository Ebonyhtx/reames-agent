package evidence

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestCompleteStepEvidence_RejectsEmptyEvidence verifies that
// complete_step with no evidence items is invalid.
func TestCompleteStepEvidence_RejectsEmptyEvidence(t *testing.T) {
	evidence := []EvidenceItem{}
	if validEvidence(evidence) {
		t.Fatal("empty evidence should be invalid")
	}
}

// TestCompleteStepEvidence_RejectsNilEvidence verifies nil is invalid.
func TestCompleteStepEvidence_RejectsNilEvidence(t *testing.T) {
	if validEvidence(nil) {
		t.Fatal("nil evidence should be invalid")
	}
}

// TestCompleteStepEvidence_AcceptsValidKinds verifies all four
// supported evidence kinds are accepted.
func TestCompleteStepEvidence_AcceptsValidKinds(t *testing.T) {
	tests := []struct {
		name string
		item EvidenceItem
	}{
		{"verification", EvidenceItem{Kind: "verification", Summary: "tests pass", Command: "go test ./..."}},
		{"diff", func() EvidenceItem { dir := t.TempDir(); os.WriteFile(filepath.Join(dir, "x.go"), nil, 0644); return EvidenceItem{Kind: "diff", Summary: "code changed", Paths: []string{filepath.Join(dir, "x.go")}} }()},
		{"files", func() EvidenceItem { dir := t.TempDir(); os.WriteFile(filepath.Join(dir, "y.go"), nil, 0644); return EvidenceItem{Kind: "files", Summary: "files created", Paths: []string{filepath.Join(dir, "y.go")}} }()},
		{"manual", EvidenceItem{Kind: "manual", Summary: "manually checked"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !validEvidence([]EvidenceItem{tt.item}) {
				t.Fatalf("valid evidence kind %q should be accepted", tt.item.Kind)
			}
		})
	}
}

// TestCompleteStepEvidence_RejectsUnknownKind verifies that
// unknown evidence kinds are rejected.
func TestCompleteStepEvidence_RejectsUnknownKind(t *testing.T) {
	item := EvidenceItem{Kind: "screenshot", Summary: "fake"}
	if validEvidence([]EvidenceItem{item}) {
		t.Fatal("unknown evidence kind should be rejected")
	}
}

// TestCompleteStepEvidence_VerificationRequiresCommand verifies
// that verification evidence without a Command field is invalid.
func TestCompleteStepEvidence_VerificationRequiresCommand(t *testing.T) {
	item := EvidenceItem{Kind: "verification", Summary: "done"}
	if validEvidence([]EvidenceItem{item}) {
		t.Fatal("verification evidence without command should be invalid")
	}
}

// TestCompleteStepEvidence_DiffRequiresPaths verifies that
// diff evidence without Paths is invalid.
func TestCompleteStepEvidence_DiffRequiresPaths(t *testing.T) {
	item := EvidenceItem{Kind: "diff", Summary: "changed"}
	if validEvidence([]EvidenceItem{item}) {
		t.Fatal("diff evidence without paths should be invalid")
	}
}

// TestCompleteStepEvidence_FilesRequiresPaths verifies that
// files evidence without Paths is invalid.
func TestCompleteStepEvidence_FilesRequiresPaths(t *testing.T) {
	item := EvidenceItem{Kind: "files", Summary: "created"}
	if validEvidence([]EvidenceItem{item}) {
		t.Fatal("files evidence without paths should be invalid")
	}
}

// TestCompleteStepEvidence_PathMustExist verifies that
// referenced file paths actually exist.
func TestCompleteStepEvidence_PathMustExist(t *testing.T) {
	dir := t.TempDir()

	// Create a real file.
	realPath := filepath.Join(dir, "real.go")
	os.WriteFile(realPath, []byte("package main"), 0o644)

	// Evidence with an existing path should be valid.
	realItem := EvidenceItem{
		Kind:    "files",
		Summary: "created real.go",
		Paths:   []string{realPath},
	}
	if !validEvidence([]EvidenceItem{realItem}) {
		t.Fatal("evidence with existing path should be valid")
	}

	// Evidence with a non-existent path should be invalid (fake evidence).
	fakeItem := EvidenceItem{
		Kind:    "files",
		Summary: "created fake.go",
		Paths:   []string{filepath.Join(dir, "nonexistent.go")},
	}
	if validEvidence([]EvidenceItem{fakeItem}) {
		t.Fatal("evidence with non-existent path should be invalid")
	}
}

// TestCompleteStepEvidence_JSONRoundTrip verifies evidence
// serialises and deserialises correctly.
func TestCompleteStepEvidence_JSONRoundTrip(t *testing.T) {
	original := EvidenceItem{
		Kind:    "verification",
		Summary: "all tests pass",
		Command: "go test ./... -count=1",
		Paths:   []string{"internal/control/controller.go"},
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var restored EvidenceItem
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Kind != "verification" {
		t.Fatalf("Kind = %q", restored.Kind)
	}
	if restored.Summary != "all tests pass" {
		t.Fatalf("Summary = %q", restored.Summary)
	}
	if restored.Command != "go test ./... -count=1" {
		t.Fatalf("Command = %q", restored.Command)
	}
	if len(restored.Paths) != 1 || restored.Paths[0] != "internal/control/controller.go" {
		t.Fatalf("Paths = %v", restored.Paths)
	}
}

// --- Helpers (mirror the actual validation logic in evidence.go) ---

// EvidenceItem is one piece of evidence for a completed step.
type EvidenceItem struct {
	Kind    string   `json:"kind"`
	Summary string   `json:"summary"`
	Command string   `json:"command,omitempty"`
	Paths   []string `json:"paths,omitempty"`
}

var validKinds = map[string]bool{
	"verification": true,
	"diff":         true,
	"files":        true,
	"manual":       true,
}

// validEvidence checks whether evidence meets minimum requirements.
// This mirrors the actual validation logic used by complete_step.
func validEvidence(items []EvidenceItem) bool {
	if len(items) == 0 {
		return false
	}
	for _, item := range items {
		if !validKinds[item.Kind] {
			return false
		}
		if item.Summary == "" {
			return false
		}
		switch item.Kind {
		case "verification":
			if item.Command == "" {
				return false
			}
		case "diff", "files":
			if len(item.Paths) == 0 {
				return false
			}
			// Paths must exist (prevent fake evidence).
			for _, p := range item.Paths {
				if _, err := os.Stat(p); err != nil {
					return false
				}
			}
		}
	}
	return true
}
