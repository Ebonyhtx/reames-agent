package openai

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"reames-agent/internal/provider"
)

// ResponsesError is an error reported inside a successful Responses API SSE
// connection. HTTP status failures continue to use provider.APIError; this type
// preserves the provider-issued code or incomplete reason for stream failures.
type ResponsesError struct {
	Code             string
	Message          string
	IncompleteReason string
}

func (e *ResponsesError) Error() string {
	if e == nil {
		return "openai responses: unknown stream error"
	}
	if e.IncompleteReason != "" {
		return fmt.Sprintf("openai responses: incomplete response: %s", e.IncompleteReason)
	}
	if e.Code != "" && e.Message != "" {
		return fmt.Sprintf("openai responses: %s: %s", e.Code, e.Message)
	}
	if e.Message != "" {
		return "openai responses: " + e.Message
	}
	if e.Code != "" {
		return "openai responses: " + e.Code
	}
	return "openai responses: unknown stream error"
}

func (c *client) buildResponsesRequest(req provider.Request) responsesRequest {
	messages := provider.SanitizeToolPairing(req.Messages)
	input := make([]responsesInputItem, 0, len(messages))
	var instructions []string
	for _, message := range messages {
		switch message.Role {
		case provider.RoleSystem:
			if text := strings.TrimSpace(message.Content); text != "" {
				instructions = append(instructions, text)
			}
		case provider.RoleUser:
			content := make([]responsesContentPart, 0, len(message.Images)+1)
			if message.Content != "" {
				content = append(content, responsesContentPart{Type: "input_text", Text: message.Content})
			}
			if c.vision {
				for _, image := range message.Images {
					content = append(content, responsesContentPart{Type: "input_image", ImageURL: image, Detail: c.visionDetail})
				}
			}
			if len(content) > 0 {
				input = append(input, responsesInputItem{Type: "message", Role: "user", Content: content})
			}
		case provider.RoleAssistant:
			for _, block := range message.ReasoningBlocks {
				if block.Type != "openai_reasoning" || block.Data == "" {
					continue
				}
				var summary []responsesContentPart
				if block.Text != "" {
					summary = []responsesContentPart{{Type: "summary_text", Text: block.Text}}
				}
				input = append(input, responsesInputItem{
					Type:             "reasoning",
					Summary:          summary,
					EncryptedContent: block.Data,
				})
			}
			if message.Content != "" {
				input = append(input, responsesInputItem{
					Type:    "message",
					Role:    "assistant",
					Content: []responsesContentPart{{Type: "output_text", Text: message.Content}},
				})
			}
			for _, call := range message.ToolCalls {
				input = append(input, responsesInputItem{
					Type:      "function_call",
					CallID:    call.ID,
					Name:      call.Name,
					Arguments: call.Arguments,
				})
			}
		case provider.RoleTool:
			if message.ToolCallID != "" {
				input = append(input, responsesInputItem{
					Type:   "function_call_output",
					CallID: message.ToolCallID,
					Output: message.Content,
				})
			}
		}
	}

	tools := make([]responsesTool, 0, len(req.Tools))
	for _, tool := range req.Tools {
		parameters := tool.Parameters
		if len(parameters) == 0 {
			parameters = provider.CanonicalizeSchema(nil)
		}
		tools = append(tools, responsesTool{
			Type:        "function",
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  parameters,
			Strict:      false,
		})
	}

	var reasoning *responsesReasoning
	if c.openaiReasoning || c.effort != "" {
		reasoning = &responsesReasoning{Effort: c.effort, Summary: "auto"}
	}

	return responsesRequest{
		Model:             c.model,
		Instructions:      strings.Join(instructions, "\n\n"),
		Input:             input,
		Tools:             tools,
		ToolChoice:        "auto",
		ParallelToolCalls: len(tools) > 0,
		Reasoning:         reasoning,
		Include:           []string{"reasoning.encrypted_content"},
		Store:             false,
		Stream:            true,
		MaxOutputTokens:   req.MaxTokens,
		ExtraBody:         c.extraBody,
	}
}

