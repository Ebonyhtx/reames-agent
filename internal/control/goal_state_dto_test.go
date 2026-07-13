package control

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"reames-agent/internal/agent"
	"reames-agent/internal/event"
	"reames-agent/internal/evidence"
	"reames-agent/internal/instruction"
	"reames-agent/internal/provider"
)

func TestGoalStateV2RoundTrip(t *testing.T) {
	checkHash := evidence.ProjectCheckHash("go test ./...")
	original := GoalStateV2{
		Version:            GoalStateVersion,
		Goal:               "fix the build",
		Status:             GoalStatusRunning,
		ResearchMode:       GoalResearchOn,
		AutoResearchTaskID: "task-1",
		Turns:              5,
		Blocks:             0,
		Strict:             true,
		Todos:              []evidence.TodoItem{{Content: "verify persistence", Status: "completed"}},
		MessageCount:       4,
		TranscriptDigest:   "abc123",
		DurableEvidence: &evidence.DurableState{
			WritePending: true,
			VerifiedChecks: []evidence.VerificationReference{{
				CheckHash: checkHash, ToolCallID: "bash-1",
			}},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}

	var restored GoalStateV2
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}

	if restored.Version != GoalStateVersion {
		t.Fatalf("Version = %d, want %d", restored.Version, GoalStateVersion)
	}
	if restored.Goal != "fix the build" {
		t.Fatalf("Goal = %q", restored.Goal)
	}
	if restored.Status != GoalStatusRunning {
		t.Fatalf("Status = %q", restored.Status)
	}
	if restored.Strict != true {
		t.Fatal("Strict should be true")
	}
	if len(restored.Todos) != 1 || restored.Todos[0].Content != "verify persistence" {
		t.Fatalf("Todos = %+v, want persisted todo", restored.Todos)
	}
	if restored.MessageCount != 4 || restored.TranscriptDigest != "abc123" {
		t.Fatalf("transcript anchor = count %d digest %q", restored.MessageCount, restored.TranscriptDigest)
	}
	if restored.DurableEvidence == nil || !restored.DurableEvidence.WritePending || len(restored.DurableEvidence.VerifiedChecks) != 1 {
		t.Fatalf("DurableEvidence = %+v", restored.DurableEvidence)
	}
	if strings.Contains(string(data), "go test ./...") {
		t.Fatalf("runtime sidecar exposed project-check command text: %s", data)
	}
}

func TestGoalStateDurableEvidenceRoundTripDeepCopies(t *testing.T) {
	state := &evidence.DurableState{WritePending: true, VerifiedChecks: []evidence.VerificationReference{{CheckHash: "hash", ToolCallID: "call"}}}
	v2 := GoalStateV2{DurableEvidence: state}
	internal := v2.ToGoalState()
	state.VerifiedChecks[0].ToolCallID = "mutated-source"
	if got := internal.DurableEvidence.VerifiedChecks[0].ToolCallID; got != "call" {
		t.Fatalf("ToGoalState retained source alias: %q", got)
	}

	back := FromGoalState(internal)
	internal.DurableEvidence.VerifiedChecks[0].ToolCallID = "mutated-internal"
	if got := back.DurableEvidence.VerifiedChecks[0].ToolCallID; got != "call" {
		t.Fatalf("FromGoalState retained internal alias: %q", got)
	}
}

