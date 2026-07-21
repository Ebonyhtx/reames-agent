package appserver

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"reames-agent/internal/control"
	"reames-agent/internal/event"
	"reames-agent/internal/eventwire"
)

type turnState struct {
	threadID string
	id       string
	started  time.Time
	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}

	mu         sync.Mutex
	items      map[string]ThreadItem
	order      []string
	terminal   map[string]bool
	userItem   string
	agentItem  string
	reasonItem string
	completed  time.Time
}

func newTurnState(threadID, turnID, clientID, text string, cancel context.CancelFunc) *turnState {
	started := time.Now()
	state := &turnState{
		threadID: threadID, id: turnID, started: started, cancel: cancel, done: make(chan struct{}),
		items: make(map[string]ThreadItem), terminal: make(map[string]bool),
	}
	state.userItem = stableID(turnID, "user")
	state.items[state.userItem] = ThreadItem{
		"type": "userMessage", "id": state.userItem, "clientId": nullableString(clientID),
		"content": []any{map[string]any{"type": "text", "text": text, "textElements": []any{}}},
	}
	state.order = append(state.order, state.userItem)
	return state
}

func (t *turnState) setContext(ctx context.Context) { t.ctx = ctx }

func (t *turnState) turn(status string, turnErr *TurnError) Turn {
	items := t.snapshotItems()
	started := t.started.Unix()
	var completed *int64
	var duration *int64
	if status != "inProgress" {
		t.mu.Lock()
		if t.completed.IsZero() {
			t.completed = time.Now()
		}
		end := t.completed
		t.mu.Unlock()
		value := end.Unix()
		elapsed := end.Sub(t.started).Milliseconds()
		completed, duration = &value, &elapsed
	}
	return Turn{
		ID: t.id, Items: items, ItemsView: "full", Status: status, Error: turnErr,
		StartedAt: &started, CompletedAt: completed, DurationMs: duration,
	}
}

func (t *turnState) snapshotItems() []ThreadItem {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]ThreadItem, 0, len(t.order))
	for _, id := range t.order {
		out = append(out, cloneItem(t.items[id]))
	}
	return out
}

func (t *turnState) emitUserLifecycle(s *service) {
	t.mu.Lock()
	item := cloneItem(t.items[t.userItem])
	t.terminal[t.userItem] = true
	t.mu.Unlock()
	startedMs := t.started.UnixMilli()
	s.notifyThread(t.threadID, "item/started", map[string]any{"threadId": t.threadID, "turnId": t.id, "item": item, "startedAtMs": startedMs})
	s.notifyThread(t.threadID, "item/completed", map[string]any{"threadId": t.threadID, "turnId": t.id, "item": item, "completedAtMs": time.Now().UnixMilli()})
}

func (t *turnState) ensureAgentItem(reasoning bool) (string, ThreadItem, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if reasoning {
		if t.reasonItem != "" {
			return t.reasonItem, cloneItem(t.items[t.reasonItem]), false
		}
		t.reasonItem = stableID(t.id, "reasoning")
		item := ThreadItem{"type": "reasoning", "id": t.reasonItem, "summary": []string{}, "content": []string{}}
		t.items[t.reasonItem] = item
		t.order = append(t.order, t.reasonItem)
		return t.reasonItem, cloneItem(item), true
	}
	if t.agentItem != "" {
		return t.agentItem, cloneItem(t.items[t.agentItem]), false
	}
	t.agentItem = stableID(t.id, "agent-message")
	item := ThreadItem{"type": "agentMessage", "id": t.agentItem, "text": "", "phase": nil, "memoryCitation": nil}
	t.items[t.agentItem] = item
	t.order = append(t.order, t.agentItem)
	return t.agentItem, cloneItem(item), true
}

func (t *turnState) appendAgentText(id, delta string, reasoning bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	item := t.items[id]
	if reasoning {
		content, _ := item["content"].([]string)
		if len(content) == 0 {
			content = []string{""}
		}
		content[len(content)-1] += delta
		item["content"] = content
	} else {
		item["text"], _ = item["text"].(string)
		item["text"] = item["text"].(string) + delta
	}
}

