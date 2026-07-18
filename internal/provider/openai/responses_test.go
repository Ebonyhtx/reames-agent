package openai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"reames-agent/internal/provider"
)

func TestBuildResponsesRequestMapsNativeWireItems(t *testing.T) {
	c := &client{
		model:           "gpt-test",
		apiMode:         apiModeResponses,
		vision:          true,
		visionDetail:    "high",
		openaiReasoning: true,
		effort:          "xhigh",
		extraBody: map[string]any{
			"service_tier":        "priority",
			"parallel_tool_calls": false,
			"input":               "must-not-override",
		},
	}
	req := provider.Request{
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: "stable system"},
			{Role: provider.RoleUser, Content: "inspect", Images: []string{"data:image/png;base64,abc"}},
			{Role: provider.RoleAssistant, Content: "working", ReasoningBlocks: []provider.ReasoningBlock{{Type: "openai_reasoning", Text: "checked the tree", Data: "encrypted-rs"}}, ToolCalls: []provider.ToolCall{{ID: "call-1", Name: "read_file", Arguments: `{"path":"a.go"}`}}},
			{Role: provider.RoleTool, ToolCallID: "call-1", Name: "read_file", Content: "package main"},
		},
		Tools:       []provider.ToolSchema{{Name: "read_file", Description: "Read one file", Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`)}},
		Temperature: provider.TemperaturePtr(0.7),
		MaxTokens:   4096,
	}

	wire := c.buildResponsesRequest(req)
	if wire.Instructions != "stable system" || wire.Model != "gpt-test" || wire.MaxOutputTokens != 4096 {
		t.Fatalf("request identity = %+v", wire)
	}
	if wire.Reasoning == nil || wire.Reasoning.Effort != "xhigh" || wire.Reasoning.Summary != "auto" {
		t.Fatalf("reasoning = %+v", wire.Reasoning)
	}
	if !wire.ParallelToolCalls || wire.ToolChoice != "auto" || wire.Store || !wire.Stream {
		t.Fatalf("Responses controls = %+v", wire)
	}
	if len(wire.Input) != 5 {
		t.Fatalf("input len = %d, want 5: %+v", len(wire.Input), wire.Input)
	}
	if got := wire.Input[0].Content; len(got) != 2 || got[0].Type != "input_text" || got[1].Type != "input_image" || got[1].Detail != "high" {
		t.Fatalf("user multimodal content = %+v", got)
	}
	if got := wire.Input[1]; got.Type != "reasoning" || got.EncryptedContent != "encrypted-rs" || len(got.Summary) != 1 || got.Summary[0].Text != "checked the tree" {
		t.Fatalf("reasoning replay item = %+v", got)
	}
	if got := wire.Input[2]; got.Type != "message" || got.Role != "assistant" || got.Content[0].Type != "output_text" {
		t.Fatalf("assistant item = %+v", got)
	}
	if got := wire.Input[3]; got.Type != "function_call" || got.CallID != "call-1" || got.Name != "read_file" {
		t.Fatalf("function call item = %+v", got)
	}
	if got := wire.Input[4]; got.Type != "function_call_output" || got.CallID != "call-1" || got.Output != "package main" {
		t.Fatalf("function output item = %+v", got)
	}
	if len(wire.Include) != 1 || wire.Include[0] != "reasoning.encrypted_content" {
		t.Fatalf("Responses include = %+v", wire.Include)
	}
	if len(wire.Tools) != 1 || wire.Tools[0].Type != "function" || wire.Tools[0].Strict {
		t.Fatalf("tools = %+v", wire.Tools)
	}

	raw, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{`"service_tier":"priority"`, `"parallel_tool_calls":true`, `"input":[`} {
		if !strings.Contains(text, want) {
			t.Fatalf("wire missing %s: %s", want, text)
		}
	}
	if strings.Contains(text, "must-not-override") {
		t.Fatalf("reserved extra_body field overrode input: %s", text)
	}
	if strings.Contains(text, `"temperature"`) {
		t.Fatalf("Responses wire must not inherit Chat-era temperature: %s", text)
	}
}

func TestBuildResponsesRequestLocalMetadataDoesNotChangeWireBytes(t *testing.T) {
	c := &client{model: "gpt-test", apiMode: apiModeResponses}
	base := []provider.Message{{Role: provider.RoleUser, Content: "hello"}}
	decorated := []provider.Message{{
		Role:            provider.RoleUser,
		Content:         "hello",
		Edited:          true,
		Original:        "old",
		MemoryCitations: []provider.MemoryCitation{{ID: "m1", Source: "memory.md"}},
	}}
	plain, err := json.Marshal(c.buildResponsesRequest(provider.Request{Messages: base}))
	if err != nil {
		t.Fatal(err)
	}
	withMetadata, err := json.Marshal(c.buildResponsesRequest(provider.Request{Messages: decorated}))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plain, withMetadata) {
		t.Fatalf("local metadata changed Responses wire/cache bytes:\nplain=%s\nmetadata=%s", plain, withMetadata)
	}
}

func TestResponsesStreamTextReasoningParallelToolsAndUsage(t *testing.T) {
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			http.Error(w, "wrong path", http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer secret" || r.Header.Get("Accept") != "text/event-stream" {
			http.Error(w, "missing headers", http.StatusUnauthorized)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, strings.Join([]string{
			`event: response.created`,
			`data: {"type":"response.created","response":{"id":"resp-1"}}`,
			``,
			`event: response.reasoning_summary_text.delta`,
			`data: {"type":"response.reasoning_summary_text.delta","delta":"think "}`,
			``,
			`event: response.output_item.done`,
			`data: {"type":"response.output_item.done","item":{"id":"rs-1","type":"reasoning","summary":[{"type":"summary_text","text":"think "}],"encrypted_content":"opaque-rs"}}`,
			``,
			`event: response.output_text.delta`,
			`data: {"type":"response.output_text.delta","delta":"hello"}`,
			``,
			`event: response.output_item.added`,
			`data: {"type":"response.output_item.added","item":{"id":"fc-1","type":"function_call","call_id":"call-1","name":"read_file","arguments":""}}`,
			``,
			`event: response.function_call_arguments.delta`,
			`data: {"type":"response.function_call_arguments.delta","item_id":"fc-1","delta":"{\"path\":\"a.go\"}"}`,
			``,
			`event: response.output_item.done`,
			`data: {"type":"response.output_item.done","item":{"id":"fc-1","type":"function_call","call_id":"call-1","name":"read_file","arguments":"{\"path\":\"a.go\"}"}}`,
			``,
			`event: response.output_item.done`,
			`data: {"type":"response.output_item.done","item":{"id":"fc-2","type":"function_call","call_id":"call-2","name":"web_search","arguments":"{\"query\":\"Go\"}"}}`,
			``,
			`event: response.completed`,
			`data: {"type":"response.completed","response":{"id":"resp-1","usage":{"input_tokens":100,"input_tokens_details":{"cached_tokens":40},"output_tokens":20,"output_tokens_details":{"reasoning_tokens":7},"total_tokens":120}}}`,
			``,
		}, "\n"))
	}))
	defer server.Close()

	p, err := New(provider.Config{
		Name:    "openai-native",
		BaseURL: server.URL + "/v1",
		Model:   "gpt-test",
		APIKey:  "secret",
		Extra: map[string]any{
			"api_mode":           "responses",
			"reasoning_protocol": "openai",
			"effort":             "high",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := p.Stream(context.Background(), provider.Request{Messages: []provider.Message{{Role: provider.RoleUser, Content: "go"}}})
	if err != nil {
		t.Fatal(err)
	}
	var chunks []provider.Chunk
	for chunk := range stream {
		chunks = append(chunks, chunk)
	}
	if captured["model"] != "gpt-test" || captured["stream"] != true {
		t.Fatalf("captured request = %+v", captured)
	}
	var text, reasoning string
	var reasoningBlocks []provider.ReasoningBlock
	var starts, calls []provider.ToolCall
	var usage *provider.Usage
	var done bool
	for _, chunk := range chunks {
		switch chunk.Type {
		case provider.ChunkText:
			text += chunk.Text
		case provider.ChunkReasoning:
			reasoning += chunk.Text
			if chunk.ReasoningBlock != nil {
				reasoningBlocks = append(reasoningBlocks, *chunk.ReasoningBlock)
			}
		case provider.ChunkToolCallStart:
			starts = append(starts, *chunk.ToolCall)
		case provider.ChunkToolCall:
			calls = append(calls, *chunk.ToolCall)
		case provider.ChunkUsage:
			usage = chunk.Usage
		case provider.ChunkDone:
			done = true
		case provider.ChunkError:
			t.Fatalf("unexpected stream error: %v", chunk.Err)
		}
	}
	if text != "hello" || reasoning != "think " || !done {
		t.Fatalf("text=%q reasoning=%q done=%v chunks=%+v", text, reasoning, done, chunks)
	}
	if len(reasoningBlocks) != 1 || reasoningBlocks[0].Type != "openai_reasoning" || reasoningBlocks[0].Text != "think " || reasoningBlocks[0].Data != "opaque-rs" {
		t.Fatalf("reasoning blocks = %+v", reasoningBlocks)
	}
	if len(starts) != 2 || len(calls) != 2 || calls[0].ID != "call-1" || calls[1].Name != "web_search" {
		t.Fatalf("tool starts=%+v calls=%+v", starts, calls)
	}
	if usage == nil || usage.PromptTokens != 100 || usage.CacheHitTokens != 40 || usage.CacheMissTokens != 60 || usage.ReasoningTokens != 7 || usage.FinishReason != "tool_calls" {
		t.Fatalf("usage = %+v", usage)
	}
}

func TestResponsesReasoningFallbackAndCompletedOutputDoNotDuplicate(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.output_item.added","item":{"id":"rs-1","type":"reasoning","summary":[]}}`,
		``,
		`data: {"type":"response.reasoning_text.delta","delta":"raw thought"}`,
		``,
		`data: {"type":"response.output_item.done","item":{"id":"rs-1","type":"reasoning","summary":[],"content":[{"type":"reasoning_text","text":"raw thought"}],"encrypted_content":"opaque-rs"}}`,
		``,
		`data: {"type":"response.completed","response":{"id":"resp-1","output":[{"id":"rs-1","type":"reasoning","summary":[],"content":[{"type":"reasoning_text","text":"raw thought"}],"encrypted_content":"opaque-rs"}]}}`,
		``,
	}, "\n")
	c := &client{name: "openai-native", apiMode: apiModeResponses}
	out := make(chan provider.Chunk, 8)
	emitted, err := c.readResponsesStream(context.Background(), &http.Response{Body: io.NopCloser(strings.NewReader(body))}, out)
	if err != nil || !emitted {
		t.Fatalf("emitted=%v err=%v", emitted, err)
	}
	close(out)
	var reasoning string
	var blocks []provider.ReasoningBlock
	for chunk := range out {
		if chunk.Type != provider.ChunkReasoning {
			continue
		}
		reasoning += chunk.Text
		if chunk.ReasoningBlock != nil {
			blocks = append(blocks, *chunk.ReasoningBlock)
		}
	}
	if reasoning != "raw thought" {
		t.Fatalf("reasoning = %q, want one fallback copy", reasoning)
	}
	if len(blocks) != 1 || blocks[0].Data != "opaque-rs" {
		t.Fatalf("reasoning blocks = %+v, want one opaque block", blocks)
	}
}