func TestRestoreRuntimeEvidenceRequiresExactTranscriptAnchor(t *testing.T) {
	check := instruction.VerifyCheck{Command: "go test ./...", SourcePath: "AGENTS.md"}
	session := agent.NewSession("sys")
	session.Add(provider.Message{Role: provider.RoleUser, Content: "verify"})
	session.Add(provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "bash-1", Name: "bash", Arguments: `{"command":"go test ./..."}`}}})
	session.Add(provider.Message{Role: provider.RoleTool, ToolCallID: "bash-1", Name: "bash", Content: "ok"})
	executor := agent.New(nil, nil, session, agent.Options{ProjectChecks: []instruction.VerifyCheck{check}}, event.Discard)
	c := New(Options{Executor: executor})
	count, digest := session.TranscriptAnchor()
	state := goalState{
		MessageCount: count, TranscriptDigest: digest,
		DurableEvidence: &evidence.DurableState{WritePending: true, VerifiedChecks: []evidence.VerificationReference{{
			CheckHash: evidence.ProjectCheckHash(check.Command), ToolCallID: "bash-1",
		}}},
	}

	c.restoreRuntimeEvidence(state)
	if got := executor.DurableEvidenceState(); len(got.VerifiedChecks) != 1 {
		t.Fatalf("exact anchored evidence was not restored: %+v", got)
	}

	session.Add(provider.Message{Role: provider.RoleUser, Content: "newer suffix"})
	c.restoreRuntimeEvidence(state)
	if got := executor.DurableEvidenceState(); got.WritePending || len(got.VerifiedChecks) != 0 {
		t.Fatalf("append-only transcript accepted stale evidence: %+v", got)
	}
}

func TestDurableEvidenceSurvivesSessionCrashResume(t *testing.T) {
	dir := t.TempDir()
	sessionPath := filepath.Join(dir, "session.jsonl")
	check := instruction.VerifyCheck{Command: "go test ./...", SourcePath: "AGENTS.md"}
	hash := evidence.ProjectCheckHash(check.Command)
	refState := evidence.DurableState{WritePending: true, VerifiedChecks: []evidence.VerificationReference{{CheckHash: hash, ToolCallID: "bash-1"}}}

	session := agent.NewSession("sys")
	for _, msg := range []provider.Message{
		{Role: provider.RoleUser, Content: "implement and verify"},
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "write-1", Name: "write_file", Arguments: `{"path":"a.go","content":"package a"}`}}},
		{Role: provider.RoleTool, ToolCallID: "write-1", Name: "write_file", Content: "written"},
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "bash-1", Name: "bash", Arguments: `{"command":"go test ./..."}`}}},
		{Role: provider.RoleTool, ToolCallID: "bash-1", Name: "bash", Content: "ok"},
	} {
		session.Add(msg)
	}
	if err := session.SaveSnapshot(sessionPath); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	firstExecutor := agent.New(nil, nil, session, agent.Options{ProjectChecks: []instruction.VerifyCheck{check}}, event.Discard)
	firstExecutor.RestoreDurableEvidence(refState)
	first := New(Options{Executor: firstExecutor, SessionPath: sessionPath})
	path, data, revision, ok := first.goals.set("ship safely", GoalResearchOff, "", first.goalRuntimeProjection())
	first.persistGoalState(path, data, revision, ok)
	if !ok {
		t.Fatal("first runtime did not produce sidecar data")
	}
	sidecar, err := os.ReadFile(goalStatePath(sessionPath))
	if err != nil {
		t.Fatalf("read sidecar: %v", err)
	}
	if strings.Contains(string(sidecar), check.Command) {
		t.Fatalf("sidecar leaked project-check command: %s", sidecar)
	}

	loaded, err := agent.LoadSession(sessionPath)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	secondExecutor := agent.New(nil, nil, agent.NewSession("sys"), agent.Options{ProjectChecks: []instruction.VerifyCheck{check}}, event.Discard)
	second := New(Options{Executor: secondExecutor, SessionPath: sessionPath})
	second.Resume(loaded, sessionPath)
	if second.Goal() != "ship safely" || second.GoalStatus() != GoalStatusRunning {
		t.Fatalf("resumed goal = %q status %q", second.Goal(), second.GoalStatus())
	}
	if got := secondExecutor.DurableEvidenceState(); !got.WritePending || len(got.VerifiedChecks) != 1 || got.VerifiedChecks[0].ToolCallID != "bash-1" {
		t.Fatalf("resumed durable evidence = %+v", got)
	}
	if got := secondExecutor.GoalReadinessFailure(); got != "" {
		t.Fatalf("resumed readiness = %q, want verified writer epoch", got)
	}
}