func (t *turnState) addTool(eventTool event.Tool) (ThreadItem, bool) {
	id := strings.TrimSpace(eventTool.ID)
	if id == "" {
		id = stableID(t.id, "tool", eventTool.Name, strconvItoa(len(t.order)))
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if item := t.items[id]; item != nil {
		return cloneItem(item), false
	}
	var args any = map[string]any{}
	if strings.TrimSpace(eventTool.Args) != "" {
		if json.Unmarshal([]byte(eventTool.Args), &args) != nil {
			args = map[string]any{"raw": eventTool.Args}
		}
	}
	item := ThreadItem{
		"type": "dynamicToolCall", "id": id, "namespace": nil, "tool": eventTool.Name,
		"arguments": args, "status": "inProgress", "contentItems": nil, "success": nil,
		"durationMs": nil,
	}
	t.items[id] = item
	t.order = append(t.order, id)
	return cloneItem(item), true
}

func (t *turnState) completeTool(eventTool event.Tool) (ThreadItem, bool) {
	id := strings.TrimSpace(eventTool.ID)
	if id == "" {
		id = stableID(t.id, "tool-result", eventTool.Name, strconvItoa(len(t.order)))
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	item := t.items[id]
	fresh := item == nil
	if item == nil {
		item = ThreadItem{"type": "dynamicToolCall", "id": id, "namespace": nil, "tool": eventTool.Name, "arguments": map[string]any{}}
		t.items[id] = item
		t.order = append(t.order, id)
	}
	success := eventTool.Err == ""
	output := eventTool.Output
	if !success {
		output = eventTool.Err
	}
	item["status"] = map[bool]string{true: "completed", false: "failed"}[success]
	item["success"] = success
	item["contentItems"] = []any{map[string]any{"type": "inputText", "text": output}}
	if eventTool.DurationMs > 0 {
		item["durationMs"] = eventTool.DurationMs
	}
	t.terminal[id] = true
	return cloneItem(item), fresh
}

func (t *turnState) finishItems() []ThreadItem {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]ThreadItem, 0)
	for _, id := range t.order {
		if t.terminal[id] {
			continue
		}
		item := t.items[id]
		if item["type"] == "dynamicToolCall" {
			item["status"] = "failed"
			item["success"] = false
			if item["contentItems"] == nil {
				item["contentItems"] = []any{}
			}
		}
		t.terminal[id] = true
		out = append(out, cloneItem(item))
	}
	return out
}

type eventSink struct {
	service *service
	mu      sync.Mutex
	session *threadSession
	current *turnState
	approve func(string, bool, bool, bool)
	answer  func(string, []event.AskAnswer)
}

func newEventSink(service *service) *eventSink { return &eventSink{service: service} }

func (s *eventSink) bind(session *threadSession) { s.mu.Lock(); s.session = session; s.mu.Unlock() }
func (s *eventSink) bindApproval(approve func(string, bool, bool, bool), answer func(string, []event.AskAnswer)) {
	s.mu.Lock()
	s.approve, s.answer = approve, answer
	s.mu.Unlock()
}

func (s *eventSink) begin(state *turnState) {
	s.mu.Lock()
	s.current = state
	s.mu.Unlock()
}

func (s *eventSink) state() *turnState {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.current
	if state == nil || s.session == nil {
		return nil
	}
	s.session.mu.Lock()
	matched := s.session.active == state && s.session.id == state.threadID
	s.session.mu.Unlock()
	if !matched {
		return nil
	}
	return state
}

func (s *eventSink) finish(state *turnState) {
	s.mu.Lock()
	if s.current != state {
		s.mu.Unlock()
		return
	}
	s.current = nil
	s.mu.Unlock()
	for _, item := range state.finishItems() {
		s.service.notifyThread(state.threadID, "item/completed", map[string]any{
			"threadId": state.threadID, "turnId": state.id, "item": item, "completedAtMs": time.Now().UnixMilli(),
		})
	}
}

