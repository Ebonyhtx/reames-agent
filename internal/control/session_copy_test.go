package control

import (
	"os"
	"path/filepath"
	"testing"

	"reames-agent/internal/agent"
	"reames-agent/internal/provider"
)

func TestCopySessionForWritingPreservesTranscriptAndPresentationMeta(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.jsonl")
	session := agent.NewSession("system")
	session.Add(provider.Message{Role: provider.RoleUser, Content: "question"})
	session.Add(provider.Message{Role: provider.RoleAssistant, Content: "answer"})
	if err := session.Save(src); err != nil {
		t.Fatal(err)
	}
	if err := agent.SaveBranchMeta(src, agent.BranchMeta{
		CustomTitle:   "Debugging",
		Model:         "deepseek/deepseek-chat",
		SchemaVersion: agent.BranchMetaCountsVersion,
	}); err != nil {
		t.Fatal(err)
	}
	srcBytes, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}

	copyPath, err := CopySessionForWriting(src)
	if err != nil {
		t.Fatalf("CopySessionForWriting: %v", err)
	}
	if filepath.Dir(copyPath) != dir || copyPath == src {
		t.Fatalf("copy path = %q, want fresh path beside %q", copyPath, src)
	}

	copySession, err := agent.LoadSession(copyPath)
	if err != nil {
		t.Fatalf("load copy: %v", err)
	}
	got, want := copySession.Snapshot(), session.Snapshot()
	if len(got) != len(want) {
		t.Fatalf("copy messages = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Role != want[i].Role || got[i].Content != want[i].Content {
			t.Fatalf("copy message %d = %+v, want %+v", i, got[i], want[i])
		}
	}

	meta, ok, err := agent.LoadBranchMeta(copyPath)
	if err != nil || !ok {
		t.Fatalf("copy branch meta: ok=%v err=%v", ok, err)
	}
	if meta.ParentID != agent.BranchID(src) || meta.ForkMessageIndex != len(want) {
		t.Fatalf("copy lineage = parent:%q index:%d", meta.ParentID, meta.ForkMessageIndex)
	}
	if meta.CustomTitle != "Debugging (copy)" || meta.Model != "deepseek/deepseek-chat" {
		t.Fatalf("copy presentation meta = title:%q model:%q", meta.CustomTitle, meta.Model)
	}

	srcAfter, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if string(srcAfter) != string(srcBytes) {
		t.Fatal("copy modified the source transcript")
	}
	if SessionLeaseHeld(copyPath) {
		t.Fatal("fresh copy unexpectedly owns a session lease")
	}
}

func TestCopySessionForWritingRejectsCleanupPendingSource(t *testing.T) {
	src := filepath.Join(t.TempDir(), "pending.jsonl")
	if err := MarkSessionCleanupPending(src, "delete"); err != nil {
		t.Fatal(err)
	}
	if _, err := CopySessionForWriting(src); err == nil {
		t.Fatal("CopySessionForWriting should reject a cleanup-pending source")
	}
}