func TestGoalStateV2ToGoalStateAndBack(t *testing.T) {
	v2 := GoalStateV2{
		Version:      GoalStateVersion,
		Goal:         "audit security",
		Status:       GoalStatusBlocked,
		ResearchMode: GoalResearchAuto,
		Blocks:       2,
		Block:        "missing API key",
		Strict:       false,
		Todos:        []evidence.TodoItem{{Content: "provide key", Status: "pending"}},
	}

	gs := v2.ToGoalState()
	back := FromGoalState(gs)

	if back.Goal != v2.Goal {
		t.Fatalf("Goal round-trip: %q → %q", v2.Goal, back.Goal)
	}
	if back.Status != v2.Status {
		t.Fatalf("Status round-trip: %q → %q", v2.Status, back.Status)
	}
	if back.Block != v2.Block {
		t.Fatalf("Block round-trip: %q → %q", v2.Block, back.Block)
	}
	if len(back.Todos) != 1 || back.Todos[0].Status != "pending" {
		t.Fatalf("Todos round-trip: %+v", back.Todos)
	}
}

func TestGoalStatePersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "goal-state.json")

	// Write.
	state := GoalStateV2{
		Goal:   "test persistence",
		Status: GoalStatusRunning,
	}
	if err := WriteGoalStateV2(path, state); err != nil {
		t.Fatal(err)
	}

	// Read back.
	restored, err := ReadGoalStateV2(path)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Goal != state.Goal {
		t.Fatalf("Goal = %q", restored.Goal)
	}
	if restored.Status != state.Status {
		t.Fatalf("Status = %q", restored.Status)
	}
	if restored.Version != GoalStateVersion {
		t.Fatalf("Version = %d", restored.Version)
	}

	// Non-existent file should return zero value.
	missing, err := ReadGoalStateV2(filepath.Join(dir, "nonexistent.json"))
	if err != nil {
		t.Fatal(err)
	}
	if missing.Goal != "" || missing.Status != "" {
		t.Fatal("nonexistent file should return zero value")
	}
}

func TestGoalStateV0BackwardCompat(t *testing.T) {
	// Version 0 (no version field) should be accepted.
	v0Data := []byte(`{"goal":"old goal","status":"running","turns":3}`)
	state, err := ReadGoalStateForResume(v0Data)
	if err != nil {
		t.Fatal(err)
	}
	if state.Goal != "old goal" {
		t.Fatalf("Goal = %q", state.Goal)
	}
	if state.Status != GoalStatusRunning {
		t.Fatalf("Status = %q", state.Status)
	}
	if state.Turns != 3 {
		t.Fatalf("Turns = %d", state.Turns)
	}
}

