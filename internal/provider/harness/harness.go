// Package harness provides a deterministic, localhost-only OpenAI-compatible SSE
// server for testing provider error paths (401, 429, 503, stream interruption)
// without real API keys. The server is scriptable: each Step defines the HTTP
// status code, SSE payload chunks, optional delays, and mid-stream disconnect
// behaviour.
//
// Typical use:
//
//	srv := harness.MustNew(harness.Script{
//	    {Status: 200, Chunks: []harness.Chunk{{Text: "Hello"}}},
//	    {Status: 401, Body: `{"error":{"message":"Invalid API Key"}}`},
//	})
//	defer srv.Close()
//	prov := openai.New(provider.Config{BaseURL: srv.URL(), APIKey: "sk-test"})
package harness

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"
)

const maxRequestBodyBytes = 1 << 20

// Chunk represents one SSE data payload to emit.
type Chunk struct {
	Text      string `json:"text,omitempty"`      // content delta
	Reasoning string `json:"reasoning,omitempty"` // reasoning delta
	Usage     *Usage `json:"usage,omitempty"`     // final usage
	Done      bool   `json:"done,omitempty"`      // [DONE] marker
	// DisconnectAfter causes the server to close the connection without
	// sending further chunks after this one, simulating a mid-stream
	// network break.
	DisconnectAfter bool `json:"disconnect_after,omitempty"`
}

// Usage mirrors the provider usage stats for the harness.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Step is one scripted response. The server replays steps in order;
// calls beyond the script length repeat the last step.
type Step struct {
	// Status is the HTTP status code (200, 401, 429, 503, etc.).
	Status int `json:"status"`
	// Body is the raw response body for error responses (non-200).
	// For 200, the body is built from Chunks as SSE data frames.
	Body string `json:"body,omitempty"`
	// Chunks is the SSE payload for a 200 response.
	Chunks []Chunk `json:"chunks,omitempty"`
	// DelayBefore is an optional delay before sending the response,
	// useful for simulating slow endpoints or triggering timeouts.
	DelayBefore time.Duration `json:"delay_before,omitempty"`
	// DelayBetween is an optional delay between each SSE chunk,
	// useful for verifying that streaming works incrementally.
	DelayBetween time.Duration `json:"delay_between,omitempty"`
}

// Script is an ordered list of response steps.
type Script []Step

// Server is a localhost test server that speaks the OpenAI chat completions
// SSE protocol. It implements http.Handler.
type Server struct {
	mu      sync.Mutex
	script  Script
	seen    int
	reqs    []RequestRecord
	srv     *httptest.Server
	baseURL string
}

// RequestRecord captures one incoming request for later inspection.
type RequestRecord struct {
	Body   []byte `json:"body"`
	Auth   string `json:"auth"`
	Stream bool   `json:"stream"`
}

// New creates and starts a harness server. Empty scripts are rejected because
// there is no deterministic response to replay.
func New(script Script) (*Server, error) {
	if err := validateScript(script); err != nil {
		return nil, err
	}
	s := &Server{script: script}
	s.srv = httptest.NewServer(s)
	s.baseURL = s.srv.URL
	return s, nil
}

// MustNew is a test convenience that panics when the script is invalid.
func MustNew(script Script) *Server {
	s, err := New(script)
	if err != nil {
		panic(err)
	}
	return s
}

// URL returns the base URL of the test server.
func (s *Server) URL() string { return s.baseURL }

// Close shuts down the test server.
func (s *Server) Close() { s.srv.Close() }

// Requests returns all recorded incoming requests.
func (s *Server) Requests() []RequestRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]RequestRecord, len(s.reqs))
	for i, req := range s.reqs {
		out[i] = req
		out[i].Body = append([]byte(nil), req.Body...)
	}
	return out
}

// Reset restarts the script and clears recorded requests.
func (s *Server) Reset(script Script) error {
	if err := validateScript(script); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.script = script
	s.seen = 0
	s.reqs = nil
	return nil
}

func validateScript(script Script) error {
	if len(script) == 0 {
		return errors.New("provider harness: script must contain at least one step")
	}
	for i, step := range script {
		if step.Status < 100 || step.Status > 599 {
			return fmt.Errorf("provider harness: step %d has invalid HTTP status %d", i+1, step.Status)
		}
	}
	return nil
}

