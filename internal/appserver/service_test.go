package appserver

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"reames-agent/internal/agent"
	"reames-agent/internal/control"
	"reames-agent/internal/event"
	"reames-agent/internal/provider"
)

type approvalDecision struct {
	id      string
	allow   bool
	session bool
	persist bool
}

type fakeController struct {
	dir  string
	root string
	path string
	sink event.Sink

	mu           sync.Mutex
	history      []control.TranscriptMessage
	persisted    *agent.Session
	runCount     int
	running      bool
	steers       []string
	closed       bool
	interactive  bool
	shutdown     func() error
	skipSnapshot bool

	approvalCh chan approvalDecision
	answerCh   chan []event.AskAnswer
	decisions  []approvalDecision
	answers    [][]event.AskAnswer
}

func newFakeController(dir, root string, sink event.Sink) *fakeController {
	return &fakeController{
		dir: dir, root: root, sink: sink, persisted: agent.NewSession("stable system"),
		approvalCh: make(chan approvalDecision, 4), answerCh: make(chan []event.AskAnswer, 4),
	}
}

func (c *fakeController) RunTurn(ctx context.Context, input string) error {
	c.mu.Lock()
	c.runCount++
	run := c.runCount
	c.running = true
	c.history = append(c.history, control.TranscriptMessage{Index: len(c.history) + 1, Role: control.TranscriptUser, Content: input})
	c.persisted.Add(provider.Message{Role: provider.RoleUser, Content: input})
	c.mu.Unlock()
	defer func() { c.mu.Lock(); c.running = false; c.mu.Unlock() }()
	c.sink.Emit(event.Event{Kind: event.SubagentCompleted})
	if run == 1 {
		c.sink.Emit(event.Event{Kind: event.ToolDispatch, Tool: event.Tool{ID: "tool-1", Name: "install_source", Args: `{"source":"fixture"}`}})
		c.sink.Emit(event.Event{Kind: event.ApprovalRequest, Approval: event.Approval{ID: "approval-1", Tool: "install_source", Subject: "fixture-plugin", Reason: "installs executable code"}})
		select {
		case decision := <-c.approvalCh:
			if !decision.allow {
				return errors.New("approval denied")
			}
		case <-ctx.Done():
			return ctx.Err()
		}
		c.sink.Emit(event.Event{Kind: event.AskRequest, Ask: event.Ask{ID: "ask-1", Questions: []event.AskQuestion{{ID: "choice", Header: "Choice", Prompt: "Pick one", Options: []event.AskOption{{Label: "Alpha", Description: "first"}, {Label: "Beta", Description: "second"}}}}}})
		select {
		case answers := <-c.answerCh:
			if len(answers) != 1 || len(answers[0].Selected) != 1 || answers[0].Selected[0] != "Alpha" {
				return errors.New("invalid ask answer")
			}
		case <-ctx.Done():
			return ctx.Err()
		}
		c.sink.Emit(event.Event{Kind: event.ToolProgress, Tool: event.Tool{ID: "tool-1", Name: "install_source", Output: "progress-secret-must-not-replay"}})
		c.sink.Emit(event.Event{Kind: event.ToolResult, Tool: event.Tool{ID: "tool-1", Name: "install_source", Output: "installed fixture", DurationMs: 12}})
		c.sink.Emit(event.Event{Kind: event.Text, Text: "hello from app-server"})
		c.sink.Emit(event.Event{Kind: event.Message, Text: "hello from app-server"})
		c.mu.Lock()
		c.history = append(c.history, control.TranscriptMessage{Index: len(c.history) + 1, Role: control.TranscriptAssistant, Content: "hello from app-server"})
		c.persisted.Add(provider.Message{Role: provider.RoleAssistant, Content: "hello from app-server"})
		c.mu.Unlock()
		return nil
	}
	select {
	case <-ctx.Done():
		c.mu.Lock()
		c.history = append(c.history, control.TranscriptMessage{Index: len(c.history) + 1, Role: control.TranscriptAssistant, Interrupted: true})
		c.mu.Unlock()
		return ctx.Err()
	}
}

