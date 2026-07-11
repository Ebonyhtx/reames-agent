package serve

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"reames-agent/internal/agent"
	"reames-agent/internal/config"
	"reames-agent/internal/control"
	"reames-agent/internal/eventwire"
	"reames-agent/internal/jobs"
	"reames-agent/internal/provider"
)

// fakeRunner stands in for an agent.Runner: it records the composed input and
// returns without emitting model events, so the controller's TurnDone is the
// observable signal.
type fakeRunner struct{ got chan string }

func (f fakeRunner) Run(_ context.Context, input string) error { f.got <- input; return nil }

type runnerFunc func(context.Context, string) error

func (f runnerFunc) Run(ctx context.Context, input string) error { return f(ctx, input) }

func TestServeSubmitRunsAndBroadcastsTurnDone(t *testing.T) {
	bc := NewBroadcaster()
	got := make(chan string, 1)
	ctrl := control.New(control.Options{Runner: fakeRunner{got: got}, Sink: bc})
	srv := httptest.NewServer(New(ctrl, bc, config.ServeConfig{}).Handler())
	defer srv.Close()

	sub, cancel := bc.Subscribe() // observe the broadcast deterministically
	defer cancel()

	resp, err := http.Post(srv.URL+"/submit", "application/json", strings.NewReader(`{"input":"hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("submit status = %d, want 202", resp.StatusCode)
	}

	select {
	case in := <-got:
		if in != "hi" {
			t.Errorf("runner ran %q, want hi", in)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runner never ran")
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case data := <-sub:
			var w eventwire.Event
			if err := json.Unmarshal(data, &w); err == nil && w.Kind == "turn_done" {
				return
			}
		case <-deadline:
			t.Fatal("never saw turn_done on the stream")
		}
	}
}

func TestServeVersionedCommandEndpointSubmitCancelAndStatus(t *testing.T) {
	bc := NewBroadcaster()
	started := make(chan struct{}, 1)
	stopped := make(chan struct{}, 1)
	runner := runnerFunc(func(ctx context.Context, input string) error {
		if input != "long turn" {
			t.Fatalf("runner input = %q", input)
		}
		started <- struct{}{}
		<-ctx.Done()
		stopped <- struct{}{}
		return ctx.Err()
	})
	ctrl := control.New(control.Options{Runner: runner, Sink: bc})
	srv := httptest.NewServer(New(ctrl, bc, config.ServeConfig{}).Handler())
	defer srv.Close()

	postCommand := func(command string) (int, control.CommandResult) {
		t.Helper()
		resp, err := http.Post(srv.URL+"/command", "application/json", strings.NewReader(command))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var result control.CommandResult
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("decode command result: %v", err)
		}
		return resp.StatusCode, result
	}

	status, result := postCommand(`{"version":1,"kind":"submit","submit":{"input":"long turn"}}`)
	if status != http.StatusOK || !result.Accepted || result.Kind != control.CommandSubmit {
		t.Fatalf("submit status/result = %d / %+v", status, result)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("versioned submit did not reach runner")
	}

	status, result = postCommand(`{"version":1,"kind":"submit","submit":{"input":"must not replace active turn"}}`)
	if status != http.StatusConflict || result.Accepted || result.Error == nil || result.Error.Code != control.CommandErrBusy {
		t.Fatalf("busy submit status/result = %d / %+v", status, result)
	}

	status, result = postCommand(`{"version":1,"kind":"status"}`)
	if status != http.StatusOK || !result.Accepted || !result.Status.Running || !result.Status.Cancellable {
		t.Fatalf("running status/result = %d / %+v", status, result)
	}

	status, result = postCommand(`{"version":1,"kind":"cancel"}`)
	if status != http.StatusOK || !result.Accepted || result.Kind != control.CommandCancel {
		t.Fatalf("cancel status/result = %d / %+v", status, result)
	}
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("versioned cancel did not unwind runner")
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		_, result = postCommand(`{"version":1,"kind":"status"}`)
		if !result.Status.Running && !result.Status.Cancellable {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("controller did not return idle: %+v", result.Status)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestServeVersionedCommandRejectsInvalidAndPrivilegedPayloads(t *testing.T) {
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc})
	srv := httptest.NewServer(New(ctrl, bc, config.ServeConfig{}).Handler())
	defer srv.Close()

	tests := []struct {
		name   string
		body   string
		status int
		code   control.CommandErrorCode
	}{
		{"unknown version", `{"version":9,"kind":"status"}`, http.StatusBadRequest, control.CommandErrInvalidVersion},
		{"unknown field", `{"version":1,"kind":"status","surprise":true}`, http.StatusBadRequest, control.CommandErrInvalidPayload},
		{"trailing value", `{"version":1,"kind":"status"} {}`, http.StatusBadRequest, control.CommandErrInvalidPayload},
		{"remote metadata", `{"version":1,"kind":"submit","submit":{"input":"x","display":"spoof"}}`, http.StatusForbidden, control.CommandErrForbidden},
		{"shell shortcut", `{"version":1,"kind":"submit","submit":{"input":"!echo nope"}}`, http.StatusForbidden, control.CommandErrForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := http.Post(srv.URL+"/command", "application/json", strings.NewReader(tt.body))
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			var result control.CommandResult
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != tt.status || result.Accepted || result.Error == nil || result.Error.Code != tt.code {
				t.Fatalf("status/result = %d / %+v, want %d / %s", resp.StatusCode, result, tt.status, tt.code)
			}
		})
	}
}

func TestServeWebSocketVersionedCommandAndLegacySubmitShareRemotePolicy(t *testing.T) {
	bc := NewBroadcaster()
	got := make(chan string, 1)
	ctrl := control.New(control.Options{Runner: fakeRunner{got: got}, Sink: bc})
	srv := httptest.NewServer(New(ctrl, bc, config.ServeConfig{}).Handler())
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{
		"method": "command",
		"params": map[string]any{"version": control.CommandVersion, "kind": control.CommandStatus},
	}); err != nil {
		t.Fatal(err)
	}
	var statusResult control.CommandResult
	if err := conn.ReadJSON(&statusResult); err != nil {
		t.Fatal(err)
	}
	if !statusResult.Accepted || statusResult.Kind != control.CommandStatus {
		t.Fatalf("status result = %+v", statusResult)
	}

	// Legacy WebSocket submit used to call trusted Submit and could therefore
	// execute !shell. It now maps through the same remote policy as HTTP.
	if err := conn.WriteJSON(map[string]any{
		"method": "submit", "params": map[string]any{"input": "!echo must-not-run"},
	}); err != nil {
		t.Fatal(err)
	}
	var legacyError map[string]string
	if err := conn.ReadJSON(&legacyError); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(legacyError["error"], "unavailable over HTTP") {
		t.Fatalf("legacy shell error = %#v", legacyError)
	}
	select {
	case input := <-got:
		t.Fatalf("legacy WebSocket shell reached runner: %q", input)
	default:
	}

	if err := conn.WriteJSON(map[string]any{
		"method": "command",
		"params": map[string]any{
			"version": control.CommandVersion,
			"kind":    control.CommandSubmit,
			"submit":  map[string]any{"input": "hello over ws"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case input := <-got:
		if input != "hello over ws" {
			t.Fatalf("runner input = %q", input)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("versioned WebSocket submit did not reach runner")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		var result control.CommandResult
		if json.Unmarshal(raw, &result) == nil && result.Accepted && result.Kind == control.CommandSubmit {
			return
		}
	}
	t.Fatal("versioned WebSocket submit acknowledgement not received")
}

func TestServeEndpoints(t *testing.T) {
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc}) // no runner needed for these
	srv := httptest.NewServer(New(ctrl, bc, config.ServeConfig{}).Handler())
	defer srv.Close()

	if resp, err := http.Get(srv.URL + "/history"); err != nil || resp.StatusCode != 200 {
		t.Fatalf("history = %v / %v", resp, err)
	}

	if resp, _ := http.Get(srv.URL + "/context"); resp.StatusCode != 200 {
		t.Errorf("context status = %d", resp.StatusCode)
	}

	resp, err := http.Post(srv.URL+"/plan", "application/json", strings.NewReader(`{"on":true}`))
	if err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("plan = %v / status %d", err, resp.StatusCode)
	}
	if c := ctrl.Compose("x"); !strings.Contains(c, "Plan mode") {
		t.Error("/plan {on:true} should have enabled plan mode (Compose would prepend the marker)")
	}

	resp, err = http.Post(srv.URL+"/tool-approval-mode", "application/json", strings.NewReader(`{"mode":"auto"}`))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("tool approval mode auto status = %d, want 204", resp.StatusCode)
	}
	resp.Body.Close()
	if got := ctrl.ToolApprovalMode(); got != control.ToolApprovalAuto {
		t.Fatalf("tool approval mode = %q, want auto", got)
	}
	resp, err = http.Post(srv.URL+"/tool-approval-mode", "application/json", strings.NewReader(`{"mode":"surprise"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid tool approval mode status = %d, want 400", resp.StatusCode)
	}

	if resp, _ := http.Post(srv.URL+"/submit", "application/json", strings.NewReader(`{}`)); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("empty submit should be 400, got %d", resp.StatusCode)
	}
}

func TestServeFeedbackCollectsSanitizedLocalSummary(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REAMES_AGENT_HOME", home)
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc})
	srv := httptest.NewServer(New(ctrl, bc, config.ServeConfig{}).Handler())
	defer srv.Close()

	body := `{
		"kind":"feedback",
		"source":"gateway",
		"label":"feishu",
		"message":"user alice@example.com saw api_key=sk-secret1234567890abcdef in C:\\Users\\Alice\\repo",
		"errorMessage":"connect failed with Bearer abcdefghijklmnopqrstuvwxyz123456",
		"topFrame":"internal/bot/feishu.go:10"
	}`
	resp, err := http.Post(srv.URL+"/api/feedback", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		out, _ := io.ReadAll(resp.Body)
		t.Fatalf("feedback status = %d body=%s, want 201", resp.StatusCode, out)
	}
	var accepted map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&accepted); err != nil {
		t.Fatal(err)
	}
	if accepted["fingerprint"] == "" || accepted["id"] == "" {
		t.Fatalf("accepted feedback missing identifiers: %+v", accepted)
	}

	resp2, err := http.Get(srv.URL + "/api/feedback/summary")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("summary status = %d, want 200", resp2.StatusCode)
	}
	summaryBody, err := io.ReadAll(resp2.Body)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"alice@example.com", "sk-secret", "abcdefghijklmnopqrstuvwxyz123456", `C:\\Users\\Alice`} {
		if strings.Contains(string(summaryBody), forbidden) {
			t.Fatalf("summary leaked %q:\n%s", forbidden, summaryBody)
		}
	}
	if !strings.Contains(string(summaryBody), `"total":1`) || !strings.Contains(string(summaryBody), `"count":1`) {
		t.Fatalf("summary did not aggregate feedback:\n%s", summaryBody)
	}

	raw, err := os.ReadFile(filepath.Join(home, "feedback", "feedback.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"alice@example.com", "sk-secret", "abcdefghijklmnopqrstuvwxyz123456", `C:\\Users\\Alice`} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("ledger leaked %q:\n%s", forbidden, raw)
		}
	}

	resp3, err := http.Post(srv.URL+"/api/feedback/draft", "application/json", strings.NewReader(`{"limit":10}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusCreated {
		out, _ := io.ReadAll(resp3.Body)
		t.Fatalf("draft status = %d body=%s, want 201", resp3.StatusCode, out)
	}
	var draft struct {
		Path     string `json:"path"`
		Markdown string `json:"markdown"`
		Total    int    `json:"total"`
		Groups   int    `json:"groups"`
	}
	if err := json.NewDecoder(resp3.Body).Decode(&draft); err != nil {
		t.Fatal(err)
	}
	if draft.Path == "" || draft.Total != 1 || draft.Groups != 1 {
		t.Fatalf("draft response = %+v, want path and one group", draft)
	}
	if _, err := os.Stat(draft.Path); err != nil {
		t.Fatalf("draft file missing: %v", err)
	}
	for _, forbidden := range []string{"alice@example.com", "sk-secret", "abcdefghijklmnopqrstuvwxyz123456", `C:\\Users\\Alice`} {
		if strings.Contains(draft.Markdown, forbidden) {
			t.Fatalf("draft leaked %q:\n%s", forbidden, draft.Markdown)
		}
	}
}

func TestServeFeedbackRequiresJSONContentType(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REAMES_AGENT_HOME", home)
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc})
	srv := httptest.NewServer(New(ctrl, bc, config.ServeConfig{}).Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/feedback", "text/plain", strings.NewReader(`{"kind":"feedback","message":"hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("feedback text/plain status = %d, want 415", resp.StatusCode)
	}
}

func TestServeFeedbackRespectsTokenAuth(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REAMES_AGENT_HOME", home)
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc})
	server := New(ctrl, bc, config.ServeConfig{AuthMode: "token", Token: "secret-token"})
	srv := httptest.NewServer(server.Handler())
	defer srv.Close()

	body := `{"kind":"feedback","message":"hello"}`
	resp, err := http.Post(srv.URL+"/api/feedback", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated feedback status = %d, want 401", resp.StatusCode)
	}

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/feedback", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "secret-token")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("authenticated feedback status = %d, want 201", resp.StatusCode)
	}
}

