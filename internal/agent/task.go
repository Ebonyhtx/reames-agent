package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"strconv"
	"strings"

	"reames-agent/internal/event"
	"reames-agent/internal/jobs"
	"reames-agent/internal/planmode"
	"reames-agent/internal/provider"
	"reames-agent/internal/tool"
	"reames-agent/internal/workspacelease"
)

// DefaultTaskSystemPrompt steers a sub-agent toward focused, terse delivery —
// it doesn't see the parent's conversation so it must self-contain.
const DefaultTaskSystemPrompt = `You are a sub-agent invoked by a parent coding agent to carry out one focused task.
Use the provided tools to investigate or act. Return a single final answer that is concise
and self-contained — the parent will see only that answer, not your tool calls or reasoning.
If you need to ask for clarification, fail with a precise question instead of guessing.`

// DefaultReadOnlyTaskSystemPrompt steers read-only sub-agents toward isolated
// research. They never receive writer tools, persisted transcript controls, or
// background process controls, so their final answer is the only handoff.
const DefaultReadOnlyTaskSystemPrompt = `You are a read-only research sub-agent invoked by a parent coding agent.
Use only the provided read-only tools to inspect code, docs, history, and safe shell output.
Do not attempt to write files, install capabilities, mutate memory, control long-lived
processes, or delegate to writer-capable agents. If a read-only delegation tool is
available and genuinely useful, you may use it within the configured depth limit.
Return a concise, self-contained final answer with the evidence the parent needs.`

const subagentStartContext = `<subagent-context event="SubagentStart">
Before acting, check the available skills and tools. If a relevant skill is available, invoke it before continuing. Delegate to another sub-agent only when the task genuinely benefits from isolated context and the delegation tool is available.
</subagent-context>`

var subagentRecursiveTools = []string{
	"task",
	"read_only_task",
	"run_skill",
	"read_only_skill",
	"read_skill",
	"explore",
	"research",
	"review",
	"security_review",
}

var subagentAlwaysHiddenTools = []string{
	"parallel_tasks",
	"install_skill",
	"install_source",
}

var subagentJobTools = []string{
	"wait",
	"bash_output",
	"kill_shell",
}

var readOnlySubagentWorkflowTools = []string{
	"connect_tool_source",
}

const subagentToolBoundarySummary = "Recursive agent/skill tools are exposed only while max_subagent_depth leaves another delegation layer; unsupported background job tools (parallel_tasks, wait, bash_output, kill_shell) are excluded; bash is exposed as foreground-only inside subagents."

// SubagentMetaTools returns the tool names that spawned agents should not inherit
// from the parent registry unless a future call site deliberately opts into a
// different boundary. They can spawn or author more agent work, so excluding them
// preserves one layer of delegation without adding a spawn-count cap.
func SubagentMetaTools() []string {
	out := append([]string(nil), subagentRecursiveTools...)
	out = append(out, subagentAlwaysHiddenTools...)
	return out
}

// SubagentToolRegistry returns the tool set exposed inside spawned sub-agents:
// the requested whitelist (or every parent tool), minus meta tools that would
// spawn more agent work and job tools whose runtime manager is not injected into
// sub-agents. When bash is present, it is wrapped to advertise and allow only
// foreground execution.
func SubagentToolRegistry(parent *tool.Registry, names []string) *tool.Registry {
	return SubagentToolRegistryForDepth(parent, names, 1, 1)
}

// SubagentToolRegistryForDepth returns the writer-capable tool set for a spawned
// subagent at childDepth. Recursive delegation tools are available only when the
// child still has room to spawn one more subagent.
func SubagentToolRegistryForDepth(parent *tool.Registry, names []string, childDepth, maxDepth int) *tool.Registry {
	exclude := append([]string(nil), subagentAlwaysHiddenTools...)
	if childDepth >= NormalizeMaxSubagentDepth(maxDepth) {
		exclude = append(exclude, subagentRecursiveTools...)
	}
	exclude = append(exclude, subagentJobTools...)
	sub := FilterRegistry(parent, names, exclude...)
	if bash, ok := sub.Get("bash"); ok {
		sub.Add(foregroundOnlyBash{inner: bash})
	}
	return sub
}

type foregroundOnlyBash struct {
	inner tool.Tool
}

func (b foregroundOnlyBash) Name() string { return "bash" }

func (b foregroundOnlyBash) Description() string {
	desc := strings.TrimSpace(b.inner.Description())
	if desc == "" {
		desc = "Execute a command in the shell and return combined stdout/stderr."
	}
	desc = strings.Replace(desc, "Execute a command in the shell", "Execute a foreground command in the shell", 1)
	return desc + " Background execution is unavailable inside subagents."
}

func (foregroundOnlyBash) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"command":{"type":"string","description":"Shell command to execute in the foreground"}},"required":["command"]}`)
}

func (b foregroundOnlyBash) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		RunInBackground bool `json:"run_in_background"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if p.RunInBackground {
		return "", fmt.Errorf("background bash is unavailable in subagents; run a foreground command or ask the parent agent to start a background job")
	}
	return b.inner.Execute(ctx, args)
}

func (b foregroundOnlyBash) ReadOnly() bool { return b.inner.ReadOnly() }

type readOnlyBash struct {
	inner tool.Tool
}

func (b readOnlyBash) Name() string { return "bash" }

func (b readOnlyBash) Description() string {
	desc := strings.TrimSpace(b.inner.Description())
	if desc == "" {
		desc = "Execute a command in the shell and return combined stdout/stderr."
	}
	desc = strings.Replace(desc, "Execute a command in the shell", "Execute a foreground read-only command in the shell", 1)
	return desc + " Only plan-mode safe read-only commands are allowed; shell operators, background execution, process preservation, and write-capable arguments are blocked."
}

