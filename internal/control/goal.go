package control

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"unicode"

	"reames-agent/internal/event"
	"reames-agent/internal/evidence"
	"reames-agent/internal/fileutil"
	"reames-agent/internal/store"
)

const (
	maxGoalAutoTurns   = 50
	maxGoalIdleTurns   = 2
	goalContinueTurn   = "Continue pursuing the active goal under its task contract. If it is complete, provide the concise final result and end with [goal:complete]. If progress genuinely requires user-only information, an irreversible or externally visible operation, or a changed scope, end with [goal:blocked:<short reason>]. Otherwise use sensible defaults, do the next useful work, and end with [goal:continue]."
	goalSelfCheckTurn  = "The agent signaled goal completion and all tasks are marked done. Before finalizing, perform a brief quality self-check:\n1. Verify any changed files compile or parse correctly\n2. Run the relevant tests if applicable\n3. Confirm the original request, output format, constraints, and success criteria are met\nIf everything checks out, signal [goal:complete]. If issues are found, fix them and signal [goal:complete] when done."
	goalCompleteNotice = "goal complete"
)

// goalMachine owns the active goal's finite-state machine and its persistence.
// It is a strict leaf: its methods take only the machine's own locks and never
// call back into the Controller, so the controller may hold c.mu while invoking
// a getter without risking lock inversion. The FSM is pure — advance() takes
// already-gathered inputs (the parsed marker, the executor's todo snapshot and
// readiness, whether a tool ran) and returns what to persist plus a notice, so
// no disk or executor work happens under mu.
type goalMachine struct {
	// mu guards the FSM fields below; every critical section under it is short
	// and non-blocking (no disk I/O, no executor calls).
	mu                 sync.Mutex
	goal               string
	status             string
	researchMode       GoalResearchMode
	autoResearchTaskID string
	turns              int
	blocks             int
	block              string
	interceptMsg       string
	intercepts         int
	strict             bool
	selfCheckPending   bool
	idleTurns          int
	revision           uint64

	// statePath is the persisted goal-state sidecar; empty disables persistence.
	statePath string
	// writeMu serializes goal-state disk writes so concurrent saves don't
	// interleave or land out of order. Taken OFF mu by writeState.
	writeMu         sync.Mutex
	writtenRevision map[string]uint64
	// stateWrite is a fault-injection seam for revision-ordering tests.
	stateWrite func(string, []byte, os.FileMode) error
}

// goalState is the serializable session runtime projection. Goal, Plan, Todo,
// and continuation bookkeeping are persisted together so resume/switch/rewind
// can replace the whole projection instead of layering new state over stale
// in-memory fields.
type goalState struct {
	Goal               string                 `json:"goal,omitempty"`
	Status             string                 `json:"status,omitempty"`
	ResearchMode       GoalResearchMode       `json:"researchMode,omitempty"`
	AutoResearchTaskID string                 `json:"autoResearchTaskID,omitempty"`
	Turns              int                    `json:"turns,omitempty"`
	Blocks             int                    `json:"blocks,omitempty"`
	Block              string                 `json:"block,omitempty"`
	Strict             bool                   `json:"strict,omitempty"`
	Intercepts         int                    `json:"intercepts,omitempty"`
	InterceptMsg       string                 `json:"interceptMsg,omitempty"`
	SelfCheckPending   bool                   `json:"selfCheckPending,omitempty"`
	IdleTurns          int                    `json:"idleTurns,omitempty"`
	PlanMode           bool                   `json:"planMode,omitempty"`
	TodosKnown         bool                   `json:"todosKnown,omitempty"`
	Todos              []evidence.TodoItem    `json:"todos,omitempty"`
	MessageCount       int                    `json:"messageCount,omitempty"`
	TranscriptDigest   string                 `json:"transcriptDigest,omitempty"`
	DurableEvidence    *evidence.DurableState `json:"durableEvidence,omitempty"`
	Revision           uint64                 `json:"revision,omitempty"`
}

type goalRuntimeProjection struct {
	planMode         bool
	todos            []evidence.TodoItem
	messageCount     int
	transcriptDigest string
	durableEvidence  evidence.DurableState
}

