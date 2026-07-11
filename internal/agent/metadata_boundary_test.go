package agent

import (
	"context"
	"testing"

	"reames-agent/internal/event"
	"reames-agent/internal/provider"
	"reames-agent/internal/tool"
)

type metadataCaptureProvider struct {
	request provider.Request
}

func (p *metadataCaptureProvider) Name() string { return "metadata-capture" }

func (p *metadataCaptureProvider) Stream(_ context.Context, request provider.Request) (<-chan provider.Chunk, error) {
	p.request = request
	ch := make(chan provider.Chunk, 2)
	ch <- provider.Chunk{Type: provider.ChunkText, Text: "ok"}
	ch <- provider.Chunk{Type: provider.ChunkUsage, Usage: &provider.Usage{PromptTokens: 10, CompletionTokens: 1, TotalTokens: 11}}
	close(ch)
	return ch, nil
}

func TestAgentStripsLocalMetadataBeforeProviderInterface(t *testing.T) {
	session := NewSession("stable system")
	session.Add(provider.Message{Role: provider.RoleUser, Content: "edited prompt", Edited: true, Original: "original prompt"})
	session.Add(provider.Message{Role: provider.RoleAssistant, Content: "answer", MemoryCitations: []provider.MemoryCitation{{ID: "m1", Source: "MEMORY.md"}}})
	prov := &metadataCaptureProvider{}
	a := New(prov, tool.NewRegistry(), session, Options{}, event.Discard)

	if err := a.Run(context.Background(), "continue"); err != nil {
		t.Fatal(err)
	}
	if len(prov.request.Messages) < 4 {
		t.Fatalf("provider request messages = %+v", prov.request.Messages)
	}
	for i, message := range prov.request.Messages {
		if message.Edited || message.Original != "" || len(message.MemoryCitations) > 0 {
			t.Fatalf("provider interface message %d leaked local metadata: %+v", i, message)
		}
	}
	stored := session.Messages
	if !stored[1].Edited || stored[1].Original != "original prompt" || len(stored[2].MemoryCitations) != 1 {
		t.Fatalf("session display metadata was not preserved: %+v", stored)
	}
}
