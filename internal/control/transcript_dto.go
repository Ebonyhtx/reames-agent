package control

import (
	"reames-agent/internal/agent"
	"reames-agent/internal/provider"
)

// TranscriptRole is a stable display role. It deliberately mirrors the
// provider role strings without exposing provider.Message to transports.
type TranscriptRole string

const (
	TranscriptSystem    TranscriptRole = "system"
	TranscriptUser      TranscriptRole = "user"
	TranscriptAssistant TranscriptRole = "assistant"
	TranscriptTool      TranscriptRole = "tool"
)

// TranscriptToolCall is the display-safe form of one historical tool call.
type TranscriptToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Diff      string `json:"diff,omitempty"`
	Added     int    `json:"added,omitempty"`
	Removed   int    `json:"removed,omitempty"`
}

// TranscriptMemoryCitation is local display metadata for a historical answer.
type TranscriptMemoryCitation struct {
	ID        string `json:"id,omitempty"`
	Source    string `json:"source"`
	LineStart int    `json:"lineStart,omitempty"`
	LineEnd   int    `json:"lineEnd,omitempty"`
	Note      string `json:"note,omitempty"`
	Kind      string `json:"kind,omitempty"`
}

// TranscriptMessage is a transport-stable, display-safe history entry. Hidden
// entries retain their index/role for correlation but never expose system
// prompts or controller-generated synthetic content. User Content has transient
// composition and referenced-context payloads removed; model input remains only
// in the runtime session.
type TranscriptMessage struct {
	Index           int                        `json:"index"`
	Role            TranscriptRole             `json:"role"`
	Content         string                     `json:"content,omitempty"`
	Reasoning       string                     `json:"reasoning,omitempty"`
	ToolCalls       []TranscriptToolCall       `json:"toolCalls,omitempty"`
	ToolCallID      string                     `json:"toolCallId,omitempty"`
	ToolName        string                     `json:"toolName,omitempty"`
	MemoryCitations []TranscriptMemoryCitation `json:"memoryCitations,omitempty"`
	Edited          bool                       `json:"edited,omitempty"`
	Original        string                     `json:"original,omitempty"`
	SteerText       string                     `json:"steerText,omitempty"`
	Hidden          bool                       `json:"hidden,omitempty"`
}

// Transcript returns the current session history without leaking provider
// runtime types or hidden prompt material to a transport.
func (c *Controller) Transcript() []TranscriptMessage {
	return transcriptMessages(c.History())
}

// LoadTranscript returns a display-safe projection of one persisted session.
func LoadTranscript(path string) ([]TranscriptMessage, error) {
	session, err := agent.LoadSession(path)
	if err != nil {
		return nil, err
	}
	return transcriptMessages(session.Snapshot()), nil
}

func transcriptMessages(messages []provider.Message) []TranscriptMessage {
	out := make([]TranscriptMessage, len(messages))
	for i, message := range messages {
		entry := TranscriptMessage{Index: i, Role: TranscriptRole(message.Role)}
		switch message.Role {
		case provider.RoleSystem:
			entry.Hidden = true
		case provider.RoleUser:
			if text, ok := agent.SteerText(message.Content); ok {
				entry.Content = text
				entry.SteerText = text
				break
			}
			display := StripReferencedContextPrefix(StripComposePrefixes(message.Content))
			if IsSyntheticUserMessage(display) {
				entry.Hidden = true
				break
			}
			entry.Content = display
			entry.Edited = message.Edited
			entry.Original = StripReferencedContextPrefix(StripComposePrefixes(message.Original))
		case provider.RoleAssistant:
			entry.Content = message.Content
			entry.Reasoning = message.ReasoningContent
			if len(message.ToolCalls) > 0 {
				entry.ToolCalls = make([]TranscriptToolCall, len(message.ToolCalls))
				for j, call := range message.ToolCalls {
					entry.ToolCalls[j] = TranscriptToolCall{
						ID: call.ID, Name: call.Name, Arguments: call.Arguments,
						Diff: call.Diff, Added: call.Added, Removed: call.Removed,
					}
				}
			}
			if len(message.MemoryCitations) > 0 {
				entry.MemoryCitations = make([]TranscriptMemoryCitation, len(message.MemoryCitations))
				for j, citation := range message.MemoryCitations {
					entry.MemoryCitations[j] = TranscriptMemoryCitation{
						ID: citation.ID, Source: citation.Source, LineStart: citation.LineStart,
						LineEnd: citation.LineEnd, Note: citation.Note, Kind: citation.Kind,
					}
				}
			}
		case provider.RoleTool:
			entry.Content = message.Content
			entry.ToolCallID = message.ToolCallID
			entry.ToolName = message.Name
		default:
			entry.Hidden = true
		}
		out[i] = entry
	}
	return out
}
