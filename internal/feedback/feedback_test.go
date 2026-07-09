package feedback

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNormalizeSanitizesSecretsAndFingerprints(t *testing.T) {
	rec, err := Normalize(ReportInput{
		Kind:         "crash",
		Source:       "desktop",
		Label:        "panic",
		Message:      `user alice@example.com hit api_key=sk-secret1234567890abcdef in C:\Users\Alice\repo`,
		ErrorMessage: `Bearer abcdefghijklmnopqrstuvwxyz123456`,
		TopFrame:     `/home/alice/repo/main.go:12`,
		Metadata: map[string]string{
			"token": "rk-secret1234567890abcdef",
		},
	}, time.Date(2026, 7, 10, 1, 2, 3, 4, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	joined := rec.Message + "\n" + rec.ErrorMessage + "\n" + rec.TopFrame + "\n" + rec.Metadata["token"]
	for _, forbidden := range []string{"alice@example.com", "sk-secret", "rk-secret", "abcdefghijklmnopqrstuvwxyz123456", `C:\Users\Alice`, "/home/alice"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("record leaked %q:\n%+v", forbidden, rec)
		}
	}
	for _, want := range []string{"[redacted-email]", "api_key=[redacted]", "Bearer [redacted]"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("record missing redaction marker %q:\n%s", want, joined)
		}
	}
	if rec.Fingerprint == "" || rec.ID == "" {
		t.Fatalf("record missing identifiers: %+v", rec)
	}
}

func TestStoreAppendAndSummaryDeduplicate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "feedback.jsonl")
	store := NewStore(path)
	store.now = func() time.Time { return time.Date(2026, 7, 10, 1, 0, 0, 0, time.UTC) }
	first, err := store.Append(ReportInput{
		Kind:         "feedback",
		Source:       "gateway",
		Label:        "feishu",
		Message:      "same failure",
		ErrorMessage: "connect failed",
		TopFrame:     "bot/feishu.go:10",
	})
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return time.Date(2026, 7, 10, 1, 1, 0, 0, time.UTC) }
	second, err := store.Append(ReportInput{
		Kind:         "feedback",
		Source:       "gateway",
		Label:        "feishu",
		Message:      "same failure",
		ErrorMessage: "connect failed",
		TopFrame:     "bot/feishu.go:10",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Fingerprint != second.Fingerprint || first.ID == second.ID {
		t.Fatalf("fingerprint/id mismatch: first=%+v second=%+v", first, second)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(raw), "\n") != 2 {
		t.Fatalf("jsonl records = %q, want two lines", raw)
	}
	summary, err := store.Summary(10)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Total != 2 || len(summary.Groups) != 1 || summary.Groups[0].Count != 2 {
		t.Fatalf("summary = %+v, want one duplicate group with count=2", summary)
	}
	if summary.Groups[0].LatestID != second.ID || summary.Groups[0].TopFrame != "bot/feishu.go:10" {
		t.Fatalf("summary group = %+v, want latest id and top frame", summary.Groups[0])
	}
}

func TestSummaryMissingFileIsEmpty(t *testing.T) {
	summary, err := SummarizeFile(filepath.Join(t.TempDir(), "missing.jsonl"), 10)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Total != 0 || len(summary.Groups) != 0 {
		t.Fatalf("summary = %+v, want empty", summary)
	}
}