func TestResponsesMultipleReasoningItemsPreserveOrder(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.output_item.added","item":{"id":"rs-1","type":"reasoning","summary":[]}}`,
		``,
		`data: {"type":"response.reasoning_summary_text.delta","item_id":"rs-1","delta":"first"}`,
		``,
		`data: {"type":"response.output_item.done","item":{"id":"rs-1","type":"reasoning","summary":[{"type":"summary_text","text":"first"}],"encrypted_content":"opaque-1"}}`,
		``,
		`data: {"type":"response.output_item.added","item":{"id":"rs-2","type":"reasoning","summary":[]}}`,
		``,
		`data: {"type":"response.reasoning_summary_text.delta","item_id":"rs-2","delta":"second"}`,
		``,
		`data: {"type":"response.output_item.done","item":{"id":"rs-2","type":"reasoning","summary":[{"type":"summary_text","text":"second"}],"encrypted_content":"opaque-2"}}`,
		``,
		`data: {"type":"response.completed","response":{"id":"resp-1","output":[{"id":"rs-1","type":"reasoning","summary":[{"type":"summary_text","text":"first"}],"encrypted_content":"opaque-1"},{"id":"rs-2","type":"reasoning","summary":[{"type":"summary_text","text":"second"}],"encrypted_content":"opaque-2"}]}}`,
		``,
	}, "\n")
	c := &client{name: "openai-native", apiMode: apiModeResponses}
	out := make(chan provider.Chunk, 16)
	emitted, err := c.readResponsesStream(context.Background(), &http.Response{Body: io.NopCloser(strings.NewReader(body))}, out)
	if err != nil || !emitted {
		t.Fatalf("emitted=%v err=%v", emitted, err)
	}
	close(out)
	var reasoning string
	var blocks []provider.ReasoningBlock
	for chunk := range out {
		if chunk.Type != provider.ChunkReasoning {
			continue
		}
		reasoning += chunk.Text
		if chunk.ReasoningBlock != nil {
			blocks = append(blocks, *chunk.ReasoningBlock)
		}
	}
	if reasoning != "firstsecond" {
		t.Fatalf("reasoning = %q", reasoning)
	}
	if len(blocks) != 2 || blocks[0].Data != "opaque-1" || blocks[1].Data != "opaque-2" {
		t.Fatalf("reasoning block order = %+v", blocks)
	}
}

