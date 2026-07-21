package appserver

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"reames-agent/internal/control"
	"reames-agent/internal/event"
)

type ThreadParams struct {
	Cwd                string
	Model              string
	Sink               event.Sink
	OnSessionRecovered func(control.SessionRecoveryInfo) error
}

type ThreadRuntime struct {
	Controller    Controller
	Model         string
	ModelProvider string
	Cwd           string
	Sandbox       SandboxPolicy
}

// Factory is the App-Server composition boundary. Production reuses boot.Build;
// tests can provide a controller double without changing transport semantics.
type Factory interface {
	NewThread(context.Context, ThreadParams) (ThreadRuntime, error)
	SessionDir() string
}

// Controller is the narrow control-layer port used by App-Server. It keeps the
// transport independent of provider, agent, and persistence implementations.
type Controller interface {
	RunTurn(context.Context, string) error
	Cancel()
	TrySteer(string) bool
	Transcript() []control.TranscriptMessage
	Approve(string, bool, bool, bool)
	AnswerQuestion(string, []event.AskAnswer)
	EnableInteractiveApproval()
	Snapshot() error
	SnapshotForShutdown() error
	SessionPath() string
	SessionDir() string
	WorkspaceRoot() string
	ResumeSessionPath(string, func() error) error
	Close()
	EnsureSessionPath()
}

type ServerInfo struct {
	Name    string
	Version string
	Home    string
}

func Serve(ctx context.Context, r io.Reader, w io.Writer, factory Factory, info ServerInfo) error {
	conn := NewConn(r, w)
	svc := &service{
		conn: conn, factory: factory, info: info,
		threads: make(map[string]*threadSession), optOut: make(map[string]struct{}),
	}
	conn.Handle("initialize", svc.initialize)
	conn.HandleNotify("initialized", svc.initializedNotification)
	for method, handler := range map[string]requestHandler{
		"thread/start":       svc.threadStart,
		"thread/resume":      svc.threadResume,
		"thread/list":        svc.threadList,
		"thread/loaded/list": svc.threadLoadedList,
		"thread/read":        svc.threadRead,
		"thread/name/set":    svc.threadNameSet,
		"thread/unsubscribe": svc.threadUnsubscribe,
		"turn/start":         svc.turnStart,
		"turn/steer":         svc.turnSteer,
		"turn/interrupt":     svc.turnInterrupt,
	} {
		conn.Handle(method, svc.requireInitialized(method, handler))
	}
	defer svc.closeAll()
	return conn.Serve(ctx)
}

type service struct {
	conn    *Conn
	factory Factory
	info    ServerInfo

	mu          sync.Mutex
	initialized bool
	optOut      map[string]struct{}
	threads     map[string]*threadSession
}

type threadSession struct {
	id       string
	ctrl     Controller
	sink     *eventSink
	lease    *control.SessionLeaseKeeper
	path     string
	origin   string
	cwd      string
	model    string
	provider string
	sandbox  SandboxPolicy

	mu         sync.Mutex
	active     *turnState
	subscribed bool
	closing    bool
	closed     bool
}

func (s *threadSession) abortAndWait() {
	s.mu.Lock()
	active := s.active
	s.mu.Unlock()
	if active == nil {
		return
	}
	active.cancel()
	s.ctrl.Cancel()
	<-active.done
}

func (s *threadSession) close() {
	s.abortAndWait()
	s.mu.Lock()
	if s.closing || s.closed {
		s.mu.Unlock()
		return
	}
	s.closing = true
	s.mu.Unlock()
	_ = s.ctrl.SnapshotForShutdown()
	s.cleanupUnpersistedMetadata()
	if s.lease != nil {
		s.lease.Release()
	}
	s.ctrl.Close()
	s.mu.Lock()
	s.closing = false
	s.closed = true
	s.mu.Unlock()
}

func (s *threadSession) cleanupUnpersistedMetadata() {
	s.mu.Lock()
	paths := []string{s.origin, s.path}
	s.mu.Unlock()
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		clean := filepath.Clean(path)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		if _, err := os.Stat(clean); !os.IsNotExist(err) {
			continue
		}
		if err := os.Remove(appServerMetaPath(clean)); err != nil && !os.IsNotExist(err) {
			continue
		}
	}
}

func (s *service) requireInitialized(method string, next requestHandler) requestHandler {
	return func(ctx context.Context, raw json.RawMessage) (any, error) {
		s.mu.Lock()
		ready := s.initialized
		s.mu.Unlock()
		if !ready {
			return nil, &RPCError{Code: ErrInvalidRequest, Message: method + ": Not initialized"}
		}
		return next(ctx, raw)
	}
}