func (s *eventSink) Emit(e event.Event) {
	state := s.state()
	if state == nil {
		return
	}
	switch e.Kind {
	case event.Text:
		if e.Text != "" {
			s.emitText(state, e.Text, false)
		}
	case event.Reasoning:
		if e.Text != "" {
			s.emitText(state, e.Text, true)
		}
	case event.Message:
		// Message is the finalized aggregate after Text deltas. Only use it when
		// the runtime emitted no deltas, otherwise it would duplicate content.
		state.mu.Lock()
		missing := state.agentItem == ""
		state.mu.Unlock()
		if missing && e.Text != "" {
			s.emitText(state, e.Text, false)
		}
	case event.ToolDispatch:
		if e.Tool.Partial {
			return
		}
		item, fresh := state.addTool(e.Tool)
		if fresh {
			s.service.notifyThread(state.threadID, "item/started", map[string]any{
				"threadId": state.threadID, "turnId": state.id, "item": item, "startedAtMs": time.Now().UnixMilli(),
			})
		}
	case event.ToolResult:
		item, fresh := state.completeTool(e.Tool)
		if fresh {
			s.service.notifyThread(state.threadID, "item/started", map[string]any{
				"threadId": state.threadID, "turnId": state.id, "item": item, "startedAtMs": time.Now().UnixMilli(),
			})
		}
		s.service.notifyThread(state.threadID, "item/completed", map[string]any{
			"threadId": state.threadID, "turnId": state.id, "item": item, "completedAtMs": time.Now().UnixMilli(),
		})
	case event.ApprovalRequest:
		go s.requestApproval(state, e.Approval)
	case event.AskRequest:
		go s.requestInput(state, e.Ask)
	}
}

func (s *eventSink) emitText(state *turnState, delta string, reasoning bool) {
	id, item, fresh := state.ensureAgentItem(reasoning)
	if fresh {
		s.service.notifyThread(state.threadID, "item/started", map[string]any{
			"threadId": state.threadID, "turnId": state.id, "item": item, "startedAtMs": time.Now().UnixMilli(),
		})
	}
	state.appendAgentText(id, delta, reasoning)
	method := "item/agentMessage/delta"
	if reasoning {
		method = "item/reasoning/textDelta"
	}
	s.service.notifyThread(state.threadID, method, map[string]any{
		"threadId": state.threadID, "turnId": state.id, "itemId": id, "delta": delta,
	})
}

func (s *eventSink) requestApproval(state *turnState, approval event.Approval) {
	s.mu.Lock()
	approve := s.approve
	session := s.session
	s.mu.Unlock()
	if approve == nil || session == nil {
		return
	}
	method := "item/commandExecution/requestApproval"
	if isFileApprovalTool(approval.Tool) {
		method = "item/fileChange/requestApproval"
	}
	decisions := []string{"accept", "decline", "cancel"}
	if approvalAllowsSession(approval.Tool) {
		decisions = []string{"accept", "acceptForSession", "decline", "cancel"}
	}
	params := ApprovalRequestParams{
		ThreadID: state.threadID, TurnID: state.id, ItemID: "gate-" + approval.ID,
		Reason: approval.Reason, Command: approval.Subject, Cwd: session.cwd, AvailableDecisions: decisions,
	}
	if approval.Plan != nil {
		wire := eventwire.ToWire(event.Event{Kind: event.ApprovalRequest, Approval: approval})
		raw, _ := json.Marshal(wire.Approval.Plan)
		_ = json.Unmarshal(raw, &params.ReamesPlan)
	}
	s.service.notifyThread(state.threadID, "thread/status/changed", map[string]any{
		"threadId": state.threadID, "status": ThreadStatus{Type: "active", ActiveFlags: []string{"waitingOnApproval"}},
	})
	raw, err := s.service.conn.Request(state.ctx, method, params)
	allow, allowSession := false, false
	if err == nil && s.state() == state {
		var response ApprovalResponse
		if strictDecode(raw, &response) == nil {
			switch response.Decision {
			case "accept":
				allow = true
			case "acceptForSession":
				if approvalAllowsSession(approval.Tool) {
					allow, allowSession = true, true
				}
			}
		}
	}
	approve(approval.ID, allow, allowSession, false)
	if s.state() == state {
		s.service.notifyThread(state.threadID, "thread/status/changed", map[string]any{"threadId": state.threadID, "status": ThreadStatus{Type: "active"}})
	}
}