func (readOnlyBash) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"command":{"type":"string","description":"Read-only shell command to execute in the foreground. Must match the plan-mode safe bash policy."}},"required":["command"]}`)
}

func (b readOnlyBash) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	decision := planmode.Policy{}.Decide(planmode.Call{Name: "bash", Args: args})
	if decision.Blocked {
		return decision.Message, nil
	}
	return b.inner.Execute(ctx, args)
}

func (readOnlyBash) ReadOnly() bool { return true }

// TaskTool spawns a sub-agent in its own session for a focused sub-task. The
// sub-agent runs with a filtered tool whitelist and the same step budget shape
// as the parent (see Execute); its tool calls are forwarded to the parent's
// event stream nested under this call, while only its final assistant message is
// returned to the parent model. Use cases: keep noisy tool sequences (multi-file
// exploration, repeated grep / read_file) out of the parent's context budget, or
// parallel research across independent areas (the parallel-dispatch path picks
// these up only when readOnly, which task is not).
type TaskTool struct {
	prov                 provider.Provider
	pricing              *provider.Pricing
	parentReg            *tool.Registry
	maxSteps             int
	contextWindow        int
	softCompactRatio     float64
	toolResultSnipRatio  float64
	compactRatio         float64
	compactForceRatio    float64
	recentKeep           int
	temperature          float64
	archiveDir           string
	keepPolicy           KeepPolicy
	sysPrompt            string
	gate                 Gate
	subagentModel        string
	subagentEffort       string
	resolveProvider      func(modelRef, effort string) (provider.Provider, *provider.Pricing, int, error)
	transcripts          *SubagentStore
	workspaceRoot        string
	baseModel            string
	baseEffort           string
	identityProfile      func(modelRef, effort string) (string, string)
	maxSubagentDepth     int
	delegationLimits     DelegationLimits
	workspaceCoordinator *SubagentWorkspaceCoordinator
}

// NewTaskTool wires a task tool to the parent agent's environment so its
// sub-agents can use the same provider and tools. sysPrompt is the system
// prompt every sub-agent starts with; pass "" for DefaultTaskSystemPrompt. gate
// is the permission gate sub-agents inherit — pass the headless variant so
// deny rules still bite while autonomous sub-agents are never blocked on an
// interactive prompt (there is no UI to answer one).
func NewTaskTool(prov provider.Provider, pricing *provider.Pricing, parentReg *tool.Registry,
	maxSteps, contextWindow, recentKeep int, softCompactRatio, toolResultSnipRatio, compactRatio, compactForceRatio, temperature float64, archiveDir, sysPrompt string, gate Gate,
	keepPolicy KeepPolicy, subagentModel, subagentEffort string, resolveProvider func(string, string) (provider.Provider, *provider.Pricing, int, error)) *TaskTool {
	if sysPrompt == "" {
		sysPrompt = DefaultTaskSystemPrompt
	}
	return &TaskTool{
		prov:                prov,
		pricing:             pricing,
		parentReg:           parentReg,
		maxSteps:            maxSteps,
		contextWindow:       contextWindow,
		recentKeep:          recentKeep,
		softCompactRatio:    softCompactRatio,
		toolResultSnipRatio: toolResultSnipRatio,
		compactRatio:        compactRatio,
		compactForceRatio:   compactForceRatio,
		temperature:         temperature,
		archiveDir:          archiveDir,
		keepPolicy:          keepPolicy,
		sysPrompt:           sysPrompt,
		gate:                gate,
		subagentModel:       subagentModel,
		subagentEffort:      subagentEffort,
		resolveProvider:     resolveProvider,
		maxSubagentDepth:    DefaultMaxSubagentDepth,
	}
}

// WithTranscripts enables persisted sub-agent transcript continuation for this
// task tool. The base model/effort are the parent provider identity used when no
// subagent override is configured.
func (t *TaskTool) WithTranscripts(store *SubagentStore, workspaceRoot, baseModel, baseEffort string) *TaskTool {
	t.transcripts = store
	t.workspaceRoot = strings.TrimSpace(workspaceRoot)
	t.baseModel = strings.TrimSpace(baseModel)
	t.baseEffort = strings.TrimSpace(baseEffort)
	return t
}

func (t *TaskTool) WithTranscriptIdentityResolver(resolve func(modelRef, effort string) (string, string)) *TaskTool {
	t.identityProfile = resolve
	return t
}

func (t *TaskTool) WithMaxSubagentDepth(depth int) *TaskTool {
	t.maxSubagentDepth = NormalizeMaxSubagentDepth(depth)
	return t
}

// WithDelegationLimits configures the shared resource ledger created for each
// new foreground or background delegation tree.
func (t *TaskTool) WithDelegationLimits(limits DelegationLimits) *TaskTool {
	t.delegationLimits = limits
	return t
}

func (t *TaskTool) WithWorkspaceCoordinator(coordinator *SubagentWorkspaceCoordinator) *TaskTool {
	t.workspaceCoordinator = coordinator
	return t
}

func (t *TaskTool) Name() string { return "task" }

func (t *TaskTool) Description() string {
	return "Spawn a sub-agent for a focused sub-task. The sub-agent runs in its own session with the same provider and a filtered tool list (defaults to every parent tool, then applies the subagent boundary: " + subagentToolBoundarySummary + "). Only its final answer is returned. Use this to (a) keep long exploration sequences out of the parent's context budget, or (b) delegate self-contained work like 'find every place that calls X and summarise the patterns'."
}

func (t *TaskTool) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"properties":{
  "prompt":{"type":"string","description":"What the sub-agent should accomplish. Be specific about the deliverable — the sub-agent does not see this conversation."},
  "description":{"type":"string","description":"Short label for the sub-task (3-7 words). Surfaced in the dispatch line so the user sees what's running."},
  "tools":{"type":"array","items":{"type":"string"},"description":"Optional tool whitelist. ` + subagentToolBoundarySummary + `"},
  "max_steps":{"type":"integer","description":"Optional cap on tool-call rounds. Defaults to half the parent's cap (min 5).","minimum":1},
  "run_in_background":{"type":"boolean","description":"Run the sub-agent asynchronously: returns a job id immediately and keeps working across turns in the current process. Collect its final answer with wait. After a process crash, a persisted task is exposed as interrupted and must be resumed explicitly instead of replaying side effects automatically."},
  "model":{"type":"string","description":"Optional model override for the sub-agent (a configured provider/model name)."},
  "effort":{"type":"string","description":"Optional reasoning effort for the sub-agent (e.g. high, max)."},
  "continue_from":{"type":"string","description":"Continue a prior compatible completed or interrupted subagent transcript in the current conversation context. Pass only the 'sa_...' value from the prior result. Interrupted continuations are told to verify current workspace state before repeating side effects. If the ref belongs to an ancestor conversation, the framework continues a current-conversation copy."}
},
"required":["prompt"]
}`)
}