func (s *service) initialize(_ context.Context, raw json.RawMessage) (any, error) {
	var p InitializeParams
	if err := strictDecode(raw, &p); err != nil {
		return nil, invalidParams("initialize", err)
	}
	if strings.TrimSpace(p.ClientInfo.Name) == "" || strings.TrimSpace(p.ClientInfo.Version) == "" {
		return nil, invalidParams("initialize", errors.New("clientInfo.name and clientInfo.version are required"))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.initialized {
		return nil, &RPCError{Code: ErrInvalidRequest, Message: "initialize: Already initialized"}
	}
	s.initialized = true
	if p.Capabilities != nil {
		for _, method := range p.Capabilities.OptOutNotificationMethods {
			method = strings.TrimSpace(method)
			if method != "" {
				s.optOut[method] = struct{}{}
			}
		}
	}
	family := "unix"
	if runtime.GOOS == "windows" {
		family = "windows"
	}
	name := strings.TrimSpace(s.info.Name)
	if name == "" {
		name = "reames-agent"
	}
	return InitializeResponse{
		UserAgent: name + "/" + s.info.Version, CodexHome: s.info.Home,
		PlatformFamily: family, PlatformOS: runtime.GOOS,
	}, nil
}

func (s *service) initializedNotification(_ context.Context, _ json.RawMessage) {
	// Codex marks the connection ready when initialize succeeds. The follow-up
	// notification is an acknowledgement and intentionally has no state effect.
}

func (s *service) threadStart(ctx context.Context, raw json.RawMessage) (any, error) {
	var p ThreadStartParams
	if err := strictDecode(raw, &p); err != nil {
		return nil, invalidParams("thread/start", err)
	}
	// Validate the wire enum before building a controller or tolerating any
	// history content. Unknown modes must fail before mutation.
	if err := validateHistoryMode(p.HistoryMode); err != nil {
		return nil, invalidParams("thread/start", err)
	}
	if p.Ephemeral {
		return nil, invalidParams("thread/start", errors.New("ephemeral threads are not supported"))
	}

	sink := newEventSink(s)
	rt, err := s.factory.NewThread(ctx, ThreadParams{Cwd: p.Cwd, Model: p.Model, Sink: sink, OnSessionRecovered: sink.sessionRecovered})
	if err != nil {
		return nil, &RPCError{Code: ErrInternal, Message: "thread/start: " + err.Error()}
	}
	if rt.Controller == nil {
		return nil, &RPCError{Code: ErrInternal, Message: "thread/start: factory returned no controller"}
	}
	ctrl := rt.Controller
	ctrl.EnableInteractiveApproval()
	ctrl.EnsureSessionPath()
	path := strings.TrimSpace(ctrl.SessionPath())
	if path == "" {
		ctrl.Close()
		return nil, &RPCError{Code: ErrInternal, Message: "thread/start: persistence is unavailable"}
	}
	id := control.BranchID(path)
	if id == "" {
		ctrl.Close()
		return nil, &RPCError{Code: ErrInternal, Message: "thread/start: could not derive thread identity"}
	}
	lease := control.NewSessionLeaseKeeper()
	if err := lease.Rebind(path); err != nil {
		ctrl.Close()
		return nil, sessionLeaseError("thread/start", err)
	}
	cwd := firstNonEmpty(rt.Cwd, ctrl.WorkspaceRoot(), p.Cwd)
	if cwd == "" {
		cwd, _ = filepath.Abs(".")
	}
	provider := firstNonEmpty(rt.ModelProvider, modelProvider(rt.Model), modelProvider(p.Model))
	sess := &threadSession{
		id: id, ctrl: ctrl, sink: sink, lease: lease, path: path, cwd: cwd,
		origin: path, model: firstNonEmpty(rt.Model, p.Model), provider: provider,
		sandbox: conservativeSandbox(rt.Sandbox), subscribed: true,
	}
	sink.bind(sess)
	sink.bindApproval(ctrl.Approve, ctrl.AnswerQuestion)
	meta := appServerMeta{ThreadID: id, OriginTranscript: filepath.Base(path), ActiveTranscript: filepath.Base(path)}
	if err := saveAppServerMeta(path, meta); err != nil {
		sess.close()
		return nil, &RPCError{Code: ErrInternal, Message: "thread/start: save App-Server metadata: " + err.Error()}
	}

	s.mu.Lock()
	if _, exists := s.threads[id]; exists {
		s.mu.Unlock()
		sess.close()
		return nil, &RPCError{Code: ErrInvalidRequest, Message: "thread/start: thread identity collision"}
	}
	s.threads[id] = sess
	s.mu.Unlock()

	thread := s.threadObject(sess, false)
	response := s.threadOpenResponse(sess, thread)
	return deferredResponse{value: response, after: func() {
		s.notifyThread(id, "thread/started", map[string]any{"thread": thread})
	}}, nil
}

func (s *service) threadResume(ctx context.Context, raw json.RawMessage) (any, error) {
	var p ThreadResumeParams
	if err := strictDecode(raw, &p); err != nil {
		return nil, invalidParams("thread/resume", err)
	}
	id := strings.TrimSpace(p.ThreadID)
	if id == "" {
		return nil, invalidParams("thread/resume", errors.New("threadId is required"))
	}
	if existing := s.thread(id); existing != nil {
		existing.mu.Lock()
		modelMismatch := strings.TrimSpace(p.Model) != "" && strings.TrimSpace(p.Model) != existing.model
		cwdMismatch := strings.TrimSpace(p.Cwd) != "" && filepath.Clean(p.Cwd) != filepath.Clean(existing.cwd)
		if modelMismatch || cwdMismatch {
			existing.mu.Unlock()
			return nil, invalidParams("thread/resume", errors.New("model or cwd override is not supported for an already loaded thread"))
		}
		existing.subscribed = true
		existing.mu.Unlock()
		return s.threadOpenResponse(existing, s.threadObject(existing, true)), nil
	}
	path, meta, err := s.resolveThread(id)
	if err != nil {
		return nil, &RPCError{Code: ErrInvalidParams, Message: "thread/resume: unknown thread " + id}
	}
	cwd := firstNonEmpty(p.Cwd, meta.WorkspaceRoot)
	sink := newEventSink(s)
	rt, err := s.factory.NewThread(ctx, ThreadParams{Cwd: cwd, Model: p.Model, Sink: sink, OnSessionRecovered: sink.sessionRecovered})
	if err != nil {
		return nil, &RPCError{Code: ErrInternal, Message: "thread/resume: " + err.Error()}
	}
	ctrl := rt.Controller
	if ctrl == nil {
		return nil, &RPCError{Code: ErrInternal, Message: "thread/resume: factory returned no controller"}
	}
	ctrl.EnableInteractiveApproval()
	lease := control.NewSessionLeaseKeeper()
	if err := ctrl.ResumeSessionPath(path, func() error { return lease.Rebind(path) }); err != nil {
		lease.Release()
		ctrl.Close()
		if control.IsSessionLeaseHeld(err) {
			return nil, sessionLeaseError("thread/resume", err)
		}
		return nil, &RPCError{Code: ErrInvalidParams, Message: "thread/resume: unknown thread " + id}
	}
	provider := firstNonEmpty(rt.ModelProvider, modelProvider(rt.Model), modelProvider(meta.Model))
	appMeta, hasAppMeta, metaErr := loadAppServerMeta(path)
	if metaErr != nil {
		lease.Release()
		ctrl.Close()
		return nil, &RPCError{Code: ErrInternal, Message: "thread/resume: " + metaErr.Error()}
	}
	origin := path
	if hasAppMeta {
		origin = filepath.Join(filepath.Dir(path), appMeta.OriginTranscript)
	}
	sess := &threadSession{
		id: id, ctrl: ctrl, sink: sink, lease: lease, path: path,
		origin: origin, cwd: firstNonEmpty(rt.Cwd, ctrl.WorkspaceRoot(), cwd), model: firstNonEmpty(rt.Model, p.Model, meta.Model),
		provider: provider, sandbox: conservativeSandbox(rt.Sandbox), subscribed: true,
	}
	sink.bind(sess)
	sink.bindApproval(ctrl.Approve, ctrl.AnswerQuestion)
	if !hasAppMeta {
		appMeta = appServerMeta{ThreadID: id, OriginTranscript: filepath.Base(path), ActiveTranscript: filepath.Base(path)}
		if err := saveAppServerMeta(path, appMeta); err != nil {
			sess.close()
			return nil, &RPCError{Code: ErrInternal, Message: "thread/resume: save App-Server metadata: " + err.Error()}
		}
	}
	s.mu.Lock()
	if current := s.threads[id]; current != nil {
		s.mu.Unlock()
		sess.close()
		current.mu.Lock()
		current.subscribed = true
		current.mu.Unlock()
		return s.threadOpenResponse(current, s.threadObject(current, true)), nil
	}
	s.threads[id] = sess
	s.mu.Unlock()
	return s.threadOpenResponse(sess, s.threadObject(sess, true)), nil
}

func (s *service) threadList(_ context.Context, raw json.RawMessage) (any, error) {
	var p ThreadListParams
	if err := decodeThreadListParams(raw, &p); err != nil {
		return nil, invalidParams("thread/list", err)
	}
	if p.Limit < 0 || p.Limit > 100 {
		return nil, invalidParams("thread/list", errors.New("limit must be between 1 and 100"))
	}
	if p.SortKey != "" && p.SortKey != "created_at" && p.SortKey != "updated_at" && p.SortKey != "recency_at" {
		return nil, invalidParams("thread/list", fmt.Errorf("unsupported sortKey %q", p.SortKey))
	}
	if p.SortDirection != "" && p.SortDirection != "asc" && p.SortDirection != "desc" {
		return nil, invalidParams("thread/list", fmt.Errorf("unsupported sortDirection %q", p.SortDirection))
	}
	records, err := s.sessionRecords()
	if err != nil {
		return nil, &RPCError{Code: ErrInternal, Message: "thread/list: " + err.Error()}
	}
	filtered := make([]threadRecord, 0, len(records))
	for _, record := range records {
		if !matchesCwd(record.info.WorkspaceRoot, p.Cwd) || !matchesSearch(record.info, p.SearchTerm) {
			continue
		}
		filtered = append(filtered, record)
	}
	sortKey := p.SortKey
	if sortKey == "" {
		sortKey = "created_at"
	}
	desc := p.SortDirection != "asc"
	sort.SliceStable(filtered, func(i, j int) bool {
		a, b := sessionSortTime(filtered[i].info, sortKey), sessionSortTime(filtered[j].info, sortKey)
		if a.Equal(b) {
			if desc {
				return filtered[i].info.Path > filtered[j].info.Path
			}
			return filtered[i].info.Path < filtered[j].info.Path
		}
		if desc {
			return a.After(b)
		}
		return a.Before(b)
	})
	start, err := decodeCursor(p.Cursor)
	if err != nil || start > len(filtered) {
		return nil, invalidParams("thread/list", errors.New("invalid cursor"))
	}
	limit := p.Limit
	if limit == 0 {
		limit = 20
	}
	end := start + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	data := make([]Thread, 0, end-start)
	for _, record := range filtered[start:end] {
		data = append(data, s.threadObjectFromRecord(record, false))
	}
	var next, backwards *string
	if end < len(filtered) {
		value := encodeCursor(end)
		next = &value
	}
	if len(data) > 0 {
		back := start - limit
		if back < 0 {
			back = 0
		}
		value := encodeCursor(back)
		backwards = &value
	}
	return ThreadListResponse{Data: data, NextCursor: next, BackwardsCursor: backwards}, nil
}

func (s *service) threadLoadedList(_ context.Context, raw json.RawMessage) (any, error) {
	var p ThreadLoadedListParams
	if err := strictDecode(raw, &p); err != nil {
		return nil, invalidParams("thread/loaded/list", err)
	}
	if p.Limit < 0 || p.Limit > 100 {
		return nil, invalidParams("thread/loaded/list", errors.New("limit must be between 1 and 100"))
	}
	s.mu.Lock()
	ids := make([]string, 0, len(s.threads))
	for id := range s.threads {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	sort.Strings(ids)
	start, err := decodeCursor(p.Cursor)
	if err != nil || start > len(ids) {
		return nil, invalidParams("thread/loaded/list", errors.New("invalid cursor"))
	}
	limit := p.Limit
	if limit == 0 {
		limit = 100
	}
	end := start + limit
	if end > len(ids) {
		end = len(ids)
	}
	var next *string
	if end < len(ids) {
		value := encodeCursor(end)
		next = &value
	}
	return ThreadLoadedListResponse{Data: append([]string(nil), ids[start:end]...), NextCursor: next}, nil
}

func (s *service) threadRead(_ context.Context, raw json.RawMessage) (any, error) {
	var p ThreadReadParams
	if err := strictDecode(raw, &p); err != nil {
		return nil, invalidParams("thread/read", err)
	}
	id := strings.TrimSpace(p.ThreadID)
	if id == "" {
		return nil, invalidParams("thread/read", errors.New("threadId is required"))
	}
	if sess := s.thread(id); sess != nil {
		return ThreadReadResponse{Thread: s.threadObject(sess, p.IncludeTurns)}, nil
	}
	_, info, err := s.resolveThreadInfo(id)
	if err != nil {
		return nil, &RPCError{Code: ErrInvalidParams, Message: "thread/read: unknown thread " + id}
	}
	return ThreadReadResponse{Thread: s.threadObjectFromInfo(info, p.IncludeTurns)}, nil
}

func (s *service) threadNameSet(_ context.Context, raw json.RawMessage) (any, error) {
	var p ThreadNameSetParams
	if err := strictDecode(raw, &p); err != nil {
		return nil, invalidParams("thread/name/set", err)
	}
	id := strings.TrimSpace(p.ThreadID)
	if id == "" {
		return nil, invalidParams("thread/name/set", errors.New("threadId is required"))
	}
	path := ""
	if loaded := s.thread(id); loaded != nil {
		path = loaded.path
	} else {
		var err error
		path, _, err = s.resolveThreadInfo(id)
		if err != nil {
			return nil, &RPCError{Code: ErrInvalidParams, Message: "thread/name/set: unknown thread " + id}
		}
	}
	// RenameSession is the canonical metadata writer. Resolve and validate it
	// before mutation; there is no transcript-text fallback writer.
	if err := control.RenameSession(path, strings.TrimSpace(p.Name)); err != nil {
		return nil, &RPCError{Code: ErrInternal, Message: "thread/name/set: " + err.Error()}
	}
	name := strings.TrimSpace(p.Name)
	return deferredResponse{value: struct{}{}, after: func() {
		s.notify("thread/name/updated", map[string]any{"threadId": id, "name": name})
	}}, nil
}

func (s *service) threadUnsubscribe(_ context.Context, raw json.RawMessage) (any, error) {
	var p ThreadUnsubscribeParams
	if err := strictDecode(raw, &p); err != nil {
		return nil, invalidParams("thread/unsubscribe", err)
	}
	id := strings.TrimSpace(p.ThreadID)
	if id == "" {
		return nil, invalidParams("thread/unsubscribe", errors.New("threadId is required"))
	}
	sess := s.thread(id)
	if sess == nil {
		return ThreadUnsubscribeResponse{Status: "notLoaded"}, nil
	}
	sess.mu.Lock()
	status := "notSubscribed"
	if sess.subscribed {
		sess.subscribed = false
		status = "unsubscribed"
	}
	sess.mu.Unlock()
	return ThreadUnsubscribeResponse{Status: status}, nil
}

func (s *service) turnStart(ctx context.Context, raw json.RawMessage) (any, error) {
	var p TurnStartParams
	if err := strictDecode(raw, &p); err != nil {
		return nil, invalidParams("turn/start", err)
	}
	id := strings.TrimSpace(p.ThreadID)
	if id == "" {
		return nil, invalidParams("turn/start", errors.New("threadId is required"))
	}
	text, err := flattenTextInput(p.Input)
	if err != nil {
		return nil, invalidParams("turn/start", err)
	}
	sess := s.thread(id)
	if sess == nil {
		return nil, &RPCError{Code: ErrInvalidParams, Message: "turn/start: unknown thread " + p.ThreadID}
	}
	runCtx, cancel := context.WithCancel(ctx)
	turnID := nextTurnID(sess.id, sess.ctrl.Transcript())
	state := newTurnState(sess.id, turnID, p.ClientUserMessageID, text, cancel)
	state.setContext(runCtx)
	sess.mu.Lock()
	if sess.closed || sess.active != nil {
		sess.mu.Unlock()
		cancel()
		return nil, &RPCError{Code: ErrInvalidRequest, Message: "turn/start: thread already has an active turn"}
	}
	sess.active = state
	sess.subscribed = true
	sess.mu.Unlock()
	sess.sink.begin(state)
	initial := state.turn("inProgress", nil)
	return deferredResponse{value: TurnStartResponse{Turn: initial}, after: func() {
		go s.runTurn(runCtx, sess, state, text)
	}}, nil
}

func (s *service) runTurn(ctx context.Context, sess *threadSession, state *turnState, text string) {
	s.notifyThread(state.threadID, "thread/status/changed", map[string]any{"threadId": state.threadID, "status": ThreadStatus{Type: "active"}})
	s.notifyThread(state.threadID, "turn/started", map[string]any{"threadId": state.threadID, "turn": state.turn("inProgress", nil)})
	state.emitUserLifecycle(s)
	runErr := sess.ctrl.RunTurn(ctx, text)
	if snapshotErr := sess.ctrl.Snapshot(); snapshotErr != nil && runErr == nil {
		runErr = fmt.Errorf("persist turn: %w", snapshotErr)
	}
	status := "completed"
	var turnErr *TurnError
	if ctx.Err() != nil {
		status = "interrupted"
	} else if runErr != nil {
		status = "failed"
		turnErr = &TurnError{Message: runErr.Error()}
	}
	sess.sink.finish(state)

	sess.mu.Lock()
	matched := sess.active == state && sess.id == state.threadID
	if matched {
		sess.active = nil
	}
	sess.mu.Unlock()
	if matched {
		turn := state.turn(status, turnErr)
		s.notifyThread(state.threadID, "turn/completed", map[string]any{"threadId": state.threadID, "turn": turn})
		s.notifyThread(state.threadID, "thread/status/changed", map[string]any{"threadId": state.threadID, "status": ThreadStatus{Type: "idle"}})
	}
	close(state.done)
}

func (s *service) turnSteer(_ context.Context, raw json.RawMessage) (any, error) {
	var p TurnSteerParams
	if err := strictDecode(raw, &p); err != nil {
		return nil, invalidParams("turn/steer", err)
	}
	id := strings.TrimSpace(p.ThreadID)
	turnID := strings.TrimSpace(p.ExpectedTurnID)
	if id == "" || turnID == "" {
		return nil, invalidParams("turn/steer", errors.New("threadId and expectedTurnId are required"))
	}
	text, err := flattenTextInput(p.Input)
	if err != nil {
		return nil, invalidParams("turn/steer", err)
	}
	sess := s.thread(id)
	if sess == nil {
		return nil, &RPCError{Code: ErrInvalidParams, Message: "turn/steer: unknown thread " + p.ThreadID}
	}
	sess.mu.Lock()
	active := sess.active
	matched := active != nil && active.threadID == id && active.id == turnID
	sess.mu.Unlock()
	if !matched {
		return nil, &RPCError{Code: ErrInvalidRequest, Message: "turn/steer: active thread/turn identity does not match"}
	}
	if !sess.ctrl.TrySteer(text) {
		return nil, &RPCError{Code: ErrInvalidRequest, Message: "turn/steer: active turn cannot accept guidance"}
	}
	return TurnSteerResponse{TurnID: active.id}, nil
}

func (s *service) turnInterrupt(_ context.Context, raw json.RawMessage) (any, error) {
	var p TurnInterruptParams
	if err := strictDecode(raw, &p); err != nil {
		return nil, invalidParams("turn/interrupt", err)
	}
	id := strings.TrimSpace(p.ThreadID)
	turnID := strings.TrimSpace(p.TurnID)
	if id == "" || turnID == "" {
		return nil, invalidParams("turn/interrupt", errors.New("threadId and turnId are required"))
	}
	sess := s.thread(id)
	if sess == nil {
		return nil, &RPCError{Code: ErrInvalidParams, Message: "turn/interrupt: unknown thread " + p.ThreadID}
	}
	sess.mu.Lock()
	active := sess.active
	matched := active != nil && active.threadID == id && active.id == turnID
	sess.mu.Unlock()
	if !matched {
		return nil, &RPCError{Code: ErrInvalidRequest, Message: "turn/interrupt: active thread/turn identity does not match"}
	}
	return deferredResponse{value: TurnInterruptResponse{}, after: func() {
		active.cancel()
		sess.ctrl.Cancel()
	}}, nil
}

func (s *service) threadOpenResponse(sess *threadSession, thread Thread) ThreadStartResponse {
	return ThreadStartResponse{
		Thread: thread, Model: sess.model, ModelProvider: sess.provider, Cwd: sess.cwd,
		RuntimeWorkspaceRoots: []string{sess.cwd}, InstructionSources: []string{},
		ApprovalPolicy: "onRequest", ApprovalsReviewer: "user",
		Sandbox: sess.sandbox,
	}
}

func conservativeSandbox(policy SandboxPolicy) SandboxPolicy {
	if policy.Type == "workspaceWrite" || policy.Type == "readOnly" || policy.Type == "dangerFullAccess" {
		return policy
	}
	return SandboxPolicy{Type: "dangerFullAccess"}
}

func (s *service) threadObject(sess *threadSession, includeTurns bool) Thread {
	records, _ := s.sessionRecords()
	for _, record := range records {
		if record.id == sess.id {
			thread := s.threadObjectFromRecord(record, includeTurns)
			thread.ModelProvider = sess.provider
			thread.Cwd = sess.cwd
			thread.Source = "appServer"
			thread.Status = sess.status()
			accept := thread.Status.Type != "active"
			thread.CanAcceptDirectInput = &accept
			if includeTurns {
				thread.Turns = transcriptTurns(sess.id, sess.ctrl.Transcript())
			}
			return thread
		}
	}
	now := time.Now().Unix()
	path := sess.path
	accept := true
	thread := Thread{
		ID: sess.id, SessionID: sess.id, Preview: previewFromTranscript(sess.ctrl.Transcript()),
		HistoryMode: "legacy", ModelProvider: sess.provider, CreatedAt: now, UpdatedAt: now,
		RecencyAt: &now, Status: sess.status(), Path: &path, Cwd: sess.cwd,
		CLIVersion: s.info.Version, Source: "appServer", CanAcceptDirectInput: &accept,
		Turns: []Turn{},
	}
	if meta, ok, _ := control.LoadSessionMeta(sess.path); ok {
		thread.Name = stringPtrIf(meta.CustomTitle)
	}
	if thread.Status.Type == "active" {
		accept = false
		thread.CanAcceptDirectInput = &accept
	}
	if includeTurns {
		thread.Turns = transcriptTurns(sess.id, sess.ctrl.Transcript())
	}
	return thread
}

func (sess *threadSession) status() ThreadStatus {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	if sess.active != nil {
		return ThreadStatus{Type: "active"}
	}
	return ThreadStatus{Type: "idle"}
}

func (s *service) threadObjectFromInfo(info control.SessionInfo, includeTurns bool) Thread {
	id := s.threadIDForPath(info.Path)
	return s.threadObjectFromRecord(threadRecord{id: id, info: info}, includeTurns)
}

func (s *service) threadObjectFromRecord(record threadRecord, includeTurns bool) Thread {
	info, id := record.info, record.id
	path := info.Path
	updated := firstTime(info.LastActivityAt, info.ModTime, info.CreatedAt).Unix()
	created := firstTime(info.CreatedAt, info.ModTime).Unix()
	name := stringPtrIf(info.CustomTitle)
	parent := stringPtrIf(info.ParentID)
	provider := ""
	if meta, ok, _ := control.LoadSessionMeta(info.Path); ok {
		provider = modelProvider(meta.Model)
	}
	thread := Thread{
		ID: id, SessionID: id, ParentThreadID: parent, Preview: info.Preview,
		HistoryMode: "legacy", ModelProvider: provider, CreatedAt: created, UpdatedAt: updated,
		RecencyAt: &updated, Status: ThreadStatus{Type: "notLoaded"}, Path: &path,
		Cwd: info.WorkspaceRoot, CLIVersion: s.info.Version, Source: "unknown", Name: name,
		Turns: []Turn{},
	}
	if loaded := s.thread(id); loaded != nil {
		thread.Status = loaded.status()
		thread.Source = "appServer"
		thread.ModelProvider = loaded.provider
		thread.Cwd = loaded.cwd
		accept := thread.Status.Type != "active"
		thread.CanAcceptDirectInput = &accept
	}
	if includeTurns {
		if loaded := s.thread(id); loaded != nil {
			thread.Turns = transcriptTurns(id, loaded.ctrl.Transcript())
		} else if transcript, err := control.LoadTranscript(info.Path); err == nil {
			thread.Turns = transcriptTurns(id, transcript)
		}
	}
	return thread
}

func (s *service) resolveThread(id string) (string, control.SessionMeta, error) {
	path, info, err := s.resolveThreadInfo(id)
	if err != nil {
		return "", control.SessionMeta{}, err
	}
	meta, ok, err := control.LoadSessionMeta(path)
	if err != nil {
		return "", control.SessionMeta{}, err
	}
	if !ok {
		meta.WorkspaceRoot = info.WorkspaceRoot
	}
	return path, meta, nil
}

func (s *service) resolveThreadInfo(id string) (string, control.SessionInfo, error) {
	records, err := s.sessionRecords()
	if err != nil {
		return "", control.SessionInfo{}, err
	}
	for _, record := range records {
		if record.id == id {
			return record.info.Path, record.info, nil
		}
	}
	return "", control.SessionInfo{}, errors.New("not found")
}

type threadRecord struct {
	id   string
	info control.SessionInfo
}

func (s *service) sessionRecords() ([]threadRecord, error) {
	infos, err := control.ListSessions(s.factory.SessionDir())
	if err != nil {
		return nil, err
	}
	byPath := make(map[string]control.SessionInfo, len(infos))
	for _, info := range infos {
		byPath[filepath.Clean(info.Path)] = info
	}
	byID := make(map[string]threadRecord, len(infos))
	for _, info := range infos {
		meta, ok, err := loadAppServerMeta(info.Path)
		if err != nil {
			return nil, err
		}
		id := control.BranchID(info.Path)
		active := info
		if ok {
			id = meta.ThreadID
			activePath, safe := activePathFromMeta(filepath.Dir(info.Path), meta)
			if !safe {
				return nil, fmt.Errorf("invalid active transcript for App-Server thread %s", id)
			}
			var exists bool
			active, exists = byPath[filepath.Clean(activePath)]
			if !exists {
				return nil, fmt.Errorf("active transcript missing for App-Server thread %s", id)
			}
			targetMeta, targetOK, targetErr := loadAppServerMeta(active.Path)
			if targetErr != nil {
				return nil, targetErr
			}
			if !targetOK || targetMeta.ThreadID != id {
				return nil, fmt.Errorf("active transcript identity mismatch for App-Server thread %s", id)
			}
		}
		byID[id] = threadRecord{id: id, info: active}
	}
	out := make([]threadRecord, 0, len(byID))
	for _, record := range byID {
		out = append(out, record)
	}
	return out, nil
}

func (s *service) threadIDForPath(path string) string {
	if meta, ok, err := loadAppServerMeta(path); err == nil && ok {
		return meta.ThreadID
	}
	return control.BranchID(path)
}

func (s *service) thread(id string) *threadSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.threads[id]
}

func (s *service) notify(method string, params any) {
	s.mu.Lock()
	_, omitted := s.optOut[method]
	initialized := s.initialized
	s.mu.Unlock()
	if initialized && !omitted {
		_ = s.conn.Notify(method, params)
	}
}

func (s *service) notifyThread(id, method string, params any) {
	sess := s.thread(id)
	if sess == nil {
		return
	}
	sess.mu.Lock()
	subscribed := sess.subscribed && !sess.closed
	sess.mu.Unlock()
	if subscribed {
		s.notify(method, params)
	}
}

func (s *service) closeAll() {
	s.mu.Lock()
	threads := make([]*threadSession, 0, len(s.threads))
	for _, sess := range s.threads {
		threads = append(threads, sess)
	}
	s.threads = make(map[string]*threadSession)
	s.mu.Unlock()
	for _, sess := range threads {
		sess.close()
	}
}

type deferredResponse struct {
	value any
	after func()
}

func (r deferredResponse) ResponseValue() any { return r.value }
func (r deferredResponse) AfterResponse() {
	if r.after != nil {
		r.after()
	}
}

func invalidParams(method string, err error) *RPCError {
	return &RPCError{Code: ErrInvalidParams, Message: method + ": " + err.Error()}
}

func sessionLeaseError(method string, err error) *RPCError {
	if control.IsSessionLeaseHeld(err) {
		return &RPCError{Code: ErrInvalidRequest, Message: method + ": " + control.SessionInUseMessage(err) + "; " + control.SessionLeaseCloseHint}
	}
	return &RPCError{Code: ErrInternal, Message: method + ": session lease: " + err.Error()}
}

func modelProvider(model string) string {
	model = strings.TrimSpace(model)
	if provider, _, ok := strings.Cut(model, "/"); ok {
		return provider
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value
		}
	}
	return time.Unix(0, 0)
}