func (c *fakeController) Cancel() {}
func (c *fakeController) TrySteer(text string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.running {
		return false
	}
	c.steers = append(c.steers, text)
	return true
}
func (c *fakeController) Transcript() []control.TranscriptMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]control.TranscriptMessage(nil), c.history...)
}
func (c *fakeController) Approve(id string, allow, session, persist bool) {
	decision := approvalDecision{id: id, allow: allow, session: session, persist: persist}
	c.mu.Lock()
	c.decisions = append(c.decisions, decision)
	c.mu.Unlock()
	c.approvalCh <- decision
}
func (c *fakeController) AnswerQuestion(_ string, answers []event.AskAnswer) {
	c.mu.Lock()
	c.answers = append(c.answers, append([]event.AskAnswer(nil), answers...))
	c.mu.Unlock()
	c.answerCh <- answers
}
func (c *fakeController) EnableInteractiveApproval() { c.interactive = true }
func (c *fakeController) Snapshot() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.path == "" {
		return errors.New("missing path")
	}
	if err := c.persisted.Save(c.path); err != nil {
		return err
	}
	return control.UpdateSessionMeta(c.path, true, func(meta *control.SessionMeta) error {
		meta.WorkspaceRoot = c.root
		meta.Model = "fixture/model"
		return nil
	})
}
func (c *fakeController) SnapshotForShutdown() error {
	if c.shutdown != nil {
		return c.shutdown()
	}
	if c.skipSnapshot {
		return nil
	}
	return c.Snapshot()
}
func (c *fakeController) SessionPath() string   { c.mu.Lock(); defer c.mu.Unlock(); return c.path }
func (c *fakeController) SessionDir() string    { return c.dir }
func (c *fakeController) WorkspaceRoot() string { return c.root }
func (c *fakeController) ResumeSessionPath(path string, before func() error) error {
	loaded, err := agent.LoadSession(path)
	if err != nil {
		return err
	}
	if before != nil {
		if err := before(); err != nil {
			return err
		}
	}
	transcript, err := control.LoadTranscript(path)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.path = path
	c.persisted = loaded
	c.history = transcript
	c.mu.Unlock()
	return nil
}
func (c *fakeController) Close() { c.mu.Lock(); c.closed = true; c.mu.Unlock() }
func (c *fakeController) EnsureSessionPath() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.path == "" {
		c.path = filepath.Join(c.dir, "appserver-fixture.jsonl")
	}
}

type fakeFactory struct {
	dir         string
	root        string
	mu          sync.Mutex
	controllers []*fakeController
}

func (f *fakeFactory) SessionDir() string { return f.dir }
func (f *fakeFactory) NewThread(_ context.Context, p ThreadParams) (ThreadRuntime, error) {
	c := newFakeController(f.dir, f.root, p.Sink)
	f.mu.Lock()
	f.controllers = append(f.controllers, c)
	f.mu.Unlock()
	return ThreadRuntime{Controller: c, Model: "fixture/model", ModelProvider: "fixture", Cwd: f.root}, nil
}
func (f *fakeFactory) latest() *fakeController {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.controllers[len(f.controllers)-1]
}

type wireHarness struct {
	enc    *json.Encoder
	reader *bufio.Reader
	cancel context.CancelFunc
	input  *io.PipeWriter
	done   chan error
}

func newWireHarness(t *testing.T, f Factory) *wireHarness {
	t.Helper()
	serverInput, input := io.Pipe()
	output, serverOutput := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		err := Serve(ctx, serverInput, serverOutput, f, ServerInfo{Name: "reames-agent", Version: "test-version", Home: t.TempDir()})
		_ = serverOutput.Close()
		done <- err
	}()
	h := &wireHarness{enc: json.NewEncoder(input), reader: bufio.NewReader(output), cancel: cancel, input: input, done: done}
	t.Cleanup(func() {
		_ = input.Close()
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("app-server did not stop")
		}
	})
	return h
}