func TestResponsesMultipleMessageItemsPreserveFinalText(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.output_item.done","item":{"id":"msg-1","type":"message","role":"assistant","content":[{"type":"output_text","text":"first"}]}}`,
		``,
		`data: {"type":"response.output_item.done","item":{"id":"msg-2","type":"message","role":"assistant","content":[{"type":"output_text","text":"second"}]}}`,
		``,
		`data: {"type":"response.completed","response":{"id":"resp-1","output":[{"id":"msg-1","type":"message","role":"assistant","content":[{"type":"output_text","text":"first"}]},{"id":"msg-2","type":"message","role":"assistant","content":[{"type":"output_text","text":"second"}]}]}}`,
		``,
	}, "\n")
	c := &client{name: "openai-native", apiMode: apiModeResponses}
	out := make(chan provider.Chunk, 16)
	emitted, err := c.readResponsesStream(context.Background(), &http.Response{Body: io.NopCloser(strings.NewReader(body))}, out)
	if err != nil || !emitted {
		t.Fatalf("emitted=%v err=%v", emitted, err)
	}
	close(out)
	var text string
	for chunk := range out {
		if chunk.Type == provider.ChunkText {
			text += chunk.Text
		}
	}
	if text != "firstsecond" {
		t.Fatalf("text = %q, want both message items exactly once", text)
	}
}