// goalAdvanceInput carries everything the FSM needs for one continuation step,
// gathered by the caller off the machine's lock.
type goalAdvanceInput struct {
	status           string // parsed marker status ("" when the turn carried no marker)
	reason           string // blocked reason from the marker, if any
	toolCalled       bool   // whether the last turn made any tool call
	selfCheckTurn    bool   // whether the host actually issued the strict self-check turn
	todos            []evidence.TodoItem
	readiness        string // executor.GoalReadinessFailure()
	planMode         bool
	messageCount     int
	transcriptDigest string
	durableEvidence  evidence.DurableState
}

// goalAdvanceResult reports the FSM step's outcome. data/path/ok describe the
// state to persist (built under mu for every active transition); notice is surfaced
// to the user; cont reports whether the goal loop should continue.
type goalAdvanceResult struct {
	notice   string
	cont     bool
	path     string
	data     []byte
	ok       bool
	revision uint64
}

// goalStatePath derives a session's persisted goal-state sidecar.
func goalStatePath(sessionPath string) string {
	return store.SessionGoalState(sessionPath)
}

func (g *goalMachine) setStatePath(path string) {
	g.mu.Lock()
	g.statePath = path
	g.mu.Unlock()
}

// snapshot returns the fields Compose injects into outgoing turns.
func (g *goalMachine) snapshot() (goal, status string, mode GoalResearchMode, autoResearchTaskID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.goal, g.status, g.researchMode, g.autoResearchTaskID
}

func (g *goalMachine) goalText() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.goal
}

func (g *goalMachine) currentAutoResearchTaskID() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if strings.TrimSpace(g.goal) == "" || g.status != GoalStatusRunning {
		return ""
	}
	return g.autoResearchTaskID
}

// active reports whether a goal is currently running.
func (g *goalMachine) active() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return strings.TrimSpace(g.goal) != "" && g.status == GoalStatusRunning
}

// statusForDisplay maps the empty zero status to "stopped" for frontends.
func (g *goalMachine) statusForDisplay() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.status == "" {
		return GoalStatusStopped
	}
	return g.status
}

// set installs a session-scoped goal (or clears it when goal is empty), resets
// the per-goal counters, and returns the state to persist. ok is false (no
// persistence) when the goal is unchanged or no state path is configured.
func (g *goalMachine) set(goal string, mode GoalResearchMode, autoResearchTaskID string, runtime goalRuntimeProjection) (string, []byte, uint64, bool) {
	goal = strings.TrimSpace(goal)
	g.mu.Lock()
	defer g.mu.Unlock()
	if goal != "" && g.goal == goal && g.status == GoalStatusRunning && g.researchMode == mode && g.autoResearchTaskID == autoResearchTaskID {
		return "", nil, 0, false
	}
	g.turns, g.blocks, g.block = 0, 0, ""
	g.interceptMsg, g.intercepts = "", 0
	g.selfCheckPending, g.idleTurns, g.strict = false, 0, false
	if goal == "" {
		g.goal, g.status, g.researchMode, g.autoResearchTaskID = "", GoalStatusStopped, GoalResearchAuto, ""
	} else {
		g.goal, g.status, g.researchMode, g.autoResearchTaskID = goal, GoalStatusRunning, mode, autoResearchTaskID
	}
	g.revision++
	return g.buildStateLocked(runtime)
}

func (g *goalMachine) setStrict(strict bool, runtime goalRuntimeProjection) (string, []byte, uint64, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.strict = strict
	g.revision++
	return g.buildStateLocked(runtime)
}

// stop transitions a running goal to the given terminal status and clears the
// transient intercept/idle bookkeeping.
func (g *goalMachine) stop(status string, runtime goalRuntimeProjection) (string, []byte, uint64, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if strings.TrimSpace(g.goal) != "" && g.status == GoalStatusRunning {
		g.status = status
	}
	g.interceptMsg = ""
	g.intercepts = 0
	g.selfCheckPending = false
	g.idleTurns = 0
	g.revision++
	return g.buildStateLocked(runtime)
}

// takeIntercept consumes a pending continuation-turn instruction, if any.
func (g *goalMachine) takeIntercept() (string, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.interceptMsg == "" {
		return "", false
	}
	msg := g.interceptMsg
	g.interceptMsg = ""
	return msg, true
}

func (g *goalMachine) requeueIntercept(msg string, runtime goalRuntimeProjection) (string, []byte, uint64, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.status != GoalStatusRunning || strings.TrimSpace(g.goal) == "" || strings.TrimSpace(msg) == "" {
		return "", nil, 0, false
	}
	if g.interceptMsg == "" {
		g.interceptMsg = msg
	}
	g.revision++
	return g.buildStateLocked(runtime)
}