func stringPtrIf(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func matchesCwd(cwd string, filters []string) bool {
	if len(filters) == 0 {
		return true
	}
	for _, filter := range filters {
		if filepath.Clean(cwd) == filepath.Clean(filter) {
			return true
		}
	}
	return false
}

func matchesSearch(info control.SessionInfo, search string) bool {
	search = strings.ToLower(strings.TrimSpace(search))
	if search == "" {
		return true
	}
	return strings.Contains(strings.ToLower(info.CustomTitle), search) || strings.Contains(strings.ToLower(info.Preview), search)
}

func sessionSortTime(info control.SessionInfo, key string) time.Time {
	switch key {
	case "updated_at", "recency_at":
		return firstTime(info.LastActivityAt, info.ModTime, info.CreatedAt)
	default:
		return firstTime(info.CreatedAt, info.ModTime)
	}
}

func encodeCursor(offset int) string {
	return base64.RawURLEncoding.EncodeToString([]byte("v1:" + strconv.Itoa(offset)))
}

func decodeCursor(cursor string) (int, error) {
	if cursor == "" {
		return 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil || !strings.HasPrefix(string(raw), "v1:") {
		return 0, errors.New("invalid cursor")
	}
	offset, err := strconv.Atoi(strings.TrimPrefix(string(raw), "v1:"))
	if err != nil || offset < 0 {
		return 0, errors.New("invalid cursor")
	}
	return offset, nil
}

func decodeThreadListParams(raw json.RawMessage, dst *ThreadListParams) error {
	var fields map[string]json.RawMessage
	if len(strings.TrimSpace(string(raw))) == 0 {
		raw = json.RawMessage("{}")
	}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	allowed := map[string]struct{}{"cursor": {}, "limit": {}, "sortKey": {}, "sortDirection": {}, "cwd": {}, "searchTerm": {}}
	for key := range fields {
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("json: unknown field %q", key)
		}
	}
	return json.Unmarshal(raw, dst)
}

func stableID(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		_, _ = h.Write([]byte(part))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)[:16])
}