func TestGoalStateV2RejectsFutureVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "future.json")

	// Write a state with a version newer than we support.
	if err := os.WriteFile(path, []byte(`{"version":999,"goal":"future","status":"running"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ReadGoalStateV2(path)
	if err == nil {
		t.Fatal("should reject future version")
	}
}

func TestReadGoalStateForResumeRejectsFutureVersion(t *testing.T) {
	_, err := ReadGoalStateForResume([]byte(`{"version":999,"goal":"future","status":"running"}`))
	if err == nil {
		t.Fatal("resume reader should reject future version")
	}
}

func TestReadGoalStateForResumeRejectsMalformedVersion(t *testing.T) {
	for _, data := range []string{
		`{"version":"1","goal":"future","status":"running"}`,
		`{"version":-1,"goal":"future","status":"running"}`,
	} {
		if _, err := ReadGoalStateForResume([]byte(data)); err == nil {
			t.Fatalf("resume reader accepted malformed version in %s", data)
		}
	}
}

func TestGoalMachineDoesNotRestoreFutureVersion(t *testing.T) {
	dir := t.TempDir()
	sessionPath := filepath.Join(dir, "session.jsonl")
	if err := os.WriteFile(goalStatePath(sessionPath), []byte(`{"version":999,"goal":"future","status":"running","todos":[{"content":"future","status":"completed"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var machine goalMachine
	machine.restoreRunningFromState(sessionPath)
	if machine.active() {
		t.Fatal("future goal-state version must not restore a running goal")
	}
	if todos, ok := machine.terminalTodosFromState(sessionPath); ok || len(todos) != 0 {
		t.Fatalf("future goal-state version restored terminal todos: %+v", todos)
	}
}

func TestReadGoalStateForResumeV1(t *testing.T) {
	v1 := []byte(`{"version":1,"goal":"resume test","status":"running","turns":5,"strict":true,"todos":[{"content":"ship","status":"completed"}]}`)
	gs, err := ReadGoalStateForResume(v1)
	if err != nil {
		t.Fatal(err)
	}
	if gs.Goal != "resume test" {
		t.Fatalf("Goal = %q", gs.Goal)
	}
	if gs.Status != GoalStatusRunning {
		t.Fatalf("Status = %q", gs.Status)
	}
	if !gs.Strict {
		t.Fatal("Strict should be true")
	}
	if len(gs.Todos) != 1 || gs.Todos[0].Status != "completed" {
		t.Fatalf("Todos = %+v, want completed todo", gs.Todos)
	}
}

func TestReadGoalStateForResumeV0Fallback(t *testing.T) {
	v0 := []byte(`{"goal":"legacy goal","status":"blocked","block":"missing key"}`)
	gs, err := ReadGoalStateForResume(v0)
	if err != nil {
		t.Fatal(err)
	}
	if gs.Goal != "legacy goal" {
		t.Fatalf("Goal = %q", gs.Goal)
	}
	if gs.Status != GoalStatusBlocked {
		t.Fatalf("Status = %q", gs.Status)
	}
}

func TestGoalContinuationStateSurvivesCrashResume(t *testing.T) {
	dir := t.TempDir()
	sessionPath := filepath.Join(dir, "session.jsonl")
	runtime := goalRuntimeProjection{
		todos:        []evidence.TodoItem{{Content: "wait for operator", Status: "in_progress"}},
		messageCount: 4,
	}
	var first goalMachine
	first.setStatePath(goalStatePath(sessionPath))
	path, data, revision, ok := first.set("deploy safely", GoalResearchOff, "", runtime)
	first.writeState(path, data, revision)
	if !ok {
		t.Fatal("initial goal state was not persisted")
	}
	for range 2 {
		res := first.advance(goalAdvanceInput{
			status: GoalStatusBlocked, reason: "operator approval", toolCalled: true,
			todos: runtime.todos, messageCount: runtime.messageCount,
		})
		first.writeState(res.path, res.data, res.revision)
		if res.notice != "" {
			t.Fatalf("goal blocked before third identical report: %q", res.notice)
		}
	}

	var resumed goalMachine
	resumed.setStatePath(goalStatePath(sessionPath))
	state, ok := resumed.restoreSessionState(sessionPath)
	if !ok || state.Turns != 2 || state.Blocks != 2 || state.Block != "operator approval" {
		t.Fatalf("restored continuation state = %+v, ok=%v", state, ok)
	}
	res := resumed.advance(goalAdvanceInput{
		status: GoalStatusBlocked, reason: "operator approval", toolCalled: true,
		todos: runtime.todos, messageCount: runtime.messageCount,
	})
	if res.notice != "goal blocked: operator approval" || resumed.statusForDisplay() != GoalStatusBlocked {
		t.Fatalf("third blocker after resume = notice %q status %q", res.notice, resumed.statusForDisplay())
	}
}

func TestGoalCompletionInterceptSurvivesCrashAndCannotOverrideEvidence(t *testing.T) {
	dir := t.TempDir()
	sessionPath := filepath.Join(dir, "session.jsonl")
	todos := []evidence.TodoItem{{Content: "run verification", Status: "in_progress"}}
	var first goalMachine
	first.setStatePath(goalStatePath(sessionPath))
	path, data, revision, _ := first.set("ship", GoalResearchOff, "", goalRuntimeProjection{todos: todos})
	first.writeState(path, data, revision)
	res := first.advance(goalAdvanceInput{status: GoalStatusComplete, toolCalled: true, todos: todos})
	first.writeState(res.path, res.data, res.revision)

	var resumed goalMachine
	resumed.setStatePath(goalStatePath(sessionPath))
	state, ok := resumed.restoreSessionState(sessionPath)
	if !ok || state.Intercepts != 1 || state.InterceptMsg == "" {
		t.Fatalf("completion intercept did not survive: %+v ok=%v", state, ok)
	}
	res = resumed.advance(goalAdvanceInput{status: GoalStatusComplete, toolCalled: true, todos: todos})
	if !res.cont || res.notice != "" || resumed.statusForDisplay() != GoalStatusRunning {
		t.Fatalf("repeated completion bypassed evidence: result=%+v status=%q", res, resumed.statusForDisplay())
	}
}

func TestGoalStateWriterRejectsOlderRevision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "goal.json")
	var first goalMachine
	first.writeState(path, []byte(`{"version":2,"status":"running","revision":2}`), 2)
	// A different controller/process has no in-memory writtenRevision entry. The
	// disk revision must still prevent its stale write from winning.
	var stale goalMachine
	stale.writeState(path, []byte(`{"version":2,"status":"stopped","revision":1}`), 1)
	state, err := ReadGoalStateV2(path)
	if err != nil {
		t.Fatal(err)
	}
	if state.Revision != 2 || state.Status != GoalStatusRunning {
		t.Fatalf("older revision overwrote newer state: %+v", state)
	}
}

func TestGoalStateWriterSerializesRevisionCheckAcrossControllers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "goal.json")
	staleEntered := make(chan struct{})
	releaseStale := make(chan struct{})
	var stale goalMachine
	stale.stateWrite = func(path string, data []byte, mode os.FileMode) error {
		close(staleEntered)
		<-releaseStale
		return os.WriteFile(path, data, mode)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		stale.writeState(path, []byte(`{"version":2,"status":"stopped","revision":1}`), 1)
	}()
	<-staleEntered

	freshDone := make(chan struct{})
	go func() {
		defer wg.Done()
		var fresh goalMachine
		fresh.writeState(path, []byte(`{"version":2,"status":"running","revision":2}`), 2)
		close(freshDone)
	}()
	select {
	case <-freshDone:
		t.Fatal("newer writer bypassed the older writer's revision transaction")
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseStale)
	wg.Wait()

	state, err := ReadGoalStateV2(path)
	if err != nil {
		t.Fatal(err)
	}
	if state.Revision != 2 || state.Status != GoalStatusRunning {
		t.Fatalf("concurrent stale write won: %+v", state)
	}
}