// ServeHTTP implements http.Handler for the OpenAI chat completions endpoint.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBodyBytes+1))
	if err != nil {
		http.Error(w, "could not read request body", http.StatusBadRequest)
		return
	}
	if len(body) > maxRequestBodyBytes {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}

	rec := RequestRecord{
		Body: body,
		Auth: r.Header.Get("Authorization"),
		Stream: strings.Contains(r.URL.Query().Get("stream"), "true") ||
			strings.Contains(r.URL.Path, "stream"),
	}
	var payload struct {
		Stream bool `json:"stream"`
	}
	if json.Unmarshal(body, &payload) == nil {
		rec.Stream = rec.Stream || payload.Stream
	}

	s.mu.Lock()
	stepIdx := s.seen
	if stepIdx < len(s.script) {
		s.seen++
	}
	s.reqs = append(s.reqs, rec)
	step := s.script[min(stepIdx, len(s.script)-1)]
	s.mu.Unlock()

	if step.DelayBefore > 0 {
		select {
		case <-time.After(step.DelayBefore):
		case <-r.Context().Done():
			return
		}
	}

	if step.Status != 200 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(step.Status)
		if step.Body != "" {
			fmt.Fprint(w, step.Body)
		}
		return
	}

	// SSE streaming response.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(200)
	flusher, canFlush := w.(http.Flusher)

	for _, ch := range step.Chunks {
		payload := buildSSEChunk(ch)
		fmt.Fprint(w, payload)
		if canFlush {
			flusher.Flush()
		}
		if ch.DisconnectAfter {
			// Simulate mid-stream disconnect by stopping here.
			// The hijack-able connection just stops sending.
			if hijacker, ok := w.(http.Hijacker); ok {
				conn, _, _ := hijacker.Hijack()
				if conn != nil {
					conn.Close()
				}
			}
			return
		}
		if step.DelayBetween > 0 {
			select {
			case <-time.After(step.DelayBetween):
			case <-r.Context().Done():
				return
			}
		}
	}

	// Send [DONE] marker.
	fmt.Fprint(w, "data: [DONE]\n\n")
	if canFlush {
		flusher.Flush()
	}
}

// buildSSEChunk constructs an OpenAI-compatible SSE data frame.
func buildSSEChunk(ch Chunk) string {
	if ch.Done {
		return "data: [DONE]\n\n"
	}
	obj := map[string]interface{}{
		"id":      "harness-1",
		"object":  "chat.completion.chunk",
		"created": 0,
		"model":   "harness",
		"choices": []map[string]interface{}{{
			"index": 0,
			"delta": map[string]interface{}{},
		}},
	}
	delta := obj["choices"].([]map[string]interface{})[0]["delta"].(map[string]interface{})
	if ch.Text != "" {
		delta["content"] = ch.Text
	}
	if ch.Reasoning != "" {
		delta["reasoning_content"] = ch.Reasoning
	}
	if ch.Usage != nil {
		obj["usage"] = map[string]interface{}{
			"prompt_tokens":     ch.Usage.PromptTokens,
			"completion_tokens": ch.Usage.CompletionTokens,
			"total_tokens":      ch.Usage.TotalTokens,
		}
	}
	b, _ := json.Marshal(obj)
	return fmt.Sprintf("data: %s\n\n", string(b))
}

// --- Convenience constructors for common error scenarios ---

// AuthError401 returns a script step simulating a 401 authentication error.
func AuthError401(keyEnv string) Step {
	return Step{
		Status: 401,
		Body:   fmt.Sprintf(`{"error":{"message":"Incorrect API key provided: %s","type":"invalid_request_error","code":"invalid_api_key"}}`, keyEnv),
	}
}

// RateLimit429 returns a script step simulating a 429 rate limit error.
func RateLimit429() Step {
	return Step{
		Status: 429,
		Body:   `{"error":{"message":"Rate limit reached. Please try again later.","type":"rate_limit_error"}}`,
	}
}

// ServerError503 returns a script step simulating a 503 server error.
func ServerError503() Step {
	return Step{
		Status: 503,
		Body:   `{"error":{"message":"The server is currently overloaded.","type":"server_error"}}`,
	}
}

// TextChunk returns a simple text response step.
func TextChunk(text string) Step {
	return Step{
		Status: 200,
		Chunks: []Chunk{
			{Text: text},
		},
	}
}

// StreamDisconnect returns a step that sends one text chunk then disconnects.
func StreamDisconnect(text string) Step {
	return Step{
		Status: 200,
		Chunks: []Chunk{
			{Text: text, DisconnectAfter: true},
		},
	}
}
