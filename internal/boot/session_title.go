package boot

import (
	"context"
	"strings"

	"reames-agent/internal/config"
	"reames-agent/internal/nilutil"
	"reames-agent/internal/provider"
)

const sessionTitlePrompt = `Generate a very short title (3-5 words max) for this conversation based on the user's first message. Reply with ONLY the title, no quotes, no punctuation at the end.`

// SessionTitleGenerator owns the optional lightweight provider used for saved
// session labels. Its usage is intentionally separate from the active turn and
// is not emitted onto the shared conversation event stream.
type SessionTitleGenerator struct{ provider provider.Provider }

// NewSessionTitleGenerator resolves the configured flash model. Failure is a
// supported state: callers fall back to a truncated local preview.
func NewSessionTitleGenerator() *SessionTitleGenerator {
	cfg, err := config.Load()
	if err != nil {
		return nil
	}
	entry, ok := cfg.ResolveModel("deepseek-flash")
	if !ok {
		return nil
	}
	prov, err := provider.New(entry.Kind, provider.Config{
		Name: entry.Name, BaseURL: entry.BaseURL, Model: entry.Model, APIKey: entry.APIKey(),
		Extra: map[string]any{"effort": "off"},
	})
	if err != nil {
		return nil
	}
	return &SessionTitleGenerator{provider: prov}
}

// Generate returns an empty title on any provider failure.
func (g *SessionTitleGenerator) Generate(ctx context.Context, firstMessage string) string {
	if g == nil || nilutil.IsNil(g.provider) || strings.TrimSpace(firstMessage) == "" {
		return ""
	}
	if runes := []rune(firstMessage); len(runes) > 300 {
		firstMessage = string(runes[:300]) + "..."
	}
	chunks, err := g.provider.Stream(ctx, provider.Request{
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: sessionTitlePrompt},
			{Role: provider.RoleUser, Content: firstMessage},
		},
		Temperature: provider.TemperaturePtr(0), MaxTokens: 20,
	})
	if err != nil {
		return ""
	}
	var text strings.Builder
	for chunk := range chunks {
		switch chunk.Type {
		case provider.ChunkText:
			text.WriteString(chunk.Text)
		case provider.ChunkError:
			return ""
		}
	}
	title := strings.TrimSpace(text.String())
	if len(title) >= 2 && ((title[0] == '"' && title[len(title)-1] == '"') || (title[0] == '\'' && title[len(title)-1] == '\'')) {
		title = title[1 : len(title)-1]
	}
	return strings.TrimSpace(title)
}