func TestGoalStateWriterPreservesMalformedSidecar(t *testing.T) {
	path := filepath.Join(t.TempDir(), "goal.json")
	want := []byte(`{"version":"broken","status":"running"}`)
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatal(err)
	}
	var machine goalMachine
	machine.writeState(path, []byte(`{"version":2,"status":"stopped","revision":3}`), 3)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("malformed sidecar was overwritten: %s", got)
	}
}

func TestGoalTurnLimitSurvivesCrashResume(t *testing.T) {
	dir := t.TempDir()
	sessionPath := filepath.Join(dir, "session.jsonl")
	runtime := goalRuntimeProjection{messageCount: 3}
	var first goalMachine
	first.setStatePath(goalStatePath(sessionPath))
	path, data, revision, _ := first.set("finish reliably", GoalResearchOff, "", runtime)
	first.writeState(path, data, revision)
	for range maxGoalAutoTurns - 1 {
		res := first.advance(goalAdvanceInput{toolCalled: true, messageCount: runtime.messageCount})
		first.writeState(res.path, res.data, res.revision)
		if res.notice != "" {
			t.Fatalf("goal stopped before turn limit: %q", res.notice)
		}
	}

	var resumed goalMachine
	resumed.setStatePath(goalStatePath(sessionPath))
	state, ok := resumed.restoreSessionState(sessionPath)
	if !ok || state.Turns != maxGoalAutoTurns-1 {
		t.Fatalf("restored turn budget = %+v ok=%v", state, ok)
	}
	res := resumed.advance(goalAdvanceInput{toolCalled: true, messageCount: runtime.messageCount})
	if res.notice != "goal continuation limit reached" || resumed.statusForDisplay() != GoalStatusBlocked {
		t.Fatalf("turn limit after resume = result %+v status %q", res, resumed.statusForDisplay())
	}
}

