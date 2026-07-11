package control

import (
	"log/slog"
	"strings"

	"reames-agent/internal/provider"
)

// SessionHistorySnapshot is an opaque, immutable transport handoff used while a
// frontend rebuilds a controller. Runtime message types stay inside control.
type SessionHistorySnapshot struct{ messages []provider.Message }

type sessionHistorySource interface{ History() []provider.Message }

func CaptureSessionHistory(source sessionHistorySource) SessionHistorySnapshot {
	if source == nil {
		return SessionHistorySnapshot{}
	}
	return SessionHistorySnapshot{messages: append([]provider.Message(nil), source.History()...)}
}

func systemPromptFromMessages(messages []provider.Message) string {
	for _, message := range messages {
		if message.Role == provider.RoleSystem {
			return message.Content
		}
	}
	return ""
}

func messagesWithSystemPrompt(messages []provider.Message, system string) []provider.Message {
	if strings.TrimSpace(system) == "" {
		return messages
	}
	out := append([]provider.Message(nil), messages...)
	for i := range out {
		if out[i].Role == provider.RoleSystem {
			out[i] = provider.Message{Role: provider.RoleSystem, Content: system}
			return out
		}
	}
	return append([]provider.Message{{Role: provider.RoleSystem, Content: system}}, out...)
}

// AdoptHistoryWithCurrentSystemPrompt resumes carried history using the newly
// built controller's system prompt. AdoptHistory preserves a compatible disk
// baseline, so a later rewrite cannot silently overwrite a newer transcript.
func (c *Controller) AdoptHistoryWithCurrentSystemPrompt(messages []provider.Message, path string) {
	fresh := systemPromptFromMessages(c.History())
	persisted := systemPromptFromMessages(messages)
	if persisted != "" && fresh != "" && persisted != fresh {
		slog.Warn("control: resume swapped a differing system prompt; conversation prefix cache will miss",
			"path", path, "persisted_len", len(persisted), "fresh_len", len(fresh))
	}
	c.AdoptHistory(messagesWithSystemPrompt(messages, fresh), path)
}

// AdoptLoadedHistoryWithCurrentSystemPrompt preserves legacy system-less
// transcripts exactly. A loaded session that already has a system message is
// refreshed; a bare historical user transcript is not silently given a new
// visible entry during restore.
func (c *Controller) AdoptLoadedHistoryWithCurrentSystemPrompt(messages []provider.Message, path string) {
	if systemPromptFromMessages(messages) == "" {
		c.AdoptHistory(messages, path)
		return
	}
	c.AdoptHistoryWithCurrentSystemPrompt(messages, path)
}

// AdoptSessionHistoryWithCurrentSystemPrompt consumes an opaque history
// captured before a potentially slow controller rebuild.
func (c *Controller) AdoptSessionHistoryWithCurrentSystemPrompt(history SessionHistorySnapshot, path string) {
	c.AdoptHistoryWithCurrentSystemPrompt(history.messages, path)
}