// ReadOnly is false: a sub-agent can invoke any whitelisted tool, including
// writers. Conservative classification keeps the parallel-dispatch path from
// running two sub-agents at once and letting their writes race.
func (t *TaskTool) ReadOnly() bool { return false }

// ResolveProfile extracts model/effort from task args and applies config defaults.
func (t *TaskTool) ResolveProfile(args json.RawMessage) *event.Profile {
	var p struct {
		Model  string `json:"model"`
		Effort string `json:"effort"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil
	}
	model, effort := t.effectiveProfile(p.Model, p.Effort)
	if model == "" && effort == "" {
		return nil
	}
	return &event.Profile{Model: model, Effort: effort}
}

// ReadOnlyTaskTool runs an isolated sub-agent with a strictly read-only tool
// registry. It intentionally omits background execution and transcript
// continuation/fork controls so the call has no durable host side effects.
type ReadOnlyTaskTool struct {
	task *TaskTool
}

func NewReadOnlyTaskTool(task *TaskTool) *ReadOnlyTaskTool {
	return &ReadOnlyTaskTool{task: task}
}

func (*ReadOnlyTaskTool) Name() string { return "read_only_task" }

func (*ReadOnlyTaskTool) Description() string {
	return "Spawn a read-only research sub-agent for a focused investigation. The sub-agent runs in an isolated, ephemeral session with read-only tools only; bash is wrapped to allow only plan-mode safe foreground commands. It cannot write files, install capabilities, mutate memory, run background jobs, continue/fork transcripts, or delegate to writer-capable agents. Read-only nested delegation may be available until max_subagent_depth is reached. Only its final answer is returned."
}

func (*ReadOnlyTaskTool) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"properties":{
  "prompt":{"type":"string","description":"What the read-only sub-agent should investigate. Be specific about the evidence or summary to return — the sub-agent does not see this conversation."},
  "description":{"type":"string","description":"Short label for the read-only sub-task (3-7 words). Surfaced in the dispatch line so the user sees what's running."},
  "tools":{"type":"array","items":{"type":"string"},"description":"Optional read-only tool whitelist. Writer, installer, memory mutation, background job, and delegation tools are never exposed."},
  "max_steps":{"type":"integer","description":"Optional cap on tool-call rounds. Defaults to half the parent's cap (min 5).","minimum":1},
  "model":{"type":"string","description":"Optional model override for the sub-agent (a configured provider/model name)."},
  "effort":{"type":"string","description":"Optional reasoning effort for the sub-agent (e.g. high, max)."}
},
"required":["prompt"]
}`)
}

func (*ReadOnlyTaskTool) ReadOnly() bool { return true }

// PlanModeSafe reports true: read_only_task spawns a strictly read-only research
// sub-agent (no writers, installers, memory mutation, background jobs, or
// delegation), so it is safe to run while planning.
func (*ReadOnlyTaskTool) PlanModeSafe() bool { return true }

func (r *ReadOnlyTaskTool) ResolveProfile(args json.RawMessage) *event.Profile {
	if r == nil || r.task == nil {
		return nil
	}
	return r.task.ResolveProfile(args)
}