// advance runs one continuation step of the goal FSM from already-gathered
// inputs. It mutates the machine, decides whether to keep looping, and builds
// the state to persist for every active transition.
func (g *goalMachine) advance(in goalAdvanceInput) goalAdvanceResult {
	g.mu.Lock()
	defer g.mu.Unlock()
	if strings.TrimSpace(g.goal) == "" || g.status != GoalStatusRunning {
		return goalAdvanceResult{cont: false}
	}
	g.turns++
	var notice string
	switch in.status {
	case GoalStatusComplete:
		if incomplete := formatIncompleteTodos(in.todos, in.readiness); len(incomplete) > 0 {
			// Completion is evidence-gated in every mode. Strict adds the final
			// self-check; it never changes whether incomplete work may complete.
			g.intercepts++
			g.interceptMsg = incomplete
			g.selfCheckPending = false
			break
		}
		// Todos are all done — in strict mode run self-check before final
		// completion. Non-strict mode completes immediately.
		if g.strict && !in.selfCheckTurn {
			g.selfCheckPending = true
			g.interceptMsg = goalSelfCheckTurn
			break
		}
		// Self-check passed — complete the goal.
		g.intercepts = 0
		g.selfCheckPending = false
		g.idleTurns = 0
		g.goal = ""
		g.status = GoalStatusComplete
		g.blocks = 0
		g.block = ""
		g.interceptMsg = ""
		notice = goalCompleteNotice
	case GoalStatusBlocked:
		g.selfCheckPending = false
		reason := cleanGoalBlockReason(in.reason)
		if reason == "" {
			reason = "blocked"
		}
		if sameGoalBlock(g.block, reason) {
			g.blocks++
		} else {
			g.blocks = 1
			g.block = reason
		}
		if g.blocks >= 3 {
			g.status = GoalStatusBlocked
			notice = "goal blocked: " + reason
		}
	default:
		g.blocks = 0
		g.block = ""
		g.intercepts = 0
		g.selfCheckPending = false
	}
	// Idle detection: if the agent went multiple turns without any tool calls,
	// inject a reminder to make progress (unless the goal is already completing
	// or hitting the auto-turn limit).
	if notice == "" && g.interceptMsg == "" {
		if in.toolCalled {
			g.idleTurns = 0
		} else {
			g.idleTurns++
			if g.idleTurns >= maxGoalIdleTurns {
				g.idleTurns = 0
				g.interceptMsg = "No tool calls in recent turns. Either make progress with tools or signal [goal:blocked:<reason>]."
			}
		}
	}
	if notice == "" && g.turns >= maxGoalAutoTurns {
		g.status = GoalStatusBlocked
		g.block = "goal continuation limit reached"
		g.intercepts = 0
		g.selfCheckPending = false
		g.interceptMsg = ""
		g.idleTurns = 0
		notice = g.block
	}
	res := goalAdvanceResult{notice: notice, cont: notice == ""}
	// Every transition is durable, including continue/block streaks, idle
	// reminders, completion intercepts, and the self-check phase. A crash may
	// replay a pending reminder, but cannot reset the safety budgets.
	g.revision++
	runtime := goalRuntimeProjection{
		planMode: in.planMode, todos: in.todos,
		messageCount: in.messageCount, transcriptDigest: in.transcriptDigest,
		durableEvidence: in.durableEvidence,
	}
	res.path, res.data, res.revision, res.ok = g.buildStateLocked(runtime)
	return res
}

// buildStateLocked marshals the current goal state for persistence. The caller
// holds mu; this only reads in-memory state, never touching disk. Returns ok=false
// when persistence is disabled (no state path). The matching writeState does the
// disk write OFF mu so the per-turn save can't stall a status poll.
func (g *goalMachine) buildStateLocked(runtime goalRuntimeProjection) (path string, data []byte, revision uint64, ok bool) {
	if g.statePath == "" {
		return "", nil, 0, false
	}
	b, err := json.Marshal(FromGoalState(g.stateLocked(runtime)))
	if err != nil {
		slog.Warn("controller: marshal goal state", "err", err)
		return "", nil, 0, false
	}
	return g.statePath, b, g.revision, true
}