func (h *wireHarness) send(t *testing.T, frame any) {
	t.Helper()
	if err := h.enc.Encode(frame); err != nil {
		t.Fatal(err)
	}
}
func (h *wireHarness) read(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	ch := make(chan struct {
		raw []byte
		err error
	}, 1)
	go func() {
		raw, err := h.reader.ReadBytes('\n')
		ch <- struct {
			raw []byte
			err error
		}{raw, err}
	}()
	select {
	case result := <-ch:
		if result.err != nil {
			t.Fatal(result.err)
		}
		var frame map[string]json.RawMessage
		if err := json.Unmarshal(result.raw, &frame); err != nil {
			t.Fatalf("decode %s: %v", result.raw, err)
		}
		return frame
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for app-server frame")
		return nil
	}
}
func frameID(frame map[string]json.RawMessage) string { return strings.TrimSpace(string(frame["id"])) }
func frameMethod(frame map[string]json.RawMessage) string {
	var method string
	_ = json.Unmarshal(frame["method"], &method)
	return method
}

func readResponse(t *testing.T, h *wireHarness, id string, observe func(map[string]json.RawMessage)) map[string]json.RawMessage {
	t.Helper()
	for {
		frame := h.read(t)
		if frameID(frame) == id && frameMethod(frame) == "" {
			return frame
		}
		if observe != nil {
			observe(frame)
		}
	}
}