func (r *ReadOnlyTaskTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	if r == nil || r.task == nil {
		return "", fmt.Errorf("read_only_task is not configured")
	}
	var p struct {
		Prompt      string   `json:"prompt"`
		Description string   `json:"description"`
		Tools       []string `json:"tools"`
		MaxSteps    int      `json:"max_steps"`
		Model       string   `json:"model"`
		Effort      string   `json:"effort"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if strings.TrimSpace(p.Prompt) == "" {
		return "", fmt.Errorf("prompt is required")
	}

	maxSteps := p.MaxSteps
	if maxSteps <= 0 && r.task.maxSteps > 0 {
		maxSteps = r.task.maxSteps / 2
		if maxSteps < 5 {
			maxSteps = 5
		}
	}

	childDepth, err := r.task.nextSubagentDepth(ctx)
	if err != nil {
		return "", err
	}
	parentReg := ToolRegistryFromContext(ctx, r.task.parentReg)
	subReg := ReadOnlySubagentToolRegistryForDepth(parentReg, p.Tools, childDepth, r.task.maxDepth())
	if subReg.Len() == 0 {
		return "", fmt.Errorf("read_only_task has no read-only tools available")
	}
	modelRef, effortRef := r.task.effectiveProfile(p.Model, p.Effort)
	prov, pricing, ctxWin, err := r.task.resolveSubSessionRuntime(modelRef, effortRef)
	if err != nil {
		return "", fmt.Errorf("read-only sub-agent profile: %w", err)
	}
	workspaceRoot := WorkspaceRootFromContext(ctx, r.task.workspaceRoot)
	return r.task.runSubSession(ctx, p.Prompt, subReg, subSink(ctx), maxSteps, prov, pricing, ctxWin, NewSession(DefaultReadOnlyTaskSystemPrompt), childDepth, nil, workspaceRoot, nil)
}

func (t *TaskTool) effectiveProfile(model, effort string) (string, string) {
	model = strings.TrimSpace(model)
	effort = strings.TrimSpace(effort)
	if model == "" {
		model = strings.TrimSpace(t.subagentModel)
	}
	if effort == "" {
		effort = strings.TrimSpace(t.subagentEffort)
	}
	return model, effort
}

func (t *TaskTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Prompt          string   `json:"prompt"`
		Description     string   `json:"description"`
		Tools           []string `json:"tools"`
		MaxSteps        int      `json:"max_steps"`
		RunInBackground bool     `json:"run_in_background"`
		Model           string   `json:"model"`
		Effort          string   `json:"effort"`
		ContinueFrom    string   `json:"continue_from"`
		ForkFrom        string   `json:"fork_from"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if p.Prompt == "" {
		return "", fmt.Errorf("prompt is required")
	}

	maxSteps := p.MaxSteps
	if maxSteps <= 0 {
		// No explicit cap from the caller: mirror the parent. A finite parent caps
		// the sub-agent at half its budget (min 5) so a delegated sub-task stays
		// shorter than the whole turn; an unbounded parent yields an unbounded
		// sub-agent. The sub-agent shares the parent's ctx, so cancelling the turn
		// stops it, and it compacts its own context — the same bounds the parent has.
		if t.maxSteps > 0 {
			maxSteps = t.maxSteps / 2
			if maxSteps < 5 {
				maxSteps = 5
			}
		}
	}

	childDepth, err := t.nextSubagentDepth(ctx)
	if err != nil {
		return "", err
	}
	var jobManager *jobs.Manager
	if p.RunInBackground {
		var ok bool
		jobManager, ok = jobs.FromContext(ctx)
		if !ok {
			return "", fmt.Errorf("background execution is not available in this context")
		}
	}
	modelRef, effortRef := t.effectiveProfile(p.Model, p.Effort)
	prov, pricing, ctxWin, err := t.resolveSubSessionRuntime(modelRef, effortRef)
	if err != nil {
		return "", fmt.Errorf("sub-agent profile: %w", err)
	}
	sourceRoot := WorkspaceRootFromContext(ctx, t.workspaceRoot)
	parentReg := ToolRegistryFromContext(ctx, t.parentReg)
	baseReg := t.buildSubRegFrom(parentReg, p.Tools, childDepth)
	workspace := SubagentWorkspace{Mode: SubagentWorkspaceSharedReadOnly, SourceRoot: sourceRoot, ExecutionRoot: sourceRoot}
	var workspaceLease *workspacelease.Owner
	continueRef := strings.TrimSpace(p.ContinueFrom)
	legacyForkRef := strings.TrimSpace(p.ForkFrom)
	freshWriterWorkspace := false
	writer := RegistryCanWrite(baseReg)
	if writer && t.workspaceCoordinator != nil && strings.TrimSpace(ParentSession(ctx)) == "" {
		return "", errors.New("writer subagents require a persisted parent session so their isolated delivery can be previewed and accepted; use read_only_task in ephemeral headless runs")
	}
	if writer && t.workspaceCoordinator != nil {
		lookupRef := continueRef
		if lookupRef == "" {
			lookupRef = legacyForkRef
		}
		if lookupRef != "" {
			meta, loadErr := t.transcripts.LoadMeta(lookupRef)
			if loadErr != nil {
				return "", loadErr
			}
			if meta.Workspace.Mode != SubagentWorkspaceGitWorktree {
				return "", fmt.Errorf("subagent reference %q predates isolated writer workspaces; start a fresh writer subagent", lookupRef)
			}
			if DeliveryBlocksContinuation(meta.Delivery.Status) {
				return "", fmt.Errorf("subagent delivery %q is already %s; start a fresh writer subagent", lookupRef, meta.Delivery.Status)
			}
			workspace = meta.Workspace
			workspaceLease, err = t.workspaceCoordinator.Resume(meta)
			if err != nil {
				return "", fmt.Errorf("resume subagent workspace %q: %w", lookupRef, err)
			}
		} else {
			workspace, workspaceLease, err = t.workspaceCoordinator.CreateWriter(ctx, sourceRoot)
			if err != nil {
				return "", err
			}
			freshWriterWorkspace = true
		}
	}
	subReg := baseReg
	if t.workspaceCoordinator != nil {
		subReg, err = t.workspaceCoordinator.Registry(parentReg, p.Tools, childDepth, t.maxDepth(), workspace)
		if err != nil {
			if freshWriterWorkspace {
				t.workspaceCoordinator.RejectPreparation(workspace)
			}
			return "", fmt.Errorf("bind subagent tools to isolated workspace: %w", err)
		}
	}
	parentID, parent, _, _ := CallContext(ctx)
	run, err := t.prepareTranscriptRun(subReg, modelRef, effortRef, ParentSession(ctx), parentID, p.ContinueFrom, p.ForkFrom, sourceRoot, workspace)
	if err != nil {
		if freshWriterWorkspace {
			t.workspaceCoordinator.RejectPreparation(workspace)
		}
		return "", err
	}
	abortBeforeExecution := func(cause error) error {
		var cleanupErr error
		if freshWriterWorkspace {
			cleanupErr = t.transcripts.AbortFreshPreparation(run)
		} else {
			cleanupErr = t.transcripts.SaveInterrupted(run)
		}
		return errors.Join(cause, cleanupErr)
	}
	if t.transcripts != nil && run != nil && run.Ref != "" {
		if err := t.transcripts.MarkRunning(run); err != nil {
			persistErr := abortBeforeExecution(fmt.Errorf("persist subagent start %q: %w", run.Ref, err))
			run.Release()
			return "", persistErr
		}
		if effects, ok := SubagentEffectsFromContext(ctx); ok {
			if workspace.Mode == SubagentWorkspaceGitWorktree {
				effects = effects.isolatedWorkspaceEffects()
			}
			bound, err := effects.withJournal(run)
			if err != nil {
				persistErr := abortBeforeExecution(fmt.Errorf("persist subagent effect journal %q: %w", run.Ref, err))
				run.Release()
				return "", persistErr
			}
			ctx = WithSubagentEffects(ctx, bound)
		}
	}
	sessionSync := t.sessionSync(run)
	subPrompt := p.Prompt
	if run != nil && run.ResumedFromInterrupted {
		subPrompt = interruptedSubagentResumeContext + "\n\n" + subPrompt
	}

	// Background: register a job that runs the sub-agent under the manager's
	// session context (so it survives this turn) and return immediately. The
	// sub-agent's tool activity still streams, nested under this call, because the
	// nested sink captures the parent ID + stream now (not from the job ctx).
	if p.RunInBackground {
		nested := subSinkFor(parentID, parent)
		effects, _ := SubagentEffectsFromContext(ctx)
		label := p.Description
		if label == "" {
			label = "task"
		}
		runJob := func(jobCtx context.Context, _ io.Writer) (result string, err error) {
			jobCtx = WithSubagentEffects(jobCtx, effects)
			defer run.Release()
			defer func() {
				if r := recover(); r != nil {
					panicErr := fmt.Errorf("internal error: panic: %v\n%s", r, debug.Stack())
					result = FormatSubagentRunResult(panicErr.Error(), run, true)
					err = errors.Join(panicErr, t.transcripts.SaveFailed(run))
				}
			}()
			answer, err := t.runSubSession(jobCtx, subPrompt, subReg, nested, maxSteps, prov, pricing, ctxWin, run.Session, childDepth, sessionSync, workspace.ExecutionRoot, workspaceLease)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return FormatSubagentRunResult(err.Error(), run, true), errors.Join(err, t.transcripts.SaveInterrupted(run))
				}
				return FormatSubagentRunResult(err.Error(), run, true), errors.Join(err, t.transcripts.SaveFailed(run))
			}
			if err := t.transcripts.SaveCompleted(run); err != nil {
				return FormatSubagentRunResult("", run, true), errors.Join(err, t.transcripts.SaveFailed(run))
			}
			return FormatSubagentRunResult(answer, run, false), nil
		}
		var job *jobs.Job
		if run != nil && run.Ref != "" {
			job, err = jobManager.StartRecoverableForSession(jobs.SessionFromContext(ctx), "task", label, run.Ref, runJob)
			if err != nil {
				persistErr := abortBeforeExecution(fmt.Errorf("start recoverable background task for subagent %q: %w; retry with continue_from %q", run.Ref, err, run.Ref))
				run.Release()
				return "", persistErr
			}
		} else {
			job = jobManager.StartForSession(jobs.SessionFromContext(ctx), "task", label, runJob)
		}
		if run != nil && run.Ref != "" {
			return fmt.Sprintf("Started background task %q (%s).\n%s\nIt runs across turns; collect its final answer with wait (or wait will return it once done), and you'll be notified when it finishes.", job.ID, label, FormatSubagentReference(run)), nil
		}
		return fmt.Sprintf("Started background task %q (%s). It runs across turns; collect its final answer with wait (or wait will return it once done), and you'll be notified when it finishes.", job.ID, label), nil
	}

	// Foreground: run synchronously, nesting events under this call.
	defer run.Release()
	answer, err := t.runSubSession(ctx, subPrompt, subReg, subSink(ctx), maxSteps, prov, pricing, ctxWin, run.Session, childDepth, sessionSync, workspace.ExecutionRoot, workspaceLease)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return "", errors.Join(err, t.transcripts.SaveInterrupted(run))
		}
		return "", errors.Join(err, t.transcripts.SaveFailed(run))
	}
	if t.transcripts != nil && run.Ref != "" {
		if err := t.transcripts.SaveCompleted(run); err != nil {
			return "", errors.Join(err, t.transcripts.SaveFailed(run))
		}
		return FormatSubagentRunResult(answer, run, false), nil
	}
	return answer, nil
}

