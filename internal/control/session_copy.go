package control

import (
	"fmt"
	"path/filepath"
	"strings"

	"reames-agent/internal/agent"
)

// CopySessionForWriting duplicates src beside the original transcript and
// returns a fresh, unleased session path. It is the shared persistence boundary
// behind the CLI --copy escape hatch: the source is read only, while the copy
// receives an independent event log and branch sidecar rooted at src.
func CopySessionForWriting(src string) (string, error) {
	if agent.IsCleanupPending(src) {
		return "", fmt.Errorf("session is pending cleanup")
	}
	loaded, err := agent.LoadSession(src)
	if err != nil {
		return "", err
	}
	msgs := loaded.Snapshot()

	var srcMeta agent.BranchMeta
	if meta, ok, metaErr := agent.LoadBranchMeta(src); metaErr == nil && ok {
		srcMeta = meta
	}
	label := "session"
	if model, ok := agent.LoadSessionModel(src); ok && strings.TrimSpace(model) != "" {
		label = model
	}

	newPath := agent.NewSessionPath(filepath.Dir(src), label)
	copySession := agent.NewSession("")
	copySession.Messages = msgs
	if err := copySession.Save(newPath); err != nil {
		return "", fmt.Errorf("copy session: %w", err)
	}
	preview, turns := agent.SessionPreviewFromMessages(msgs)
	meta := agent.BranchMeta{
		ParentID:         agent.BranchID(src),
		ForkTurn:         -1,
		ForkMessageIndex: len(msgs),
		Preview:          preview,
		Turns:            turns,
		SchemaVersion:    agent.BranchMetaCountsVersion,
		Model:            srcMeta.Model,
	}
	if title := strings.TrimSpace(firstNonEmptyString(srcMeta.CustomTitle, srcMeta.TopicTitle)); title != "" {
		meta.CustomTitle = title + " (copy)"
	}
	if err := agent.SaveBranchMeta(newPath, meta); err != nil {
		return "", fmt.Errorf("copy session meta: %w", err)
	}
	return newPath, nil
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