func (g *goalMachine) stateLocked(runtime goalRuntimeProjection) goalState {
	status := g.status
	if status == "" {
		status = GoalStatusStopped
	}
	return goalState{
		Goal:               g.goal,
		Status:             status,
		ResearchMode:       g.researchMode,
		AutoResearchTaskID: g.autoResearchTaskID,
		Turns:              g.turns,
		Blocks:             g.blocks,
		Block:              g.block,
		Strict:             g.strict,
		Intercepts:         g.intercepts,
		InterceptMsg:       g.interceptMsg,
		SelfCheckPending:   g.selfCheckPending,
		IdleTurns:          g.idleTurns,
		PlanMode:           runtime.planMode,
		TodosKnown:         true,
		Todos:              append([]evidence.TodoItem(nil), runtime.todos...),
		MessageCount:       runtime.messageCount,
		TranscriptDigest:   runtime.transcriptDigest,
		DurableEvidence:    durableEvidencePointer(runtime.durableEvidence),
		Revision:           g.revision,
	}
}

func durableEvidencePointer(state evidence.DurableState) *evidence.DurableState {
	if !state.WritePending && len(state.VerifiedChecks) == 0 {
		return nil
	}
	clone := state.Clone()
	return &clone
}

func (g *goalMachine) snapshotData(runtime goalRuntimeProjection) json.RawMessage {
	g.mu.Lock()
	defer g.mu.Unlock()
	b, err := json.Marshal(FromGoalState(g.stateLocked(runtime)))
	if err != nil {
		slog.Warn("controller: marshal checkpoint runtime", "err", err)
		return nil
	}
	return b
}

// writeState persists pre-marshaled goal-state bytes to disk, OFF mu and
// serialized by writeMu so concurrent saves don't interleave or land out of
// order. Callers that guard a writer propagate the returned error; background
// refresh callers may keep their existing best-effort behavior.
func (g *goalMachine) writeState(path string, data []byte, revision uint64) error {
	if path == "" || data == nil {
		return nil
	}
	g.writeMu.Lock()
	defer g.writeMu.Unlock()
	// The disk revision check and the atomic replacement form one compare/write
	// transaction across Controller instances in this process. Session leases
	// provide the corresponding cross-process single-writer boundary.
	unlock := lockGoalStateFile(path)
	defer unlock()
	if g.writtenRevision == nil {
		g.writtenRevision = make(map[string]uint64)
	}
	if last, exists := g.writtenRevision[path]; exists && revision <= last {
		return nil
	}
	if current, err := os.ReadFile(path); err == nil {
		state, parseErr := ReadGoalStateForResume(current)
		if parseErr != nil {
			slog.Warn("controller: preserve unreadable goal state", "path", path, "err", parseErr)
			return fmt.Errorf("preserve unreadable goal state %q: %w", path, parseErr)
		}
		if revision <= state.Revision {
			g.writtenRevision[path] = state.Revision
			return nil
		}
	} else if !os.IsNotExist(err) {
		slog.Warn("controller: inspect goal state revision", "path", path, "err", err)
		return fmt.Errorf("inspect goal state revision %q: %w", path, err)
	}
	writeFile := g.stateWrite
	if writeFile == nil {
		writeFile = fileutil.AtomicWriteFile
	}
	if err := writeFile(path, data, 0o644); err != nil {
		slog.Warn("controller: write goal state", "err", err)
		return fmt.Errorf("write goal state %q: %w", path, err)
	}
	g.writtenRevision[path] = revision
	return nil
}

// persistRuntime writes the current Goal FSM together with Plan/Todo state.
func (g *goalMachine) persistRuntime(runtime goalRuntimeProjection) error {
	g.mu.Lock()
	g.revision++
	path, data, revision, ok := g.buildStateLocked(runtime)
	g.mu.Unlock()
	if ok {
		return g.writeState(path, data, revision)
	}
	return nil
}

// readSessionState reads one persisted runtime projection. Invalid or future
// formats fail closed and leave the caller's transcript-derived state intact.
func (g *goalMachine) readSessionState(sessionPath string) (goalState, bool) {
	if strings.TrimSpace(sessionPath) == "" {
		return goalState{}, false
	}
	data, err := os.ReadFile(goalStatePath(sessionPath))
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("controller: read goal state", "err", err)
		}
		return goalState{}, false
	}
	state, err := ReadGoalStateForResume(data)
	if err != nil {
		slog.Warn("controller: parse goal state", "err", err)
		return goalState{}, false
	}
	return state, true
}