func TestGoalIdleReminderSurvivesCrashResume(t *testing.T) {
	dir := t.TempDir()
	sessionPath := filepath.Join(dir, "session.jsonl")
	var first goalMachine
	first.setStatePath(goalStatePath(sessionPath))
	path, data, revision, _ := first.set("make progress", GoalResearchOff, "", goalRuntimeProjection{})
	first.writeState(path, data, revision)
	res := first.advance(goalAdvanceInput{})
	first.writeState(res.path, res.data, res.revision)

	var resumed goalMachine
	resumed.setStatePath(goalStatePath(sessionPath))
	state, ok := resumed.restoreSessionState(sessionPath)
	if !ok || state.IdleTurns != 1 {
		t.Fatalf("restored idle budget = %+v ok=%v", state, ok)
	}
	res = resumed.advance(goalAdvanceInput{})
	if !res.cont {
		t.Fatalf("idle reminder should continue the goal: %+v", res)
	}
	state, err := ReadGoalStateForResume(res.data)
	if err != nil {
		t.Fatal(err)
	}
	if state.InterceptMsg == "" || state.IdleTurns != 0 {
		t.Fatalf("second idle turn after resume = %+v", state)
	}
}

func TestStrictGoalRequiresActualSelfCheckTurnAfterCrash(t *testing.T) {
	dir := t.TempDir()
	sessionPath := filepath.Join(dir, "session.jsonl")
	todos := []evidence.TodoItem{{Content: "verify release", Status: "completed"}}
	runtime := goalRuntimeProjection{todos: todos, messageCount: 4}
	var first goalMachine
	first.setStatePath(goalStatePath(sessionPath))
	path, data, revision, _ := first.set("ship", GoalResearchOff, "", runtime)
	first.writeState(path, data, revision)
	path, data, revision, _ = first.setStrict(true, runtime)
	first.writeState(path, data, revision)
	res := first.advance(goalAdvanceInput{status: GoalStatusComplete, toolCalled: true, todos: todos, messageCount: 4})
	first.writeState(res.path, res.data, res.revision)

	var resumed goalMachine
	resumed.setStatePath(goalStatePath(sessionPath))
	state, ok := resumed.restoreSessionState(sessionPath)
	if !ok || !state.SelfCheckPending || state.InterceptMsg != goalSelfCheckTurn {
		t.Fatalf("restored self-check phase = %+v ok=%v", state, ok)
	}
	// A normal turn that merely repeats completion cannot stand in for the
	// host-issued strict self-check after a crash.
	res = resumed.advance(goalAdvanceInput{status: GoalStatusComplete, toolCalled: true, todos: todos, messageCount: 6})
	if !res.cont || resumed.statusForDisplay() != GoalStatusRunning {
		t.Fatalf("ordinary completion bypassed self-check: result=%+v status=%q", res, resumed.statusForDisplay())
	}
	if msg, ok := resumed.takeIntercept(); !ok || msg != goalSelfCheckTurn {
		t.Fatalf("self-check intercept = %q ok=%v", msg, ok)
	}
	res = resumed.advance(goalAdvanceInput{status: GoalStatusComplete, toolCalled: true, selfCheckTurn: true, todos: todos, messageCount: 8})
	if res.notice != goalCompleteNotice || resumed.statusForDisplay() != GoalStatusComplete {
		t.Fatalf("actual self-check did not complete: result=%+v status=%q", res, resumed.statusForDisplay())
	}
}