type responsesRequest struct {
	Model             string               `json:"model"`
	Instructions      string               `json:"instructions,omitempty"`
	Input             []responsesInputItem `json:"input"`
	Tools             []responsesTool      `json:"tools,omitempty"`
	ToolChoice        string               `json:"tool_choice"`
	ParallelToolCalls bool                 `json:"parallel_tool_calls"`
	Reasoning         *responsesReasoning  `json:"reasoning,omitempty"`
	Include           []string             `json:"include,omitempty"`
	Store             bool                 `json:"store"`
	Stream            bool                 `json:"stream"`
	MaxOutputTokens   int                  `json:"max_output_tokens,omitempty"`
	ExtraBody         map[string]any       `json:"-"`
}

func (r responsesRequest) MarshalJSON() ([]byte, error) {
	type wire responsesRequest
	base := wire(r)
	base.ExtraBody = nil
	raw, err := json.Marshal(base)
	if err != nil || len(r.ExtraBody) == 0 {
		return raw, err
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, err
	}
	for key, value := range cleanExtraBody(r.ExtraBody) {
		body[key] = value
	}
	return json.Marshal(body)
}

type responsesInputItem struct {
	Type             string                 `json:"type"`
	Role             string                 `json:"role,omitempty"`
	Content          []responsesContentPart `json:"content,omitempty"`
	Summary          []responsesContentPart `json:"summary,omitempty"`
	CallID           string                 `json:"call_id,omitempty"`
	Name             string                 `json:"name,omitempty"`
	Arguments        string                 `json:"arguments,omitempty"`
	Output           string                 `json:"output,omitempty"`
	EncryptedContent string                 `json:"encrypted_content,omitempty"`
}

type responsesContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

type responsesTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
	Strict      bool            `json:"strict"`
}

type responsesReasoning struct {
	Effort  string `json:"effort,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type responsesWireError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type responsesCompleted struct {
	ID                string                `json:"id"`
	Output            []responsesOutputItem `json:"output"`
	Usage             *responsesUsage       `json:"usage"`
	Error             *responsesWireError   `json:"error"`
	IncompleteDetails *struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
}

type responsesUsage struct {
	InputTokens        int `json:"input_tokens"`
	OutputTokens       int `json:"output_tokens"`
	TotalTokens        int `json:"total_tokens"`
	InputTokensDetails *struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"input_tokens_details"`
	OutputTokensDetails *struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
}

type responsesOutputItem struct {
	ID               string                 `json:"id"`
	Type             string                 `json:"type"`
	Role             string                 `json:"role"`
	Content          []responsesContentPart `json:"content"`
	Summary          []responsesContentPart `json:"summary"`
	CallID           string                 `json:"call_id"`
	Name             string                 `json:"name"`
	Arguments        string                 `json:"arguments"`
	Input            string                 `json:"input"`
	EncryptedContent string                 `json:"encrypted_content"`
}

type responsesStreamEvent struct {
	Type     string               `json:"type"`
	Delta    string               `json:"delta"`
	Text     string               `json:"text"`
	ItemID   string               `json:"item_id"`
	CallID   string               `json:"call_id"`
	Item     *responsesOutputItem `json:"item"`
	Response *responsesCompleted  `json:"response"`
	Error    *responsesWireError  `json:"error"`
}

type responsesToolState struct {
	call    provider.ToolCall
	started bool
	done    bool
}

type responsesReasoningState struct {
	summary        strings.Builder
	raw            strings.Builder
	encrypted      string
	displayEmitted bool
	blockEmitted   bool
}