func TestServeSubmitRejectsShellShortcut(t *testing.T) {
	bc := NewBroadcaster()
	got := make(chan string, 1)
	ctrl := control.New(control.Options{Runner: fakeRunner{got: got}, Sink: bc})
	srv := httptest.NewServer(New(ctrl, bc, config.ServeConfig{}).Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/submit", "application/json", strings.NewReader(`{"input":"!echo nope"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("shell submit status = %d, want 403", resp.StatusCode)
	}
	select {
	case in := <-got:
		t.Fatalf("runner should not run shell submit, got %q", in)
	default:
	}
}

func TestHistoryMessagesPreserveToolDetails(t *testing.T) {
	got := historyMessages([]control.TranscriptMessage{
		{Role: control.TranscriptUser, Content: "run command"},
		{Role: control.TranscriptAssistant, Content: "checking", Reasoning: "think", ToolCalls: []control.TranscriptToolCall{{
			ID: "call_1", Name: "bash", Arguments: `{"command":"pwd"}`,
		}}},
		{Role: control.TranscriptTool, ToolName: "bash", ToolCallID: "call_1", Content: "/tmp/project\n"},
	})

	if len(got) != 3 {
		t.Fatalf("history length = %d, want 3", len(got))
	}
	if got[1].Reasoning != "think" {
		t.Fatalf("assistant reasoning = %q, want think", got[1].Reasoning)
	}
	if len(got[1].ToolCalls) != 1 || got[1].ToolCalls[0].ID != "call_1" || got[1].ToolCalls[0].Name != "bash" || got[1].ToolCalls[0].Arguments != `{"command":"pwd"}` {
		t.Fatalf("assistant tool calls not preserved: %+v", got[1].ToolCalls)
	}
	if got[2].ToolCallID != "call_1" || got[2].ToolName != "bash" || got[2].Content != "/tmp/project\n" {
		t.Fatalf("tool result details not preserved: %+v", got[2])
	}
}

