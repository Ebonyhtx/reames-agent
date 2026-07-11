package boot

import (
	"context"
	"strings"
	"testing"

	"reames-agent/internal/provider"
)

type sessionTitleProvider struct {
	request provider.Request
	chunks  []provider.Chunk
}

func (p *sessionTitleProvider) Name() string { return "session-title" }

func (p *sessionTitleProvider) Stream(_ context.Context, request provider.Request) (<-chan provider.Chunk, error) {
	p.request = request
	ch := make(chan provider.Chunk, len(p.chunks))
	for _, chunk := range p.chunks {
		ch <- chunk
	}
	close(ch)
	return ch, nil
}

func TestSessionTitleGeneratorBoundsInputAndCleansQuotes(t *testing.T) {
	prov := &sessionTitleProvider{chunks: []provider.Chunk{
		{Type: provider.ChunkText, Text: ` "Parser repair" `},
		{Type: provider.ChunkUsage, Usage: &provider.Usage{TotalTokens: 12}},
	}}
	generator := &SessionTitleGenerator{provider: prov}
	if got := generator.Generate(context.Background(), strings.Repeat("界", 320)); got != "Parser repair" {
		t.Fatalf("title = %q", got)
	}
	if len(prov.request.Messages) != 2 || prov.request.Messages[0].Content != sessionTitlePrompt {
		t.Fatalf("request = %+v", prov.request)
	}
	if got := []rune(prov.request.Messages[1].Content); len(got) != 303 || string(got[300:]) != "..." {
		t.Fatalf("bounded user message runes = %d tail=%q", len(got), string(got[max(0, len(got)-3):]))
	}
	if prov.request.MaxTokens != 20 || prov.request.Temperature == nil || *prov.request.Temperature != 0 {
		t.Fatalf("generation bounds = %+v", prov.request)
	}
}

func TestSessionTitleGeneratorFailsClosedOnChunkError(t *testing.T) {
	generator := &SessionTitleGenerator{provider: &sessionTitleProvider{chunks: []provider.Chunk{{Type: provider.ChunkError, Err: context.Canceled}}}}
	if got := generator.Generate(context.Background(), "task"); got != "" {
		t.Fatalf("error title = %q", got)
	}
}