// restoreSessionState replaces every session-scoped Goal FSM field. It always
// clears the previous session first, so switching to a branch with no sidecar
// cannot retain and later persist the source branch's goal.
func (g *goalMachine) restoreSessionState(sessionPath string) (goalState, bool) {
	state, ok := g.readSessionState(sessionPath)
	if ok && !validGoalState(state) {
		ok = false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.replaceStateLocked(state, ok, false)
	if !ok {
		return goalState{}, false
	}
	return state, true
}

// restoreRunningFromState is retained for focused compatibility tests. Session
// lifecycle code uses restoreSessionState so terminal/empty branches also clear
// stale in-memory state.
func (g *goalMachine) restoreRunningFromState(sessionPath string) {
	state, ok := g.readSessionState(sessionPath)
	if !ok || state.Status != GoalStatusRunning || strings.TrimSpace(state.Goal) == "" {
		return
	}
	g.mu.Lock()
	g.replaceStateLocked(state, true, false)
	g.mu.Unlock()
}

func (g *goalMachine) terminalTodosFromState(sessionPath string) ([]evidence.TodoItem, bool) {
	state, ok := g.readSessionState(sessionPath)
	if !ok {
		return nil, false
	}
	switch state.Status {
	case GoalStatusComplete, GoalStatusBlocked, GoalStatusStopped:
	default:
		return nil, false
	}
	if !state.TodosKnown && len(state.Todos) == 0 {
		return nil, false
	}
	return append([]evidence.TodoItem(nil), state.Todos...), true
}

func (g *goalMachine) restoreCheckpointState(data []byte) (goalState, bool) {
	state, err := g.decodeCheckpointState(data)
	if err != nil {
		slog.Warn("controller: parse checkpoint runtime", "err", err)
		return goalState{}, false
	}
	g.applyCheckpointState(state)
	return state, true
}

func (g *goalMachine) decodeCheckpointState(data []byte) (goalState, error) {
	state, err := ReadGoalStateForResume(data)
	if err != nil {
		return goalState{}, err
	}
	if !validGoalState(state) {
		return goalState{}, fmt.Errorf("invalid checkpoint runtime status %q", state.Status)
	}
	return state, nil
}

func (g *goalMachine) applyCheckpointState(state goalState) {
	g.mu.Lock()
	g.replaceStateLocked(state, true, true)
	g.mu.Unlock()
}

func validGoalState(state goalState) bool {
	switch state.Status {
	case GoalStatusRunning:
		return strings.TrimSpace(state.Goal) != ""
	case GoalStatusComplete, GoalStatusBlocked, GoalStatusStopped:
		return true
	default:
		return false
	}
}

func (g *goalMachine) replaceStateLocked(state goalState, ok, keepRevisionMonotonic bool) {
	previousRevision := g.revision
	g.goal = ""
	g.status = GoalStatusStopped
	g.researchMode = GoalResearchAuto
	g.autoResearchTaskID = ""
	g.turns, g.blocks, g.block = 0, 0, ""
	g.strict = false
	g.interceptMsg, g.intercepts = "", 0
	g.selfCheckPending, g.idleTurns = false, 0
	g.revision = 0
	if !ok {
		return
	}
	switch state.Status {
	case GoalStatusRunning, GoalStatusComplete, GoalStatusBlocked, GoalStatusStopped:
	default:
		return
	}
	if state.Status == GoalStatusRunning && strings.TrimSpace(state.Goal) == "" {
		return
	}
	g.goal = strings.TrimSpace(state.Goal)
	g.status = state.Status
	g.researchMode = state.ResearchMode
	g.autoResearchTaskID = strings.TrimSpace(state.AutoResearchTaskID)
	g.turns = state.Turns
	g.blocks = state.Blocks
	g.block = state.Block
	g.strict = state.Strict
	g.interceptMsg, g.intercepts = state.InterceptMsg, state.Intercepts
	g.selfCheckPending, g.idleTurns = state.SelfCheckPending, state.IdleTurns
	g.revision = state.Revision
	if keepRevisionMonotonic && previousRevision > g.revision {
		g.revision = previousRevision
	}
}

// formatIncompleteTodos renders the reminder shown when [goal:complete] arrives
// while the executor's canonical todos or project-readiness checks aren't done.
// Returns empty when nothing is blocking. Pure: the caller gathers todos and the
// readiness reason from the executor off the goal lock.
func formatIncompleteTodos(todos []evidence.TodoItem, readiness string) string {
	var parts []string
	if len(todos) > 0 {
		if incomplete := evidence.IncompleteTodos(todos); len(incomplete) > 0 {
			var b strings.Builder
			b.WriteString("the following tasks are still incomplete:")
			for _, t := range incomplete {
				fmt.Fprintf(&b, "\n  - %s (%s)", t.Content, t.Status)
			}
			parts = append(parts, b.String())
		}
	}
	if readiness != "" {
		parts = append(parts, readiness)
	}
	if len(parts) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Goal signaled complete but issues remain:\n")
	for _, p := range parts {
		b.WriteString("- ")
		b.WriteString(p)
		b.WriteString("\n")
	}
	b.WriteString("Fix or use todo_write/complete_step to mark done, then [goal:complete] again.")
	return b.String()
}

func parseGoalStatusMarker(text string) (status, reason string, ok bool) {
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		switch lower {
		case "[goal:complete]":
			return GoalStatusComplete, "", true
		case "[goal:continue]":
			return GoalStatusRunning, "", true
		}
		const blockedPrefix = "[goal:blocked:"
		if strings.HasPrefix(lower, blockedPrefix) && strings.HasSuffix(line, "]") {
			return GoalStatusBlocked, strings.TrimSpace(line[len(blockedPrefix) : len(line)-1]), true
		}
		return "", "", false
	}
	return "", "", false
}