func TestAppServerWireLifecycleApprovalReplayAndInterrupt(t *testing.T) {
	dir := t.TempDir()
	root := t.TempDir()
	factory := &fakeFactory{dir: dir, root: root}
	h := newWireHarness(t, factory)
	h.send(t, map[string]any{"id": 1, "method": "thread/list", "params": map[string]any{}})
	notInitialized := h.read(t)
	if string(notInitialized["error"]) == "" || !strings.Contains(string(notInitialized["error"]), "Not initialized") {
		t.Fatalf("pre-init response=%s", mustJSON(notInitialized))
	}
	h.send(t, map[string]any{"id": 2, "method": "initialize", "params": map[string]any{"clientInfo": map[string]any{"name": "fixture", "version": "1.0"}, "capabilities": map[string]any{"experimentalApi": true}}})
	initialized := h.read(t)
	if _, ok := initialized["jsonrpc"]; ok {
		t.Fatalf("App-Server wire must omit jsonrpc: %s", mustJSON(initialized))
	}
	if frameID(initialized) != "2" || strings.Contains(string(initialized["result"]), "progress-secret") {
		t.Fatalf("initialize response=%s", mustJSON(initialized))
	}
	h.send(t, map[string]any{"method": "initialized", "params": map[string]any{}})

	h.send(t, map[string]any{"id": 3, "method": "thread/start", "params": map[string]any{"cwd": root, "historyMode": "legacy"}})
	start := readResponse(t, h, "3", nil)
	var opened ThreadStartResponse
	if err := json.Unmarshal(start["result"], &opened); err != nil {
		t.Fatal(err)
	}
	if opened.Thread.ID == "" || opened.Thread.Preview != "" || opened.Thread.Name != nil || !factory.latest().interactive {
		t.Fatalf("thread/start=%+v", opened)
	}
	threadID := opened.Thread.ID

	h.send(t, map[string]any{"id": 4, "method": "turn/start", "params": map[string]any{"threadId": threadID, "clientUserMessageId": "client-1", "input": []any{map[string]any{"type": "text", "text": "ship it"}}}})
	turnStart := readResponse(t, h, "4", nil)
	var started TurnStartResponse
	if err := json.Unmarshal(turnStart["result"], &started); err != nil {
		t.Fatal(err)
	}
	if started.Turn.Status != "inProgress" || started.Turn.ID == "" {
		t.Fatalf("turn/start=%+v", started)
	}
	turnID := started.Turn.ID
	seenChildCompletion := false
	seenApproval := false
	seenAsk := false
	seenTurnCompletedBeforeSettlement := false
	var completed Turn
	for completed.ID == "" {
		frame := h.read(t)
		method := frameMethod(frame)
		if method == "turn/completed" && (!seenApproval || !seenAsk) {
			seenTurnCompletedBeforeSettlement = true
		}
		if method == "item/commandExecution/requestApproval" {
			seenApproval = true
			var params ApprovalRequestParams
			if err := json.Unmarshal(frame["params"], &params); err != nil {
				t.Fatal(err)
			}
			if params.ThreadID != threadID || params.TurnID != turnID || contains(params.AvailableDecisions, "acceptForSession") {
				t.Fatalf("approval params=%+v", params)
			}
			h.send(t, map[string]any{"id": json.RawMessage(frame["id"]), "result": map[string]any{"decision": "accept"}})
		}
		if method == "item/tool/requestUserInput" {
			seenAsk = true
			var params RequestUserInputParams
			if err := json.Unmarshal(frame["params"], &params); err != nil {
				t.Fatal(err)
			}
			if params.ThreadID != threadID || params.TurnID != turnID || len(params.Questions) != 1 {
				t.Fatalf("ask params=%+v", params)
			}
			h.send(t, map[string]any{"id": json.RawMessage(frame["id"]), "result": map[string]any{"answers": map[string]any{"choice": map[string]any{"answers": []string{"Alpha"}}}}})
		}
		if method == "subagent/completed" {
			seenChildCompletion = true
		}
		if method == "turn/completed" {
			var params struct {
				ThreadID string `json:"threadId"`
				Turn     Turn   `json:"turn"`
			}
			if err := json.Unmarshal(frame["params"], &params); err != nil {
				t.Fatal(err)
			}
			if params.ThreadID != threadID || params.Turn.ID != turnID {
				t.Fatalf("completion identity=%+v", params)
			}
			completed = params.Turn
		}
	}
	if seenTurnCompletedBeforeSettlement {
		t.Fatal("child/runtime activity completed the primary turn before approval and Ask settled")
	}
	if seenChildCompletion {
		t.Fatal("subagent completion leaked as a top-level App-Server completion")
	}
	if completed.Status != "completed" {
		t.Fatalf("completed turn=%+v", completed)
	}
	controller := factory.latest()
	controller.mu.Lock()
	decision := controller.decisions[0]
	controller.mu.Unlock()
	if decision.id != "approval-1" || !decision.allow || decision.session || decision.persist {
		t.Fatalf("approval decision=%+v", decision)
	}
	controller.mu.Lock()
	recordedAnswers := append([][]event.AskAnswer(nil), controller.answers...)
	controller.mu.Unlock()
	if len(recordedAnswers) != 1 || recordedAnswers[0][0].Selected[0] != "Alpha" {
		t.Fatalf("Ask answers=%+v", recordedAnswers)
	}

	h.send(t, map[string]any{"id": 5, "method": "thread/read", "params": map[string]any{"threadId": threadID, "includeTurns": true}})
	read := readResponse(t, h, "5", nil)
	if strings.Contains(string(read["result"]), "progress-secret-must-not-replay") {
		t.Fatalf("transient tool progress entered replay: %s", read["result"])
	}
	var readResult ThreadReadResponse
	if err := json.Unmarshal(read["result"], &readResult); err != nil {
		t.Fatal(err)
	}
	if len(readResult.Thread.Turns) != 1 || readResult.Thread.Turns[0].ID != turnID || readResult.Thread.Preview != "ship it" {
		t.Fatalf("thread/read=%+v", readResult.Thread)
	}

	h.send(t, map[string]any{"id": 6, "method": "thread/name/set", "params": map[string]any{"threadId": threadID, "name": "Explicit name"}})
	_ = readResponse(t, h, "6", nil)
	h.send(t, map[string]any{"id": 7, "method": "thread/list", "params": map[string]any{"limit": 1, "sortKey": "updated_at"}})
	listFrame := readResponse(t, h, "7", nil)
	var list ThreadListResponse
	if err := json.Unmarshal(listFrame["result"], &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Data) != 1 || list.Data[0].Name == nil || *list.Data[0].Name != "Explicit name" || list.Data[0].Preview != "ship it" {
		t.Fatalf("thread/list must separate explicit name and preview: %+v", list.Data)
	}

	h.send(t, map[string]any{"id": 8, "method": "turn/start", "params": map[string]any{"threadId": threadID, "input": []any{map[string]any{"type": "text", "text": "block"}}}})
	secondFrame := readResponse(t, h, "8", nil)
	var second TurnStartResponse
	_ = json.Unmarshal(secondFrame["result"], &second)
	h.send(t, map[string]any{"id": 9, "method": "turn/interrupt", "params": map[string]any{"threadId": threadID, "turnId": "wrong-turn"}})
	wrong := readResponse(t, h, "9", nil)
	if !strings.Contains(string(wrong["error"]), "identity does not match") {
		t.Fatalf("wrong interrupt=%s", mustJSON(wrong))
	}
	h.send(t, map[string]any{"id": 10, "method": "turn/interrupt", "params": map[string]any{"threadId": threadID, "turnId": second.Turn.ID}})
	_ = readResponse(t, h, "10", nil)
	for {
		frame := h.read(t)
		if frameMethod(frame) != "turn/completed" {
			continue
		}
		var params struct {
			ThreadID string `json:"threadId"`
			Turn     Turn   `json:"turn"`
		}
		_ = json.Unmarshal(frame["params"], &params)
		if params.ThreadID == threadID && params.Turn.ID == second.Turn.ID {
			if params.Turn.Status != "interrupted" {
				t.Fatalf("interrupt completion=%+v", params.Turn)
			}
			break
		}
	}

	h.send(t, map[string]any{"id": 11, "method": "thread/unsubscribe", "params": map[string]any{"threadId": threadID}})
	unsub := readResponse(t, h, "11", nil)
	if !strings.Contains(string(unsub["result"]), "unsubscribed") {
		t.Fatalf("unsubscribe=%s", mustJSON(unsub))
	}
}