func (c *client) readResponsesStream(ctx context.Context, resp *http.Response, out chan<- provider.Chunk) (emitted bool, _ error) {
	defer resp.Body.Close()

	idleTimeout := c.idleTimeout
	if idleTimeout <= 0 {
		idleTimeout = defaultStreamIdleTimeout
	}
	done := make(chan struct{})
	defer close(done)
	activity := make(chan struct{}, 1)
	var stalled atomic.Bool
	go watchStreamActivity(ctx, resp, idleTimeout, activity, done, &stalled)

	states := map[string]*responsesToolState{}
	var order []string
	reasoningStates := map[string]*responsesReasoningState{}
	var reasoningOrder []string
	var activeReasoningKey string
	textItems := map[string]bool{}
	var anonymousTextStreamed bool
	var sawCompleted bool

	ensureState := func(key string) *responsesToolState {
		if key == "" {
			key = fmt.Sprintf("item_%d", len(order))
		}
		if state, ok := states[key]; ok {
			return state
		}
		state := &responsesToolState{}
		states[key] = state
		order = append(order, key)
		return state
	}
	ensureReasoningState := func(key string) (string, *responsesReasoningState) {
		if key == "" {
			key = activeReasoningKey
		}
		if key == "" {
			key = fmt.Sprintf("reasoning_%d", len(reasoningOrder))
		}
		if state, ok := reasoningStates[key]; ok {
			return key, state
		}
		state := &responsesReasoningState{}
		reasoningStates[key] = state
		reasoningOrder = append(reasoningOrder, key)
		return key, state
	}
	emitReasoningDone := func(state *responsesReasoningState) error {
		if state == nil {
			return nil
		}
		display := state.summary.String()
		if display == "" {
			display = state.raw.String()
		}
		if !state.displayEmitted && display != "" {
			state.displayEmitted = true
			emitted = true
			if !sendChunk(ctx, out, provider.Chunk{Type: provider.ChunkReasoning, Text: display}) {
				return ctx.Err()
			}
		}
		if !state.blockEmitted && state.encrypted != "" {
			state.blockEmitted = true
			emitted = true
			block := &provider.ReasoningBlock{Type: "openai_reasoning", Text: display, Data: state.encrypted}
			if !sendChunk(ctx, out, provider.Chunk{Type: provider.ChunkReasoning, ReasoningBlock: block}) {
				return ctx.Err()
			}
		}
		return nil
	}
	emitToolStart := func(state *responsesToolState) error {
		if state.started || state.call.Name == "" {
			return nil
		}
		state.started = true
		emitted = true
		if !sendChunk(ctx, out, provider.Chunk{Type: provider.ChunkToolCallStart, ToolCall: &provider.ToolCall{ID: state.call.ID, Name: state.call.Name}}) {
			return ctx.Err()
		}
		return nil
	}
	emitToolDone := func(state *responsesToolState) error {
		if state.done || state.call.Name == "" {
			return nil
		}
		if state.call.ID == "" {
			state.call.ID = fmt.Sprintf("call_%d", len(order))
		}
		if err := emitToolStart(state); err != nil {
			return err
		}
		state.done = true
		emitted = true
		call := state.call
		if !sendChunk(ctx, out, provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &call}) {
			return ctx.Err()
		}
		return nil
	}
	handleOutputItem := func(item responsesOutputItem, final bool) error {
		switch item.Type {
		case "function_call", "custom_tool_call":
			key := item.ID
			if key == "" {
				key = item.CallID
			}
			state := ensureState(key)
			if item.CallID != "" {
				state.call.ID = item.CallID
			}
			if item.Name != "" {
				state.call.Name = item.Name
			}
			if item.Arguments != "" {
				state.call.Arguments = item.Arguments
			} else if item.Input != "" {
				state.call.Arguments = item.Input
			}
			if err := emitToolStart(state); err != nil {
				return err
			}
			if final {
				return emitToolDone(state)
			}
		case "message":
			alreadyEmitted := item.ID != "" && textItems[item.ID]
			if item.ID == "" {
				alreadyEmitted = anonymousTextStreamed
			}
			if final && !alreadyEmitted {
				for _, part := range item.Content {
					if part.Type == "output_text" && part.Text != "" {
						if item.ID != "" {
							textItems[item.ID] = true
						} else {
							anonymousTextStreamed = true
						}
						emitted = true
						if !sendChunk(ctx, out, provider.Chunk{Type: provider.ChunkText, Text: part.Text}) {
							return ctx.Err()
						}
					}
				}
			}
		case "reasoning":
			reasoningKey := item.ID
			if reasoningKey != "" {
				if _, ok := reasoningStates[reasoningKey]; !ok && activeReasoningKey == "" && len(reasoningOrder) > 0 {
					candidate := reasoningOrder[len(reasoningOrder)-1]
					candidateState := reasoningStates[candidate]
					if strings.HasPrefix(candidate, "reasoning_") && candidateState != nil && !candidateState.blockEmitted {
						reasoningKey = candidate
					}
				}
			}
			key, state := ensureReasoningState(reasoningKey)
			activeReasoningKey = key
			if item.EncryptedContent != "" {
				state.encrypted = item.EncryptedContent
			}
			if state.summary.Len() == 0 {
				for _, part := range item.Summary {
					state.summary.WriteString(part.Text)
				}
			}
			if state.raw.Len() == 0 {
				for _, part := range item.Content {
					state.raw.WriteString(part.Text)
				}
			}
			if final {
				if err := emitReasoningDone(state); err != nil {
					return err
				}
				if activeReasoningKey == key {
					activeReasoningKey = ""
				}
			}
		}
		return nil
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		select {
		case activity <- struct{}{}:
		default:
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			break
		}
		var event responsesStreamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return emitted, fmt.Errorf("%s: decode Responses stream: %w", c.name, err)
		}
		switch event.Type {
		case "response.output_text.delta":
			if event.Delta != "" {
				if event.ItemID != "" {
					textItems[event.ItemID] = true
				} else {
					// Some compatible endpoints omit item_id from text deltas. In
					// that ambiguous case the final item cannot be safely replayed
					// without duplicating the visible stream.
					anonymousTextStreamed = true
				}
				emitted = true
				if !sendChunk(ctx, out, provider.Chunk{Type: provider.ChunkText, Text: event.Delta}) {
					return emitted, ctx.Err()
				}
			}
		case "response.reasoning_summary_text.delta":
			if event.Delta != "" {
				_, state := ensureReasoningState(event.ItemID)
				state.summary.WriteString(event.Delta)
				state.displayEmitted = true
				emitted = true
				if !sendChunk(ctx, out, provider.Chunk{Type: provider.ChunkReasoning, Text: event.Delta}) {
					return emitted, ctx.Err()
				}
			}
		case "response.reasoning_summary_text.done":
			if event.Text != "" {
				_, state := ensureReasoningState(event.ItemID)
				if state.summary.Len() == 0 {
					state.summary.WriteString(event.Text)
				}
				if err := emitReasoningDone(state); err != nil {
					return emitted, err
				}
			}
		case "response.reasoning_text.delta":
			if event.Delta != "" {
				_, state := ensureReasoningState(event.ItemID)
				state.raw.WriteString(event.Delta)
			}
		case "response.output_item.added":
			if event.Item != nil {
				if event.Item.Type == "reasoning" {
					key, _ := ensureReasoningState(event.Item.ID)
					activeReasoningKey = key
				}
				if err := handleOutputItem(*event.Item, false); err != nil {
					return emitted, err
				}
			}
		case "response.function_call_arguments.delta", "response.custom_tool_call_input.delta":
			key := event.ItemID
			if key == "" {
				key = event.CallID
			}
			state := ensureState(key)
			if event.CallID != "" {
				state.call.ID = event.CallID
			}
			state.call.Arguments += event.Delta
		case "response.output_item.done":
			if event.Item != nil {
				if err := handleOutputItem(*event.Item, true); err != nil {
					return emitted, err
				}
			}
		case "response.failed", "error", "response.error":
			wireErr := event.Error
			if event.Response != nil && event.Response.Error != nil {
				wireErr = event.Response.Error
			}
			if wireErr == nil {
				return emitted, &ResponsesError{Message: "response.failed event received"}
			}
			return emitted, &ResponsesError{Code: wireErr.Code, Message: wireErr.Message}
		case "response.incomplete":
			reason := "unknown"
			if event.Response != nil && event.Response.IncompleteDetails != nil && event.Response.IncompleteDetails.Reason != "" {
				reason = event.Response.IncompleteDetails.Reason
			}
			return emitted, &ResponsesError{IncompleteReason: reason}
		case "response.completed":
			if event.Response == nil {
				return emitted, &ResponsesError{Message: "response.completed missing response body"}
			}
			for _, item := range event.Response.Output {
				if err := handleOutputItem(item, true); err != nil {
					return emitted, err
				}
			}
			for _, key := range reasoningOrder {
				if err := emitReasoningDone(reasoningStates[key]); err != nil {
					return emitted, err
				}
			}
			for _, key := range order {
				if err := emitToolDone(states[key]); err != nil {
					return emitted, err
				}
			}
			if event.Response.Usage != nil {
				emitted = true
				usage := normaliseResponsesUsage(event.Response.Usage, len(order) > 0)
				if !sendChunk(ctx, out, provider.Chunk{Type: provider.ChunkUsage, Usage: usage}) {
					return emitted, ctx.Err()
				}
			}
			if !sendChunk(ctx, out, provider.Chunk{Type: provider.ChunkDone}) {
				return emitted, ctx.Err()
			}
			sawCompleted = true
		}
		if sawCompleted {
			break
		}
	}

	if err := ctx.Err(); err != nil {
		return emitted, err
	}
	if stalled.Load() {
		return emitted, fmt.Errorf("%s: Responses stream stalled: no data for %s, connection likely dropped", c.name, idleTimeout)
	}
	if err := scanner.Err(); err != nil {
		return emitted, fmt.Errorf("%s: read Responses stream: %w", c.name, err)
	}
	if !sawCompleted {
		return emitted, fmt.Errorf("%s: Responses stream ended before response.completed: %w", c.name, io.ErrUnexpectedEOF)
	}
	return emitted, nil
}