func sameGoalBlock(a, b string) bool {
	return normalizeGoalBlockReason(a) == normalizeGoalBlockReason(b)
}

func cleanGoalBlockReason(reason string) string {
	return strings.Trim(strings.TrimSpace(reason), " \t\r\n:：,，.。;；!！?？-—_[]()（）")
}

func normalizeGoalBlockReason(reason string) string {
	reason = strings.ToLower(cleanGoalBlockReason(reason))
	var b strings.Builder
	lastSpace := true
	for _, r := range reason {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastSpace = false
		default:
			if !lastSpace {
				b.WriteByte(' ')
				lastSpace = true
			}
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// ShortGoalForNotice collapses whitespace and truncates a goal for one-line UI.
func ShortGoalForNotice(goal string) string {
	goal = strings.Join(strings.Fields(goal), " ")
	runes := []rune(goal)
	const max = 160
	if len(runes) <= max {
		return goal
	}
	return string(runes[:max]) + "..."
}

// goalTodos snapshots the executor's canonical todos for goal-state persistence.
func (c *Controller) goalTodos() []evidence.TodoItem {
	if c.executor == nil {
		return nil
	}
	return c.executor.CanonicalTodoState()
}

func (c *Controller) goalRuntimeProjection() goalRuntimeProjection {
	projection := goalRuntimeProjection{planMode: c.PlanMode(), todos: c.goalTodos()}
	if c.executor != nil {
		c.reconcileSubagentEffects()
		projection.durableEvidence = c.executor.DurableEvidenceState()
	}
	if c.executor != nil && c.executor.Session() != nil {
		projection.messageCount, projection.transcriptDigest = c.executor.Session().TranscriptAnchor()
	}
	return projection
}

// persistGoalState writes a freshly built goal state to disk, off c.mu. The
// executor guard preserves the original behavior of skipping persistence when
// no executor is attached.
func (c *Controller) persistGoalState(path string, data []byte, revision uint64, ok bool) error {
	if !ok || c.executor == nil {
		return nil
	}
	return c.goals.writeState(path, data, revision)
}

func (c *Controller) restoreSessionRuntime(sessionPath string) {
	state, ok := c.goals.restoreSessionState(sessionPath)
	if !ok {
		if c.executor != nil {
			c.executor.ClearDurableEvidence()
		}
		c.setPlanMode(false, false)
		return
	}
	c.restoreRuntimeTodos(state)
	c.restoreRuntimeEvidence(state)
	c.setPlanMode(state.PlanMode, false)
}

func (c *Controller) restoreCheckpointRuntime(data []byte) bool {
	state, ok := c.goals.restoreCheckpointState(data)
	if !ok {
		return false
	}
	c.restoreRuntimeTodos(state)
	c.restoreRuntimeEvidence(state)
	c.setPlanMode(state.PlanMode, false)
	c.goals.persistRuntime(c.goalRuntimeProjection())
	return true
}

func (c *Controller) restoreCheckpointState(state goalState) {
	c.applyCheckpointState(state)
	c.goals.persistRuntime(c.goalRuntimeProjection())
}

func (c *Controller) applyCheckpointState(state goalState) {
	c.goals.applyCheckpointState(state)
	c.restoreRuntimeTodos(state)
	c.restoreRuntimeEvidence(state)
	c.setPlanMode(state.PlanMode, false)
}

func (c *Controller) restoreRuntimeTodos(state goalState) {
	if c.executor == nil {
		return
	}
	if !state.TodosKnown {
		// Legacy sidecars only used terminal Todo snapshots as a repair for the
		// old completion override. Running v0/v1 state stays transcript-derived.
		if state.Status != GoalStatusRunning && len(state.Todos) > 0 {
			c.executor.RestoreTodoState(append([]evidence.TodoItem(nil), state.Todos...))
		}
		return
	}
	// Modern v2 projections carry a transcript anchor. Exact equality means the
	// sidecar and transcript describe the same boundary (including a compacted
	// log); an append-only extension proves the transcript is newer. A rewrite or
	// divergence is intentionally inconclusive and keeps transcript-derived Todo
	// state rather than allowing a larger pre-compaction count to pose as newer.
	session := c.executor.Session()
	if session != nil && state.TranscriptDigest != "" {
		equal, extends := session.CompareTranscriptAnchor(state.MessageCount, state.TranscriptDigest)
		switch {
		case equal:
			c.executor.RestoreTodoState(append([]evidence.TodoItem(nil), state.Todos...))
		case extends:
			c.executor.RestoreTodoStateFromAnchor(append([]evidence.TodoItem(nil), state.Todos...), state.MessageCount)
		}
		return
	}
	// Legacy v2 sidecars lack an anchor. Preserve their original count heuristic
	// for compatibility; every newly persisted projection upgrades itself.
	if session == nil || state.MessageCount >= session.Len() {
		c.executor.RestoreTodoState(append([]evidence.TodoItem(nil), state.Todos...))
	}
}

func (c *Controller) restoreRuntimeEvidence(state goalState) {
	if c.executor == nil {
		return
	}
	defer c.reconcileSubagentEffects()
	c.executor.ClearDurableEvidence()
	if state.DurableEvidence == nil || state.TranscriptDigest == "" {
		return
	}
	session := c.executor.Session()
	if session == nil {
		return
	}
	equal, _ := session.CompareTranscriptAnchor(state.MessageCount, state.TranscriptDigest)
	if !equal {
		return
	}
	c.executor.RestoreDurableEvidence(state.DurableEvidence.Clone())
}

func (c *Controller) reconcileSubagentEffects() {
	if c == nil || c.executor == nil || c.subagents == nil {
		return
	}
	parentSession := c.parentSessionID()
	if parentSession == "" {
		return
	}
	acknowledged := c.executor.DurableEvidenceState().SubagentEffects
	events, err := c.subagents.RecoverSubagentEffects(parentSession, c.workspaceRoot, c.executor.Session().Snapshot(), acknowledged)
	if err == nil {
		err = c.executor.ApplyRecoveredSubagentEffects(events)
	}
	if err != nil {
		if c.reportSubagentEffectRecoveryError(err) {
			c.executor.InvalidateDurableSubagentEffects()
		}
		return
	}
	c.reportSubagentEffectRecoveryError(nil)
}

func (c *Controller) reportSubagentEffectRecoveryError(err error) bool {
	message := ""
	if err != nil {
		message = err.Error()
	}
	c.subagentEffectsMu.Lock()
	if c.subagentEffectsLastErr == message {
		c.subagentEffectsMu.Unlock()
		return false
	}
	c.subagentEffectsLastErr = message
	c.subagentEffectsMu.Unlock()
	if message != "" {
		slog.Warn("controller: subagent effect recovery failed closed", "err", err)
		c.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn, Text: "subagent effect recovery failed closed; root project checks must run again: " + message})
	}
	return true
}

func writeSessionRuntimeData(sessionPath string, data []byte) error {
	if strings.TrimSpace(sessionPath) == "" || len(data) == 0 {
		return nil
	}
	state, err := ReadGoalStateForResume(data)
	if err != nil {
		return err
	}
	return WriteGoalStateV2(goalStatePath(sessionPath), FromGoalState(state))
}