func TestThreadStartRejectsUnknownAndUnsupportedFieldsBeforeMutation(t *testing.T) {
	factory := &fakeFactory{dir: t.TempDir(), root: t.TempDir()}
	svc := &service{factory: factory, info: ServerInfo{Version: "test"}, threads: make(map[string]*threadSession), optOut: make(map[string]struct{})}
	if _, err := svc.threadStart(context.Background(), json.RawMessage(`{"historyMode":"future"}`)); err == nil || !strings.Contains(err.Error(), "unknown historyMode") {
		t.Fatalf("future history mode err=%v", err)
	}
	if _, err := svc.threadStart(context.Background(), json.RawMessage(`{"sandbox":"danger-full-access"}`)); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("ignored sandbox override err=%v", err)
	}
	if _, err := svc.threadStart(context.Background(), json.RawMessage(`{"historyMode":"paginated"}`)); err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("paginated mode err=%v", err)
	}
	factory.mu.Lock()
	count := len(factory.controllers)
	factory.mu.Unlock()
	if count != 0 {
		t.Fatalf("invalid requests built %d controllers", count)
	}
}

func TestThreadAndTurnMethodsRejectMissingIdentityBeforeLookup(t *testing.T) {
	svc := &service{threads: make(map[string]*threadSession), optOut: make(map[string]struct{})}
	tests := []struct {
		name string
		call func() (any, error)
		want string
	}{
		{"unsubscribe", func() (any, error) { return svc.threadUnsubscribe(context.Background(), json.RawMessage(`{}`)) }, "threadId is required"},
		{"turn-start", func() (any, error) {
			return svc.turnStart(context.Background(), json.RawMessage(`{"input":[{"type":"text","text":"hello"}]}`))
		}, "threadId is required"},
		{"turn-steer", func() (any, error) {
			return svc.turnSteer(context.Background(), json.RawMessage(`{"threadId":"thread","input":[{"type":"text","text":"hello"}]}`))
		}, "expectedTurnId"},
		{"turn-interrupt", func() (any, error) {
			return svc.turnInterrupt(context.Background(), json.RawMessage(`{"threadId":"thread"}`))
		}, "turnId"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.call()
			var rpcErr *RPCError
			if !errors.As(err, &rpcErr) || rpcErr.Code != ErrInvalidParams || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want invalid params containing %q", err, test.want)
			}
		})
	}
}