func watchStreamActivity(ctx context.Context, resp *http.Response, idleTimeout time.Duration, activity <-chan struct{}, done <-chan struct{}, stalled *atomic.Bool) {
	idle := time.NewTimer(idleTimeout)
	defer idle.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = resp.Body.Close()
			return
		case <-idle.C:
			stalled.Store(true)
			_ = resp.Body.Close()
			return
		case <-activity:
			if !idle.Stop() {
				select {
				case <-idle.C:
				default:
				}
			}
			idle.Reset(idleTimeout)
		case <-done:
			return
		}
	}
}

func normaliseResponsesUsage(u *responsesUsage, toolCalls bool) *provider.Usage {
	cached := 0
	if u.InputTokensDetails != nil {
		cached = u.InputTokensDetails.CachedTokens
	}
	reasoning := 0
	if u.OutputTokensDetails != nil {
		reasoning = u.OutputTokensDetails.ReasoningTokens
	}
	finish := "stop"
	if toolCalls {
		finish = "tool_calls"
	}
	return &provider.Usage{
		PromptTokens:     u.InputTokens,
		CompletionTokens: u.OutputTokens,
		TotalTokens:      u.TotalTokens,
		CacheHitTokens:   cached,
		CacheMissTokens:  max(0, u.InputTokens-cached),
		ReasoningTokens:  reasoning,
		FinishReason:     finish,
	}
}