func TestServeHistoryDoesNotExposeHiddenPromptMaterial(t *testing.T) {
	bc := NewBroadcaster()
	session := agent.NewSession("SYSTEM-SECRET")
	session.Add(provider.Message{Role: provider.RoleUser, Content: "<reasoning-language>English</reasoning-language>\n\nvisible request"})
	session.Add(provider.Message{Role: provider.RoleUser, Content: "Referenced context:\n<file path=\"secret.txt\">FILE-SECRET</file>\n\nexplain it"})
	session.Add(provider.Message{Role: provider.RoleAssistant, Content: "answer"})
	ctrl := control.New(control.Options{
		Executor: agent.New(nil, nil, session, agent.Options{}, bc),
		Sink:     bc,
	})
	srv := httptest.NewServer(New(ctrl, bc, config.ServeConfig{}).Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/history")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, secret := range []string{"SYSTEM-SECRET", "reasoning-language", "FILE-SECRET"} {
		if strings.Contains(text, secret) {
			t.Fatalf("history leaked %q: %s", secret, text)
		}
	}
	for _, visible := range []string{"visible request", "explain it", "answer"} {
		if !strings.Contains(text, visible) {
			t.Fatalf("history dropped %q: %s", visible, text)
		}
	}
}

func TestSessionsListPreviewStripsTransientReasoningLanguageBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	s := agent.NewSession("system")
	s.Add(provider.Message{Role: provider.RoleUser, Content: "<reasoning-language>\nVisible reasoning/thinking text preference: use English.\n</reasoning-language>\n\nExplain this module"})
	if err := s.Save(path); err != nil {
		t.Fatal(err)
	}

	preview, turns := agent.SessionPreview(path)
	if turns != 1 {
		t.Errorf("turns = %d, want 1", turns)
	}
	if preview != "Explain this module" {
		t.Errorf("preview = %q, want user prompt", preview)
	}
}