func (t *TaskTool) prepareTranscriptRun(subReg *tool.Registry, modelRef, effortRef, parentSession, parentID, continueFrom, legacyForkFrom, sourceRoot string, workspace SubagentWorkspace) (*SubagentRun, error) {
	continueFrom = strings.TrimSpace(continueFrom)
	legacyForkFrom = strings.TrimSpace(legacyForkFrom)
	parentSession = strings.TrimSpace(parentSession)
	if continueFrom != "" && legacyForkFrom != "" {
		return nil, fmt.Errorf("continue_from and fork_from are mutually exclusive; pass only continue_from")
	}
	if t.transcripts == nil {
		return nil, fmt.Errorf("subagent transcript store is required")
	}
	// Headless runs (e.g. `reames-agent run`) never mint a session path, so there is
	// no parent session to own a transcript. Run the sub-agent ephemerally —
	// exactly as before persisted transcripts existed — instead of failing the
	// call. Continuation/fork need a persisted owner, so they error here.
	if parentSession == "" {
		if continueFrom != "" || legacyForkFrom != "" {
			return nil, fmt.Errorf("subagent continuation requires a persisted session; none is active in this run")
		}
		return EphemeralSubagentRun(t.sysPrompt), nil
	}
	identityModel, identityEffort := t.effectiveIdentity(modelRef, effortRef)
	spec := SubagentSpec{
		Kind:             "task",
		Name:             "task",
		WorkspaceRoot:    sourceRoot,
		ParentSession:    parentSession,
		ParentToolCallID: parentID,
		SystemPrompt:     t.sysPrompt,
		Registry:         subReg,
		Model:            identityModel,
		Effort:           identityEffort,
		Workspace:        workspace,
	}
	if continueFrom != "" {
		return t.transcripts.PrepareContinue(continueFrom, spec)
	}
	if legacyForkFrom != "" {
		return t.transcripts.PrepareLegacyForkFrom(legacyForkFrom, spec)
	}
	return t.transcripts.PrepareFresh(spec)
}