func (s *eventSink) requestInput(state *turnState, ask event.Ask) {
	s.mu.Lock()
	answer := s.answer
	s.mu.Unlock()
	if answer == nil {
		return
	}
	questions := make([]RequestUserInputQuestion, 0, len(ask.Questions))
	for _, q := range ask.Questions {
		opts := make([]RequestUserInputOption, len(q.Options))
		for i, option := range q.Options {
			opts[i] = RequestUserInputOption{Label: option.Label, Description: option.Description}
		}
		questions = append(questions, RequestUserInputQuestion{ID: q.ID, Header: q.Header, Question: q.Prompt, IsOther: true, Options: opts})
	}
	params := RequestUserInputParams{ThreadID: state.threadID, TurnID: state.id, ItemID: "ask-" + ask.ID, Questions: questions}
	s.service.notifyThread(state.threadID, "thread/status/changed", map[string]any{
		"threadId": state.threadID, "status": ThreadStatus{Type: "active", ActiveFlags: []string{"waitingOnUserInput"}},
	})
	raw, err := s.service.conn.Request(state.ctx, "item/tool/requestUserInput", params)
	answers := make([]event.AskAnswer, 0, len(ask.Questions))
	if err == nil && s.state() == state {
		var response RequestUserInputResponse
		if strictDecode(raw, &response) == nil && len(response.Answers) == len(ask.Questions) {
			for _, question := range ask.Questions {
				value, ok := response.Answers[question.ID]
				if !ok || len(value.Answers) == 0 {
					answers = nil
					break
				}
				answers = append(answers, event.AskAnswer{QuestionID: question.ID, Selected: append([]string(nil), value.Answers...)})
			}
		} else {
			answers = nil
		}
	} else {
		answers = nil
	}
	answer(ask.ID, answers)
	if s.state() == state {
		s.service.notifyThread(state.threadID, "thread/status/changed", map[string]any{"threadId": state.threadID, "status": ThreadStatus{Type: "active"}})
	}
}

func (s *eventSink) sessionRecovered(info control.SessionRecoveryInfo) error {
	s.mu.Lock()
	session := s.session
	s.mu.Unlock()
	if session == nil || strings.TrimSpace(info.RecoveryPath) == "" {
		return nil
	}
	session.mu.Lock()
	if session.closed || session.lease == nil {
		session.mu.Unlock()
		return fmt.Errorf("bind recovery thread: thread is closed")
	}
	oldPath, origin, id, lease := session.path, session.origin, session.id, session.lease
	session.mu.Unlock()
	newPath := strings.TrimSpace(info.RecoveryPath)
	if filepath.Dir(newPath) != filepath.Dir(origin) {
		return fmt.Errorf("bind recovery thread: recovery transcript left the session directory")
	}
	originSnapshot, err := snapshotMeta(appServerMetaPath(origin))
	if err != nil {
		return err
	}
	recoverySnapshot, err := snapshotMeta(appServerMetaPath(newPath))
	if err != nil {
		return err
	}
	if err := lease.Rebind(newPath); err != nil {
		return fmt.Errorf("bind recovery thread lease: %w", err)
	}
	meta := appServerMeta{ThreadID: id, OriginTranscript: filepath.Base(origin), ActiveTranscript: filepath.Base(newPath)}
	rollback := func(cause error) error {
		var failures []string
		if err := restoreMeta(appServerMetaPath(origin), originSnapshot); err != nil {
			failures = append(failures, "origin metadata: "+err.Error())
		}
		if err := restoreMeta(appServerMetaPath(newPath), recoverySnapshot); err != nil {
			failures = append(failures, "recovery metadata: "+err.Error())
		}
		if err := lease.Rebind(oldPath); err != nil {
			failures = append(failures, "lease: "+err.Error())
		}
		if len(failures) > 0 {
			return fmt.Errorf("%w (rollback failed: %s)", cause, strings.Join(failures, "; "))
		}
		return cause
	}
	if err := saveAppServerMeta(newPath, meta); err != nil {
		return rollback(fmt.Errorf("save recovery metadata: %w", err))
	}
	if err := saveAppServerMeta(origin, meta); err != nil {
		return rollback(fmt.Errorf("save recovery redirect: %w", err))
	}
	session.mu.Lock()
	session.path = newPath
	session.mu.Unlock()
	return nil
}