func TestTranscriptTurnsAreStableAndDisplaySafe(t *testing.T) {
	messages := []control.TranscriptMessage{
		{Index: 0, Role: control.TranscriptSystem, Hidden: true, Content: "system-secret"},
		{Index: 1, Role: control.TranscriptUser, Content: "first task"},
		{Index: 2, Role: control.TranscriptAssistant, Reasoning: "bounded reasoning", Content: "done", ToolCalls: []control.TranscriptToolCall{{ID: "call-1", Name: "read_file", Arguments: `{"path":"a.txt"}`}}},
		{Index: 3, Role: control.TranscriptTool, ToolCallID: "call-1", ToolName: "read_file", Content: "file text"},
		{Index: 4, Role: control.TranscriptUser, Content: "guidance", SteerText: "guidance"},
		{Index: 5, Role: control.TranscriptUser, Content: "second task"},
		{Index: 6, Role: control.TranscriptAssistant, Interrupted: true},
	}
	first := transcriptTurns("thread-a", messages)
	second := transcriptTurns("thread-a", messages)
	if len(first) != 2 || first[0].ID != second[0].ID || first[1].Status != "interrupted" {
		t.Fatalf("turn projection=%+v", first)
	}
	raw := mustJSON(first)
	for _, forbidden := range []string{"system-secret", "progress-secret"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("projection leaked %q: %s", forbidden, raw)
		}
	}
	for _, required := range []string{"first task", "bounded reasoning", "file text", "guidance", "second task"} {
		if !strings.Contains(raw, required) {
			t.Fatalf("projection missing %q: %s", required, raw)
		}
	}
	if nextTurnID("thread-a", messages) == first[1].ID {
		t.Fatal("next turn id reused the last persisted turn")
	}
}

func TestDecodeThreadListParamsIsStrictAndAcceptsCwdForms(t *testing.T) {
	for _, raw := range []string{`{"cwd":"C:/one"}`, `{"cwd":["C:/one","C:/two"]}`} {
		var p ThreadListParams
		if err := decodeThreadListParams(json.RawMessage(raw), &p); err != nil || len(p.Cwd) == 0 {
			t.Fatalf("decode %s: %+v %v", raw, p, err)
		}
	}
	var p ThreadListParams
	if err := decodeThreadListParams(json.RawMessage(`{"archived":true}`), &p); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unsupported filter err=%v", err)
	}
}