func (t *TaskTool) effectiveIdentity(modelRef, effort string) (string, string) {
	if t.identityProfile != nil {
		model, eff := t.identityProfile(modelRef, effort)
		return strings.TrimSpace(model), strings.TrimSpace(eff)
	}
	return t.effectiveModelIdentity(modelRef), t.effectiveEffortIdentity(effort)
}

func (t *TaskTool) effectiveModelIdentity(modelRef string) string {
	if strings.TrimSpace(modelRef) != "" {
		return strings.TrimSpace(modelRef)
	}
	return strings.TrimSpace(t.baseModel)
}

func (t *TaskTool) effectiveEffortIdentity(effort string) string {
	if strings.TrimSpace(effort) != "" {
		return strings.TrimSpace(effort)
	}
	return strings.TrimSpace(t.baseEffort)
}

// buildSubReg returns the sub-agent's tool set: the named whitelist (minus
// unavailable sub-agent tools), or every parent tool except those tools.
func (t *TaskTool) buildSubReg(names []string, childDepth int) *tool.Registry {
	return t.buildSubRegFrom(t.parentReg, names, childDepth)
}

func (t *TaskTool) buildSubRegFrom(parent *tool.Registry, names []string, childDepth int) *tool.Registry {
	return SubagentToolRegistryForDepth(parent, names, childDepth, t.maxDepth())
}

func (t *TaskTool) maxDepth() int {
	if t == nil {
		return DefaultMaxSubagentDepth
	}
	if t.maxSubagentDepth == 0 {
		return DefaultMaxSubagentDepth
	}
	return NormalizeMaxSubagentDepth(t.maxSubagentDepth)
}

func (t *TaskTool) nextSubagentDepth(ctx context.Context) (int, error) {
	current := SubagentDepth(ctx)
	next := current + 1
	maxDepth := t.maxDepth()
	if next > maxDepth {
		return 0, fmt.Errorf("subagent delegation depth limit reached (max_subagent_depth=%d)", maxDepth)
	}
	return next, nil
}

// FilterRegistry builds a sub-registry from parent: the named whitelist (empty =
// every parent tool), minus any excluded names. Used to scope what a spawned
// sub-agent — a `task` sub-agent or a subagent skill — may call, e.g. excluding
// `task` to bar recursive nesting, or restricting to a skill's allowed-tools.
func FilterRegistry(parent *tool.Registry, names []string, exclude ...string) *tool.Registry {
	sub := tool.NewRegistry()
	if parent == nil {
		return sub
	}
	ex := make(map[string]bool, len(exclude))
	for _, e := range exclude {
		ex[e] = true
	}
	src := names
	if len(src) == 0 {
		src = parent.Names()
	}
	for _, name := range src {
		if ex[name] {
			continue
		}
		if tl, ok := parent.Get(name); ok {
			sub.Add(tl)
		}
	}
	return sub
}

var plannerNonResearchTools = []string{
	"ask",
	"bash_output",
	"complete_step",
	"slash_command",
	"todo_write",
	"wait",
}

// PlannerToolRegistry returns the tool set exposed to the two-model planner:
// read-only research tools only. It deliberately excludes workflow/meta tools
// that are technically read-only but can prompt the user, update visible task
// state, wait on jobs, or expand commands instead of inspecting context.
func PlannerToolRegistry(parent *tool.Registry) *tool.Registry {
	exclude := append(SubagentMetaTools(), plannerNonResearchTools...)
	return FilterReadOnlyRegistry(parent, exclude...)
}

// ReadOnlySubagentToolRegistry returns the tool set exposed to read-only
// sub-agents: read-only research tools plus a bash wrapper that enforces the
// plan-mode safe command policy at execution time. Workflow/meta tools are
// excluded even when their Tool.ReadOnly contract is true.
func ReadOnlySubagentToolRegistry(parent *tool.Registry, names []string) *tool.Registry {
	return ReadOnlySubagentToolRegistryForDepth(parent, names, 1, 1)
}