func TestResponsesStreamFailedAndIncompleteAreTyped(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		code       string
		incomplete string
	}{
		{
			name: "failed",
			body: `data: {"type":"response.failed","response":{"error":{"code":"context_length_exceeded","message":"too long"}}}` + "\n\n",
			code: "context_length_exceeded",
		},
		{
			name:       "incomplete",
			body:       `data: {"type":"response.incomplete","response":{"incomplete_details":{"reason":"max_output_tokens"}}}` + "\n\n",
			incomplete: "max_output_tokens",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &client{name: "openai-native", apiMode: apiModeResponses}
			resp := &http.Response{Body: io.NopCloser(strings.NewReader(tt.body))}
			_, err := c.readResponsesStream(context.Background(), resp, make(chan provider.Chunk, 8))
			var responseErr *ResponsesError
			if !errors.As(err, &responseErr) {
				t.Fatalf("error = %T %v, want *ResponsesError", err, err)
			}
			if responseErr.Code != tt.code || responseErr.IncompleteReason != tt.incomplete {
				t.Fatalf("ResponsesError = %+v", responseErr)
			}
		})
	}
}

func TestResponsesStreamCleanEOFIsNotCompletion(t *testing.T) {
	c := &client{name: "openai-native", apiMode: apiModeResponses}
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(`data: {"type":"response.output_text.delta","delta":"partial"}` + "\n\n"))}
	emitted, err := c.readResponsesStream(context.Background(), resp, make(chan provider.Chunk, 8))
	if !emitted || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("emitted=%v err=%v, want unexpected EOF after output", emitted, err)
	}
}

func TestNewResponsesModeIsExplicitAndValidatesEffort(t *testing.T) {
	base := provider.Config{Name: "openai-native", BaseURL: "https://api.openai.com/v1", Model: "gpt-test", Extra: map[string]any{"api_mode": "responses"}}
	if _, err := New(base); err != nil {
		t.Fatalf("responses mode: %v", err)
	}
	base.Extra["api_mode"] = "unknown"
	if _, err := New(base); err == nil || !strings.Contains(err.Error(), "api_mode") {
		t.Fatalf("invalid api_mode error = %v", err)
	}
	base.Extra["api_mode"] = "responses"
	base.Extra["effort"] = "ultra"
	p, err := New(base)
	if err != nil {
		t.Fatal(err)
	}
	if got := p.(*client).effort; got != "max" {
		t.Fatalf("ultra wire effort = %q, want max", got)
	}
	base.Extra["effort"] = "adaptive"
	if _, err := New(base); err == nil || !strings.Contains(err.Error(), "Responses API") {
		t.Fatalf("invalid Responses effort error = %v", err)
	}
}