func TestThreadResumeLoadsCanonicalTranscriptAndPinsStableMetadata(t *testing.T) {
	dir := t.TempDir()
	root := t.TempDir()
	path := filepath.Join(dir, "stored.jsonl")
	persisted := agent.NewSession("system-secret")
	persisted.Add(provider.Message{Role: provider.RoleUser, Content: "resume me"})
	persisted.Add(provider.Message{Role: provider.RoleAssistant, Content: "resumed"})
	if err := persisted.Save(path); err != nil {
		t.Fatal(err)
	}
	if err := control.UpdateSessionMeta(path, true, func(meta *control.SessionMeta) error {
		meta.WorkspaceRoot = root
		meta.Model = "fixture/model"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	id := control.BranchID(path)
	factory := &fakeFactory{dir: dir, root: root}
	svc := &service{conn: NewConn(strings.NewReader(""), io.Discard), factory: factory, info: ServerInfo{Version: "test"}, threads: make(map[string]*threadSession), optOut: make(map[string]struct{}), initialized: true}
	t.Cleanup(svc.closeAll)
	result, err := svc.threadResume(context.Background(), json.RawMessage(`{"threadId":"`+id+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	opened, ok := result.(ThreadStartResponse)
	if !ok {
		t.Fatalf("resume result type=%T", result)
	}
	if opened.Thread.ID != id || len(opened.Thread.Turns) != 1 || opened.Thread.Turns[0].Status != "completed" {
		t.Fatalf("resume=%+v", opened.Thread)
	}
	if strings.Contains(mustJSON(opened.Thread), "system-secret") {
		t.Fatalf("resume leaked system prompt: %s", mustJSON(opened.Thread))
	}
	meta, ok, err := loadAppServerMeta(path)
	if err != nil || !ok || meta.ThreadID != id || meta.ActiveTranscript != filepath.Base(path) {
		t.Fatalf("app-server meta=%+v ok=%v err=%v", meta, ok, err)
	}
}

func TestRecoveryRedirectKeepsThreadIdentityAndMovesLease(t *testing.T) {
	dir := t.TempDir()
	original := filepath.Join(dir, "original.jsonl")
	recovery := filepath.Join(dir, "recovery.jsonl")
	for _, path := range []string{original, recovery} {
		session := agent.NewSession("sys")
		session.Add(provider.Message{Role: provider.RoleUser, Content: "task"})
		if err := session.Save(path); err != nil {
			t.Fatal(err)
		}
	}
	id := control.BranchID(original)
	if err := saveAppServerMeta(original, appServerMeta{ThreadID: id, OriginTranscript: filepath.Base(original), ActiveTranscript: filepath.Base(original)}); err != nil {
		t.Fatal(err)
	}
	lease := control.NewSessionLeaseKeeper()
	if err := lease.Rebind(original); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(lease.Release)
	factory := &fakeFactory{dir: dir, root: dir}
	svc := &service{factory: factory, info: ServerInfo{Version: "test"}, threads: make(map[string]*threadSession), optOut: make(map[string]struct{})}
	controller := newFakeController(dir, dir, event.Discard)
	controller.path = original
	session := &threadSession{id: id, ctrl: controller, lease: lease, path: original, origin: original, cwd: dir, subscribed: true}
	sink := newEventSink(svc)
	session.sink = sink
	sink.bind(session)
	if err := sink.sessionRecovered(control.SessionRecoveryInfo{RecoveryPath: recovery}); err != nil {
		t.Fatal(err)
	}
	if session.path != recovery {
		t.Fatalf("active path=%q want %q", session.path, recovery)
	}
	for _, path := range []string{original, recovery} {
		meta, ok, err := loadAppServerMeta(path)
		if err != nil || !ok || meta.ThreadID != id || meta.ActiveTranscript != filepath.Base(recovery) {
			t.Fatalf("meta %s=%+v ok=%v err=%v", path, meta, ok, err)
		}
	}
	oldProbe := control.NewSessionLeaseKeeper()
	if err := oldProbe.Rebind(original); err != nil {
		t.Fatalf("old lease was not released: %v", err)
	}
	oldProbe.Release()
	newProbe := control.NewSessionLeaseKeeper()
	if err := newProbe.Rebind(recovery); err == nil || !control.IsSessionLeaseHeld(err) {
		newProbe.Release()
		t.Fatalf("recovery lease not held: %v", err)
	}
	records, err := svc.sessionRecords()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].id != id || records[0].info.Path != recovery {
		t.Fatalf("recovery records=%+v", records)
	}
}

func TestRecoveryMetadataPreflightFailureLeavesOriginalLease(t *testing.T) {
	dir := t.TempDir()
	original := filepath.Join(dir, "original.jsonl")
	recovery := filepath.Join(dir, "recovery.jsonl")
	for _, path := range []string{original, recovery} {
		session := agent.NewSession("sys")
		session.Add(provider.Message{Role: provider.RoleUser, Content: "task"})
		if err := session.Save(path); err != nil {
			t.Fatal(err)
		}
	}
	id := control.BranchID(original)
	if err := saveAppServerMeta(original, appServerMeta{ThreadID: id, OriginTranscript: filepath.Base(original), ActiveTranscript: filepath.Base(original)}); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(appServerMetaPath(recovery), 0o700); err != nil {
		t.Fatal(err)
	}
	lease := control.NewSessionLeaseKeeper()
	if err := lease.Rebind(original); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(lease.Release)
	svc := &service{factory: &fakeFactory{dir: dir, root: dir}, threads: make(map[string]*threadSession), optOut: make(map[string]struct{})}
	controller := newFakeController(dir, dir, event.Discard)
	controller.path = original
	session := &threadSession{id: id, ctrl: controller, lease: lease, path: original, origin: original, cwd: dir}
	sink := newEventSink(svc)
	session.sink = sink
	sink.bind(session)
	if err := sink.sessionRecovered(control.SessionRecoveryInfo{RecoveryPath: recovery}); err == nil {
		t.Fatal("expected metadata preflight failure")
	}
	if session.path != original {
		t.Fatalf("failure moved active path to %q", session.path)
	}
	oldProbe := control.NewSessionLeaseKeeper()
	if err := oldProbe.Rebind(original); err == nil || !control.IsSessionLeaseHeld(err) {
		oldProbe.Release()
		t.Fatalf("original lease not retained: %v", err)
	}
	newProbe := control.NewSessionLeaseKeeper()
	if err := newProbe.Rebind(recovery); err != nil {
		t.Fatalf("failure unexpectedly held recovery lease: %v", err)
	}
	newProbe.Release()
}

func TestShutdownRecoveryCanMoveLeaseWhileThreadIsClosing(t *testing.T) {
	dir := t.TempDir()
	original := filepath.Join(dir, "original.jsonl")
	recovery := filepath.Join(dir, "recovery.jsonl")
	for _, path := range []string{original, recovery} {
		session := agent.NewSession("sys")
		session.Add(provider.Message{Role: provider.RoleUser, Content: "task"})
		if err := session.Save(path); err != nil {
			t.Fatal(err)
		}
	}
	id := control.BranchID(original)
	if err := saveAppServerMeta(original, appServerMeta{ThreadID: id, OriginTranscript: filepath.Base(original), ActiveTranscript: filepath.Base(original)}); err != nil {
		t.Fatal(err)
	}
	lease := control.NewSessionLeaseKeeper()
	if err := lease.Rebind(original); err != nil {
		t.Fatal(err)
	}
	svc := &service{factory: &fakeFactory{dir: dir, root: dir}, threads: make(map[string]*threadSession), optOut: make(map[string]struct{})}
	controller := newFakeController(dir, dir, event.Discard)
	controller.path = original
	session := &threadSession{id: id, ctrl: controller, lease: lease, path: original, origin: original, cwd: dir}
	sink := newEventSink(svc)
	session.sink = sink
	sink.bind(session)
	controller.shutdown = func() error {
		return sink.sessionRecovered(control.SessionRecoveryInfo{OriginalPath: original, RecoveryPath: recovery, Reason: "shutdown fixture"})
	}

	session.close()
	if session.path != recovery || !session.closed || session.closing {
		t.Fatalf("closed recovery session = %+v", session)
	}
	for _, path := range []string{original, recovery} {
		meta, ok, err := loadAppServerMeta(path)
		if err != nil || !ok || meta.ThreadID != id || meta.ActiveTranscript != filepath.Base(recovery) {
			t.Fatalf("shutdown recovery meta %s=%+v ok=%v err=%v", path, meta, ok, err)
		}
	}
	probe := control.NewSessionLeaseKeeper()
	if err := probe.Rebind(recovery); err != nil {
		t.Fatalf("closed session retained recovery lease: %v", err)
	}
	probe.Release()
}

func TestLoadedThreadResumeRejectsSilentRuntimeOverrides(t *testing.T) {
	svc := &service{threads: make(map[string]*threadSession), optOut: make(map[string]struct{})}
	svc.threads["thread"] = &threadSession{id: "thread", model: "fixture/model", cwd: filepath.Clean("C:/workspace")}
	for _, raw := range []string{
		`{"threadId":"thread","model":"other/model"}`,
		`{"threadId":"thread","cwd":"C:/other"}`,
	} {
		_, err := svc.threadResume(context.Background(), json.RawMessage(raw))
		var rpcErr *RPCError
		if !errors.As(err, &rpcErr) || rpcErr.Code != ErrInvalidParams || !strings.Contains(err.Error(), "already loaded") {
			t.Fatalf("resume override %s error = %v", raw, err)
		}
	}
}

func TestCloseRemovesMetadataForNeverPersistedThread(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unused.jsonl")
	id := control.BranchID(path)
	if err := saveAppServerMeta(path, appServerMeta{ThreadID: id, OriginTranscript: filepath.Base(path), ActiveTranscript: filepath.Base(path)}); err != nil {
		t.Fatal(err)
	}
	controller := newFakeController(dir, dir, event.Discard)
	controller.path = path
	controller.skipSnapshot = true
	session := &threadSession{id: id, ctrl: controller, path: path, origin: path}
	session.close()
	if _, err := os.Stat(appServerMetaPath(path)); !os.IsNotExist(err) {
		t.Fatalf("unused App-Server metadata survived close: %v", err)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
func mustJSON(value any) string { raw, _ := json.Marshal(value); return string(raw) }