// ReadOnlySubagentToolRegistryForDepth returns the tool set exposed to read-only
// subagents. It permits only read-only delegation tools while another depth
// layer is available.
func ReadOnlySubagentToolRegistryForDepth(parent *tool.Registry, names []string, childDepth, maxDepth int) *tool.Registry {
	exclude := append([]string(nil), subagentAlwaysHiddenTools...)
	if childDepth >= NormalizeMaxSubagentDepth(maxDepth) {
		exclude = append(exclude, subagentRecursiveTools...)
	} else {
		exclude = append(exclude, "task", "run_skill", "explore", "research", "review", "security_review")
	}
	exclude = append(exclude, subagentJobTools...)
	exclude = append(exclude, plannerNonResearchTools...)
	exclude = append(exclude, readOnlySubagentWorkflowTools...)
	ex := make(map[string]bool, len(exclude))
	for _, e := range exclude {
		ex[e] = true
	}
	sub := tool.NewRegistry()
	if parent == nil {
		return sub
	}
	src := names
	if len(src) == 0 {
		src = parent.Names()
	}
	for _, name := range src {
		if ex[name] {
			continue
		}
		tl, ok := parent.Get(name)
		if !ok {
			continue
		}
		if name == "bash" {
			sub.Add(readOnlyBash{inner: tl})
			continue
		}
		if !tl.ReadOnly() {
			continue
		}
		if u, ok := tl.(tool.PlanModeUntrustedReadOnly); ok && u.PlanModeUntrustedReadOnly() {
			// An external tool's self-reported readOnlyHint isn't trusted for a
			// read-only research sub-agent; exclude it like a writer.
			continue
		}
		sub.Add(tl)
	}
	return sub
}

// FilterReadOnlyRegistry builds a sub-registry containing only tools whose
// ReadOnly contract is true, minus explicit exclusions.
func FilterReadOnlyRegistry(parent *tool.Registry, exclude ...string) *tool.Registry {
	ex := make(map[string]bool, len(exclude))
	for _, e := range exclude {
		ex[e] = true
	}
	sub := tool.NewRegistry()
	if parent == nil {
		return sub
	}
	for _, name := range parent.Names() {
		if ex[name] {
			continue
		}
		tl, ok := parent.Get(name)
		if !ok || !tl.ReadOnly() {
			continue
		}
		if u, ok := tl.(tool.PlanModeUntrustedReadOnly); ok && u.PlanModeUntrustedReadOnly() {
			continue
		}
		sub.Add(tl)
	}
	return sub
}

func (t *TaskTool) resolveSubSessionRuntime(modelRef, effort string) (provider.Provider, *provider.Pricing, int, error) {
	prov, pricing, ctxWin := t.prov, t.pricing, t.contextWindow
	if t.resolveProvider != nil && (modelRef != "" || effort != "") {
		p, pr, cw, err := t.resolveProvider(modelRef, effort)
		if err != nil {
			return nil, nil, 0, err
		}
		prov, pricing, ctxWin = p, pr, cw
	}
	return prov, pricing, ctxWin, nil
}

func (t *TaskTool) runSubSession(ctx context.Context, prompt string, subReg *tool.Registry, sink event.Sink, maxSteps int, prov provider.Provider, pricing *provider.Pricing, ctxWin int, sess *Session, childDepth int, sessionSync SessionSync, workspaceRoot string, workspaceLease *workspacelease.Owner) (string, error) {
	ctx, ledger, cleanup := EnsureDelegationLedger(ctx, t.delegationLimits)
	defer cleanup()
	effects, _ := SubagentEffectsFromContext(ctx)
	prompt = t.withWorkspaceContext(prompt, workspaceRoot)
	return RunSubAgentWithSession(ctx, prov, subReg, sess, prompt, Options{
		MaxSteps:            maxSteps,
		Temperature:         t.temperature,
		Pricing:             pricing,
		UsageSource:         event.UsageSourceSubagent,
		Gate:                t.gate,
		ContextWindow:       ctxWin,
		RecentKeep:          t.recentKeep,
		SoftCompactRatio:    t.softCompactRatio,
		ToolResultSnipRatio: t.toolResultSnipRatio,
		CompactRatio:        t.compactRatio,
		CompactForceRatio:   t.compactForceRatio,
		ArchiveDir:          t.archiveDir,
		KeepPolicy:          t.keepPolicy,
		ResponseLanguage:    ResponseLanguageFromContext(ctx),
		ReasoningLanguage:   ReasoningLanguageFromContext(ctx),
		SubagentDepth:       childDepth,
		MaxSubagentDepth:    t.maxDepth(),
		DelegationLedger:    ledger,
		SubagentEffects:     effects,
		SessionSync:         sessionSync,
		WorkspaceRoot:       workspaceRoot,
		WorkspaceLease:      workspaceLease,
	}, sink)
}

func (t *TaskTool) sessionSync(run *SubagentRun) SessionSync {
	if t == nil || t.transcripts == nil || run == nil || run.Ref == "" {
		return nil
	}
	return func(current *Session) error {
		if current != run.Session {
			return fmt.Errorf("subagent %q recovery session changed unexpectedly", run.Ref)
		}
		return t.transcripts.SaveRunning(run)
	}
}

const interruptedSubagentResumeContext = `<recovery-context event="InterruptedSubagentResume">
The previous process stopped after a durable transcript boundary. A pending tool call may have completed, partially completed, or not started.
Do not replay side-effecting work from the transcript blindly. Re-read the current workspace and verify external state before deciding what remains.
</recovery-context>`

func (t *TaskTool) withWorkspaceContext(prompt, workspaceRoot string) string {
	if t == nil {
		return prompt
	}
	ctx := subagentWorkspaceContext(workspaceRoot)
	if ctx == "" {
		return prompt
	}
	return ctx + "\n\n" + prompt
}

func subagentWorkspaceContext(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return ""
	}
	return `<workspace-context event="SubagentWorkspace">
Current workspace: ` + strconv.Quote(root) + `
File tools resolve relative paths against this workspace. For project inspection, prefer "." or relative paths unless the user explicitly named another absolute path.
</workspace-context>`
}