func transcriptTurns(threadID string, messages []control.TranscriptMessage) []Turn {
	type builder struct {
		turn        Turn
		tools       map[string]ThreadItem
		interrupted bool
		itemSeq     int
	}
	var current *builder
	var out []Turn
	flush := func() {
		if current == nil {
			return
		}
		if current.interrupted {
			current.turn.Status = "interrupted"
		}
		out = append(out, current.turn)
		current = nil
	}
	ordinal := 0
	for _, message := range messages {
		if message.Hidden {
			continue
		}
		if message.Role == control.TranscriptUser && message.SteerText == "" {
			flush()
			ordinal++
			turnID := stableID(threadID, "turn", strconvItoa(ordinal))
			itemID := stableID(turnID, "user")
			current = &builder{turn: Turn{ID: turnID, Items: []ThreadItem{{
				"type": "userMessage", "id": itemID, "clientId": nil,
				"content": []any{map[string]any{"type": "text", "text": message.Content, "textElements": []any{}}},
			}}, ItemsView: "full", Status: "completed"}, tools: make(map[string]ThreadItem)}
			continue
		}
		if current == nil {
			continue
		}
		if message.Interrupted {
			current.interrupted = true
		}
		if message.SteerText != "" {
			current.itemSeq++
			id := stableID(current.turn.ID, "steer", strconvItoa(current.itemSeq))
			current.turn.Items = append(current.turn.Items, ThreadItem{"type": "userMessage", "id": id, "clientId": nil, "content": []any{map[string]any{"type": "text", "text": message.SteerText, "textElements": []any{}}}})
			continue
		}
		switch message.Role {
		case control.TranscriptAssistant:
			if message.Reasoning != "" {
				current.itemSeq++
				id := stableID(current.turn.ID, "reasoning", strconvItoa(current.itemSeq))
				current.turn.Items = append(current.turn.Items, ThreadItem{"type": "reasoning", "id": id, "summary": []string{}, "content": []string{message.Reasoning}})
			}
			if message.Content != "" {
				current.itemSeq++
				id := stableID(current.turn.ID, "agent", strconvItoa(current.itemSeq))
				current.turn.Items = append(current.turn.Items, ThreadItem{"type": "agentMessage", "id": id, "text": message.Content, "phase": nil, "memoryCitation": nil})
			}
			for _, call := range message.ToolCalls {
				var args any = map[string]any{}
				if json.Unmarshal([]byte(call.Arguments), &args) != nil {
					args = map[string]any{"raw": call.Arguments}
				}
				item := ThreadItem{"type": "dynamicToolCall", "id": call.ID, "namespace": nil, "tool": call.Name, "arguments": args, "status": "inProgress", "contentItems": nil, "success": nil, "durationMs": nil}
				current.tools[call.ID] = item
				current.turn.Items = append(current.turn.Items, item)
			}
		case control.TranscriptTool:
			item := current.tools[message.ToolCallID]
			if item == nil {
				item = ThreadItem{"type": "dynamicToolCall", "id": message.ToolCallID, "namespace": nil, "tool": message.ToolName, "arguments": map[string]any{}}
				current.turn.Items = append(current.turn.Items, item)
			}
			item["status"] = "completed"
			item["success"] = true
			item["contentItems"] = []any{map[string]any{"type": "inputText", "text": message.Content}}
		}
	}
	flush()
	return out
}

func nextTurnID(threadID string, messages []control.TranscriptMessage) string {
	return stableID(threadID, "turn", strconvItoa(len(transcriptTurns(threadID, messages))+1))
}

func previewFromTranscript(messages []control.TranscriptMessage) string {
	for _, message := range messages {
		if !message.Hidden && message.Role == control.TranscriptUser && message.SteerText == "" {
			return message.Content
		}
	}
	return ""
}

func cloneItem(item ThreadItem) ThreadItem {
	if item == nil {
		return nil
	}
	raw, _ := json.Marshal(item)
	var out ThreadItem
	_ = json.Unmarshal(raw, &out)
	return out
}

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}
func isFileApprovalTool(tool string) bool {
	switch tool {
	case "edit_file", "write_file", "move_file", "multiedit":
		return true
	}
	return false
}
func approvalAllowsSession(tool string) bool {
	return !control.RequiresFreshHumanApprovalTool(tool) || tool == control.SandboxEscapeApprovalTool
}
func strconvItoa(value int) string { return fmt.Sprintf("%d", value) }