func TestSessionsListPreviewSeesEventLogTurns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	s := agent.NewSession("system")
	s.Add(provider.Message{Role: provider.RoleUser, Content: "first"})
	if err := s.SaveSnapshot(path); err != nil {
		t.Fatal(err)
	}
	s.Add(provider.Message{Role: provider.RoleAssistant, Content: "reply"})
	s.Add(provider.Message{Role: provider.RoleUser, Content: "second"})
	if err := s.SaveSnapshot(path); err != nil {
		t.Fatal(err)
	}

	// The second turn lives only in the event log; a checkpoint-only reader
	// would still report one turn.
	if _, turns := agent.SessionPreview(path); turns != 2 {
		t.Errorf("turns = %d, want 2 (event log turns visible)", turns)
	}
	if mod := agent.SessionContentModTime(path); mod.IsZero() {
		t.Error("SessionContentModTime returned zero for a live session")
	}
}

func TestServeCancelEndpoint(t *testing.T) {
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc})
	srv := httptest.NewServer(New(ctrl, bc, config.ServeConfig{}).Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/cancel", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("cancel status = %d, want 204", resp.StatusCode)
	}
}

func TestServeApproveMissingID(t *testing.T) {
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc})
	srv := httptest.NewServer(New(ctrl, bc, config.ServeConfig{}).Handler())
	defer srv.Close()

	// Missing id should return 400.
	resp, err := http.Post(srv.URL+"/approve", "application/json", strings.NewReader(`{"allow":true}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("approve missing id = %d, want 400", resp.StatusCode)
	}

	// Malformed JSON should return 400.
	resp2, _ := http.Post(srv.URL+"/approve", "application/json", strings.NewReader(`{bad`))
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusBadRequest {
		t.Errorf("approve bad json = %d, want 400", resp2.StatusCode)
	}
}

func TestServeNewSessionEndpoint(t *testing.T) {
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc})
	srv := httptest.NewServer(New(ctrl, bc, config.ServeConfig{}).Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/new", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("new session = %d, want 204", resp.StatusCode)
	}
}

func TestServeCompactEndpoint(t *testing.T) {
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc})
	srv := httptest.NewServer(New(ctrl, bc, config.ServeConfig{}).Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/compact", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("compact = %d, want 204", resp.StatusCode)
	}
}

func TestServeIndexPage(t *testing.T) {
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc})
	srv := httptest.NewServer(New(ctrl, bc, config.ServeConfig{}).Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("index status = %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("index content-type = %q, want text/html", ct)
	}
}

func TestServeIndexDefinesQueryHelpers(t *testing.T) {
	html := string(indexHTML)
	for _, want := range []string{
		"const $ = s => document.querySelector(s);",
		"const $$ = s => document.querySelectorAll(s);",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("serve index missing query helper %q", want)
		}
	}
}

func TestServeIndexHandlesRetryingEvents(t *testing.T) {
	html := string(indexHTML)
	for _, want := range []string{
		"case 'retrying': setRetrying(e.retryAttempt,e.retryMax); break;",
		"if(e.kind!=='retrying')clearRetrying();",
		"'retrying_status': 'Retrying ({attempt}/{max})...'",
		"'retrying_status': '正在重试 ({attempt}/{max})...'",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("serve index missing retrying support %q", want)
		}
	}
}

func TestServeIndexShowsApprovalPatchPreview(t *testing.T) {
	html := string(indexHTML)
	for _, want := range []string{
		"approval__diff",
		"'patch_preview': 'Patch preview'",
		"const diff = typeof a.diff === 'string' ? a.diff : '';",
		"const diffHtml = diff ?",
		"escHtml(diff)",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("serve index missing approval patch preview support %q", want)
		}
	}
}

func TestServeIndexPagePassesLanguagePreferenceToClient(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("AppData", filepath.Join(home, "AppData"))

	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc})
	srv := httptest.NewServer(New(ctrl, bc, config.ServeConfig{}).Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	html := string(body)
	if !strings.Contains(html, "const __LANG_PREF = 'auto';") {
		t.Fatalf("default language preference was not passed as auto:\n%s", html)
	}
	if !strings.Contains(html, "applyStaticI18n();") {
		t.Fatal("index should translate static __('key') placeholders on the client")
	}

	cfgPath := config.UserConfigPath()
	if cfgPath == "" {
		t.Fatal("user config path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte("[desktop]\nlanguage = \"en\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	resp, err = http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, err = io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "const __LANG_PREF = 'en';") {
		t.Fatalf("pinned desktop language was not passed through:\n%s", string(body))
	}
}

func TestResumeRequiresSessionPathInsideSessionDir(t *testing.T) {
	dir := t.TempDir()
	active := filepath.Join(dir, "active.jsonl")
	inside := filepath.Join(dir, "inside.jsonl")
	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "outside.jsonl")
	for _, path := range []string{active, inside, outside} {
		if err := os.WriteFile(path, []byte(`{"role":"user","content":"hi"}`+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc, SessionDir: dir, SessionPath: active})
	srv := httptest.NewServer(New(ctrl, bc, config.ServeConfig{}).Handler())
	defer srv.Close()

	post := func(path string) int {
		body, err := json.Marshal(map[string]string{"path": path})
		if err != nil {
			t.Fatal(err)
		}
		resp, err := http.Post(srv.URL+"/resume", "application/json", strings.NewReader(string(body)))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if got := post(outside); got != http.StatusForbidden {
		t.Fatalf("outside resume status = %d, want 403", got)
	}
	if got := post(inside); got != http.StatusNoContent {
		t.Fatalf("inside resume status = %d, want 204", got)
	}
	want, err := filepath.EvalSymlinks(inside)
	if err != nil {
		t.Fatal(err)
	}
	if got := filepath.Clean(ctrl.SessionPath()); got != filepath.Clean(want) {
		t.Fatalf("session path = %q, want %q", got, want)
	}
}

func TestResumeRejectsCleanupPendingSession(t *testing.T) {
	dir := t.TempDir()
	active := filepath.Join(dir, "active.jsonl")
	pending := filepath.Join(dir, "pending.jsonl")
	for _, path := range []string{active, pending} {
		if err := os.WriteFile(path, []byte(`{"role":"user","content":"hi"}`+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := agent.MarkCleanupPending(pending, "delete"); err != nil {
		t.Fatal(err)
	}

	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc, SessionDir: dir, SessionPath: active})
	srv := httptest.NewServer(New(ctrl, bc, config.ServeConfig{}).Handler())
	defer srv.Close()

	body, err := json.Marshal(map[string]string{"path": pending})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(srv.URL+"/resume", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("cleanup-pending resume status = %d, want 400", resp.StatusCode)
	}
	if got := filepath.Clean(ctrl.SessionPath()); got != filepath.Clean(active) {
		t.Fatalf("session path after rejected resume = %q, want active %q", got, active)
	}
}

func TestSessionsSkipsCleanupPending(t *testing.T) {
	dir := t.TempDir()
	active := filepath.Join(dir, "active.jsonl")
	pending := filepath.Join(dir, "pending.jsonl")
	for _, path := range []string{active, pending} {
		if err := os.WriteFile(path, []byte(`{"role":"user","content":"hi"}`+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := agent.MarkCleanupPending(pending, "delete"); err != nil {
		t.Fatal(err)
	}

	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc, SessionDir: dir, SessionPath: active})
	srv := httptest.NewServer(New(ctrl, bc, config.ServeConfig{}).Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/sessions")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got []struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "active" || filepath.Clean(got[0].Path) != filepath.Clean(active) {
		t.Fatalf("/sessions = %+v, want only active session", got)
	}
}

func TestDeleteSessionRequiresSessionNameInsideSessionDir(t *testing.T) {
	dir := t.TempDir()
	active := filepath.Join(dir, "active.jsonl")
	old := filepath.Join(dir, "old.jsonl")
	for _, path := range []string{active, old} {
		if err := os.WriteFile(path, []byte(`{"role":"user","content":"hi"}`+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ref := "sa_20260102_030405_000000000_aabbccddeeff"
	writeServeSubagentArtifact(t, dir, ref, agent.BranchID(old))
	oldJobsDir := jobs.ArtifactDir(old)
	if err := os.MkdirAll(oldJobsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldJobsDir, "bash-1.log"), []byte("output"), 0o644); err != nil {
		t.Fatal(err)
	}
	sibling := dir + "-other"
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatal(err)
	}
	escape := filepath.Join(sibling, "escape.jsonl")
	if err := os.WriteFile(escape, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc, SessionDir: dir, SessionPath: active})
	srv := httptest.NewServer(New(ctrl, bc, config.ServeConfig{}).Handler())
	defer srv.Close()

	post := func(body string) int {
		resp, err := http.Post(srv.URL+"/delete-session", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if got := post(`{"path":"` + escape + `"}`); got != http.StatusBadRequest {
		t.Fatalf("legacy path delete status = %d, want 400", got)
	}
	if got := post(`{"name":"../` + filepath.Base(sibling) + `/escape"}`); got != http.StatusBadRequest {
		t.Fatalf("sibling traversal status = %d, want 400", got)
	}
	if _, err := os.Stat(escape); err != nil {
		t.Fatalf("sibling session was removed: %v", err)
	}
	if got := post(`{"name":"active"}`); got != http.StatusConflict {
		t.Fatalf("active delete status = %d, want 409", got)
	}
	if got := post(`{"name":"old"}`); got != http.StatusNoContent {
		t.Fatalf("valid delete status = %d, want 204", got)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old session still exists or stat failed unexpectedly: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "subagents", ref+".jsonl")); !os.IsNotExist(err) {
		t.Fatalf("old session subagent jsonl still exists or stat failed unexpectedly: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "subagents", ref+".meta.json")); !os.IsNotExist(err) {
		t.Fatalf("old session subagent meta still exists or stat failed unexpectedly: %v", err)
	}
	if _, err := os.Stat(oldJobsDir); !os.IsNotExist(err) {
		t.Fatalf("old session jobs sidecar still exists or stat failed unexpectedly: %v", err)
	}
}

func writeServeSubagentArtifact(t *testing.T, dir, ref, parentSession string) {
	t.Helper()
	subagentDir := filepath.Join(dir, "subagents")
	if err := os.MkdirAll(subagentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subagentDir, ref+".jsonl"), []byte(`{"role":"user","content":"sub"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(agent.SubagentMeta{
		Ref:           ref,
		Status:        agent.SubagentCompleted,
		Kind:          "task",
		Name:          "task",
		ParentSession: parentSession,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subagentDir, ref+".meta.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestServeSubmitMalformedJSON(t *testing.T) {
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc})
	srv := httptest.NewServer(New(ctrl, bc, config.ServeConfig{}).Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/submit", "application/json", strings.NewReader(`{not json`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("malformed submit = %d, want 400", resp.StatusCode)
	}
}

func TestServePlanMalformedJSON(t *testing.T) {
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc})
	srv := httptest.NewServer(New(ctrl, bc, config.ServeConfig{}).Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/plan", "application/json", strings.NewReader(`{bad`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("malformed plan = %d, want 400", resp.StatusCode)
	}
}

func TestServeContextEndpoint(t *testing.T) {
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc})
	srv := httptest.NewServer(New(ctrl, bc, config.ServeConfig{}).Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/context")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("context status = %d", resp.StatusCode)
	}
	var body map[string]int
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode context: %v", err)
	}
	// Before any turn, used should be 0.
	if body["used"] != 0 {
		t.Errorf("used = %d, want 0", body["used"])
	}
}