func FormatSubagentReference(run *SubagentRun) string {
	if run == nil || run.Ref == "" {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Subagent reference: %s\n", run.Ref)
	if strings.TrimSpace(run.ForkedFrom) != "" {
		fmt.Fprintf(&b, "Forked from: %s\n", strings.TrimSpace(run.ForkedFrom))
		b.WriteString("The requested ref resolves to an ancestor conversation transcript, so the framework continues a copy owned by the current conversation. To continue this copied subagent transcript in a later call, pass ")
		b.WriteString(run.Ref)
		b.WriteString(" as `continue_from`. Start a fresh subagent when the next task is independent.")
		return b.String()
	}
	b.WriteString("To continue this same subagent transcript in a later call, pass this ref as `continue_from`. Start a fresh subagent when the next task is independent.")
	return b.String()
}

func FormatSubagentRunResult(answer string, run *SubagentRun, failed bool) string {
	if run == nil || run.Ref == "" {
		return answer
	}
	if failed {
		if answer == "" {
			return "Subagent reference (failed): " + run.Ref
		}
		return "Subagent reference (failed): " + run.Ref + "\n\nFinal answer:\n" + answer
	}
	result := FormatSubagentReference(run)
	if run.Meta.Workspace.Mode == SubagentWorkspaceGitWorktree {
		switch run.Meta.Delivery.Status {
		case SubagentDeliveryReady:
			result += "\nDelivery: ready in isolated branch " + run.Meta.Workspace.Branch + ". Preview with subagent_delivery_preview, then choose subagent_delivery apply, merge, or reject. Source files are unchanged until acceptance."
		case SubagentDeliveryEmpty:
			result += "\nDelivery: no file or commit changes were produced."
		}
	}
	return result + "\n\nFinal answer:\n" + answer
}

// RunSubAgentWithSession continues an existing sub-agent session with prompt and
// returns the latest final assistant answer. Fresh sub-agents pass a newly-created
// session; continued sub-agents pass a loaded transcript session.
func RunSubAgentWithSession(ctx context.Context, prov provider.Provider, reg *tool.Registry, sess *Session, prompt string, opts Options, sink event.Sink) (string, error) {
	if sess == nil {
		return "", fmt.Errorf("sub-agent session is nil")
	}
	if opts.SubagentDepth > 0 {
		ctx = WithSubagentDepth(ctx, opts.SubagentDepth)
	}
	if opts.SubagentDepth > 0 && isFreshSubagentSession(sess) {
		prompt = subagentStartContext + "\n\n" + prompt
	}
	sub := New(prov, reg, sess, opts, sink)
	if err := sub.Run(ctx, prompt); err != nil {
		if opts.DelegationLedger != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
			if cause := opts.DelegationLedger.Cause(); cause != nil {
				return "", fmt.Errorf("sub-agent: %w", opts.DelegationLedger.budgetError(cause))
			}
		}
		return "", fmt.Errorf("sub-agent: %w", err)
	}
	// Walk the session backwards for the last assistant message with content —
	// that's the sub-agent's final answer. Intermediate assistant messages with
	// tool_calls but no text don't count.
	for i := len(sess.Messages) - 1; i >= 0; i-- {
		m := sess.Messages[i]
		if m.Role == provider.RoleAssistant && strings.TrimSpace(m.Content) != "" {
			return m.Content, nil
		}
	}
	return "", fmt.Errorf("sub-agent finished without producing a final answer")
}

func isFreshSubagentSession(sess *Session) bool {
	if sess == nil {
		return false
	}
	snap := sess.Snapshot()
	return len(snap) == 1 && snap[0].Role == provider.RoleSystem
}

// NestedSink returns a sink that forwards a sub-agent's tool activity to the
// parent stream, nested under the tool call carried by ctx, so a frontend shows
// it beneath that call (the same nesting `task` uses). Falls back to the given
// sink when ctx carries no call context. Used by subagent skills.
func NestedSink(ctx context.Context, fallback event.Sink) event.Sink {
	parentID, parent, _, ok := CallContext(ctx)
	if !ok || parent == nil {
		return fallback
	}
	return subSinkFor(parentID, parent)
}

// subSink forwards a sub-agent's tool dispatch/result events and billable usage
// to the parent's event stream. Only tool activity is nested visually; the
// sub-agent's text/reasoning stays isolated and only its final answer is returned.
//
// The sub-agent's own turn/text/reasoning events are dropped — forwarding them
// would make the parent transcript noisy and could imply they belong to the
// parent model context, which they do not.
//
// Usage events are observability only, so forwarding them preserves billing
// totals without polluting the parent provider-visible prefix.
//
// Tool events are tagged with the parent task call's ID so a frontend nests them
// under it. The forwarded call IDs are namespaced with the parent ID so a
// sub-agent call can never collide with a parent call in the frontend's
// dispatch→result matching. Falls back to Discard when there's no parent stream
// (the headless run loop, or a direct Execute in tests).
func subSink(ctx context.Context) event.Sink {
	parentID, parent, _, ok := CallContext(ctx)
	if !ok || parent == nil {
		return event.Discard
	}
	return subSinkFor(parentID, parent)
}

// subSinkFor builds the nesting sink from an already-captured parent ID + stream,
// for the background path where the job runs under a context that no longer
// carries the call context. Falls back to Discard when there's no parent stream.
func subSinkFor(parentID string, parent event.Sink) event.Sink {
	if parent == nil {
		return event.Discard
	}
	return event.FuncSink(func(e event.Event) {
		switch e.Kind {
		case event.ToolDispatch, event.ToolResult:
			e.Tool.ParentID = parentID
			e.Tool.ID = parentID + "/" + e.Tool.ID
			parent.Emit(e)
		case event.Usage:
			if e.UsageSource == "" {
				e.UsageSource = event.UsageSourceSubagent
			}
			parent.Emit(e)
		}
	})
}
