package control

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"reames-agent/internal/agent"
	"reames-agent/internal/autoresearch"
	"reames-agent/internal/jobs"
	"reames-agent/internal/provider"
)

// turnOrchestrator owns foreground turn execution while Controller keeps the
// public ports, run-state guard, and session-scoped dependencies.
type turnOrchestrator struct {
	c *Controller
}

type orchestratedTurn struct {
	input          string
	raw            string
	display        string
	editedOriginal string
	synthetic      bool
	headless       bool
}

func newTurnOrchestrator(c *Controller) *turnOrchestrator {
	return &turnOrchestrator{c: c}
}

func (o *turnOrchestrator) runTurnWithRawDisplay(ctx context.Context, input, raw, display string) error {
	return o.runOrchestratedTurn(ctx, orchestratedTurn{input: input, raw: raw, display: display})
}

func (o *turnOrchestrator) runEditedTurnWithRawDisplay(ctx context.Context, input, raw, display, original string) error {
	return o.runOrchestratedTurn(ctx, orchestratedTurn{input: input, raw: raw, display: display, editedOriginal: original})
}

func (o *turnOrchestrator) runSyntheticTurnWithRawDisplay(ctx context.Context, input, raw, display string) error {
	return o.runOrchestratedTurn(ctx, orchestratedTurn{input: input, raw: raw, display: display, synthetic: true})
}

func (o *turnOrchestrator) runObservedSyntheticTurn(ctx context.Context, input, raw, display string, headless bool) (bool, error) {
	start := o.c.messageCount()
	if err := o.runOrchestratedTurn(ctx, orchestratedTurn{input: input, raw: raw, display: display, synthetic: true, headless: headless}); err != nil {
		return false, err
	}
	history := o.c.History()
	if start < 0 || start > len(history) {
		return false, nil
	}
	// A strict self-check is proven only by this turn's final assistant answer.
	// An assistant tool-call envelope is not a final: a custom runner may return
	// after writing one, and reusing the previous turn's [goal:complete] would
	// otherwise let that partial turn satisfy strict completion.
	for i := len(history) - 1; i >= start; i-- {
		msg := history[i]
		if msg.Role != provider.RoleAssistant {
			continue
		}
		return strings.TrimSpace(msg.Content) != "" && len(msg.ToolCalls) == 0, nil
	}
	return false, nil
}

func (o *turnOrchestrator) runComposedSyntheticTurn(ctx context.Context, text string) error {
	c := o.c
	return c.runner.Run(agent.WithMemoryCompilerSkip(ctx), c.ComposeSynthetic(text))
}

func (o *turnOrchestrator) runOrchestratedTurn(ctx context.Context, turn orchestratedTurn) (runErr error) {
	c := o.c
	if err := c.recoverPendingSessionTransactions(c.SessionPath()); err != nil {
		return fmt.Errorf("recover session before new turn: %w", err)
	}
	c.maybeSessionStart(ctx)
	if !turn.synthetic && !turn.headless {
		c.maybeAutoPlan(ctx, turn.raw)
	}
	parentSession := c.parentSessionID()
	ctx = agent.WithParentSession(ctx, parentSession)
	ctx = jobs.WithSession(ctx, parentSession)
	ctx = agent.WithUserImages(ctx, c.inputImages(turn.input))
	// Synthetic, controller-injected turns (goal-loop continuation,
	// plan-approved execution, …) must not be Memory v5-compiled: compiling them
	// re-injects a contract the model echoes back, which spins the goal loop
	// forever (#5342, #5329). Only genuine user turns supply a compiler source.
	if turn.synthetic || IsSyntheticUserMessage(turn.raw) {
		ctx = agent.WithMemoryCompilerSkip(ctx)
	} else {
		ctx = agent.WithMemoryCompilerSourceInput(ctx, turn.raw)
	}
	input := c.compose(turn.input, turn.raw, !turn.synthetic)
	startMessages := c.messageCount()
	turnMarked := false
	defer func() {
		if !turnMarked {
			if runErr == nil {
				c.snapshotActivityIfChanged(startMessages)
				if c.executor != nil {
					if err := c.goals.persistRuntime(c.goalRuntimeProjection()); err != nil {
						slog.Warn("controller: persist unarmed turn runtime", "err", err)
					}
				}
			} else if errors.Is(runErr, context.Canceled) && c.CancelRequested() {
				var err error
				if turn.synthetic || IsSyntheticUserMessage(turn.raw) {
					err = c.stripTurnMessagesAfter(startMessages)
				} else {
					err = c.stripCancelledVisibleTurnMessagesAfter(startMessages)
				}
				if err != nil {
					runErr = errors.Join(runErr, err)
				}
			}
			return
		}
		if runErr == nil {
			if err := c.commitInFlightTurn(startMessages); err != nil {
				runErr = err
				if recoverErr := c.recoverInterruptedTurnState(c.SessionPath()); recoverErr != nil {
					runErr = errors.Join(runErr, fmt.Errorf("recover failed turn commit: %w", recoverErr))
				}
			}
			return
		}
		visibleTurn := !turn.synthetic && !IsSyntheticUserMessage(turn.raw)
		fallback := provider.Message{
			Role: provider.RoleUser, Content: input,
			Images: append([]string(nil), c.inputImages(turn.input)...), CreatedAt: time.Now().UnixMilli(),
		}
		// Graceful stops and provider failures with durable partial display keep
		// complete tool pairs and convert unsafe fragments to LocalOnly records.
		// Startup/crash recovery still follows the stricter checkpoint rollback
		// below because the process cannot prove opaque side effects completed.
		if visibleTurn && ((errors.Is(runErr, context.Canceled) && c.CancelRequested()) ||
			provider.IsStreamInterrupted(runErr) || c.hasInterruptedDisplayAfter(startMessages, fallback)) {
			if err := c.preserveAndCommitInterruptedTurn(startMessages, fallback); err == nil {
				return
			} else {
				runErr = errors.Join(runErr, err)
			}
		}
		if err := c.recoverInterruptedTurnState(c.SessionPath()); err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("rollback interrupted turn: %w", err))
		}
	}()
	defer c.recordDisplayForNewUser(startMessages, turn.display)
	if turn.editedOriginal != "" {
		defer c.markEditedForNewUser(startMessages, turn.editedOriginal)
	}
	// Every orchestrated turn gets a recovery boundary before its user message
	// is appended. Synthetic continuations use hidden checkpoints: they can roll
	// back only their own workspace effects without appearing in user pickers.
	if err := c.beginCheckpoint(input, turn.synthetic); err != nil {
		return fmt.Errorf("begin turn recovery checkpoint: %w", err)
	}
	if c.guardianSess != nil {
		c.guardianSess.ResetTurn()
	}
	// UserPromptSubmit / Stop hooks bracket the whole turn (incl. the plan
	// research + approved-execution sub-turns below): a gating UserPromptSubmit
	// aborts before any model call; Stop fires once when the turn returns.
	if c.hooks.Enabled() {
		c.mu.Lock()
		c.turn++
		turn := c.turn
		c.mu.Unlock()
		if block, _ := c.hooks.PromptSubmit(ctx, input, turn); block {
			return nil // the hook's notify callback already surfaced the reason
		}
		defer func() { c.hooks.Stop(context.Background(), lastAssistantText(c.History()), turn) }()
	}
	if err := c.markInFlightTurn(startMessages, !turn.synthetic && !IsSyntheticUserMessage(turn.raw)); err != nil {
		if checkpointTurn, ok := c.checkpoints.currentTurn(); ok {
			if cleanupErr := c.checkpoints.truncateFrom(checkpointTurn); cleanupErr != nil {
				return errors.Join(err, fmt.Errorf("retire unarmed checkpoint turn %d: %w", checkpointTurn, cleanupErr))
			}
		}
		return err
	}
	turnMarked = true
	autoResearchTaskID := c.goals.currentAutoResearchTaskID()
	autoResearchAcceptedBefore := c.autoResearchAcceptedEvidenceIDs(autoResearchTaskID)
	c.appendAutoResearchHeartbeat(autoResearchTaskID, autoresearch.HeartbeatStartingTurn, "")
	modelInput := input
	if !turn.synthetic {
		modelInput = c.withCapabilityRoute(input, turn.raw)
	}
	err := c.runner.Run(ctx, modelInput)
	if err == nil {
		c.recordAutoResearchEvidenceFromAssistant(autoResearchTaskID, lastAssistantText(c.History()))
		c.recordAutoResearchTurnProgress(autoResearchTaskID, autoResearchAcceptedBefore)
		c.appendAutoResearchHeartbeat(autoResearchTaskID, autoresearch.HeartbeatTurnDone, "")
	} else {
		c.appendAutoResearchHeartbeat(autoResearchTaskID, autoresearch.HeartbeatWarning, err.Error())
		// When the user explicitly cancels (Ctrl+C), the incomplete turn's
		// assistant messages and tool results are already saved to the
		// session. If they stay, the next turn's model sees leftover
		// in-progress todo items and partial tool calls and may re-execute
		// the interrupted work. Keep the real user prompt for visible turns so
		// follow-up questions and resumes do not lose the user's context (#5499).
		return err
	}
	// Headless callers have no plan-approval responder. They still use the
	// shared checkpoint/runtime/Goal lifecycle, while PlanMode remains an agent
	// execution constraint rather than an interactive proposal gate.
	if turn.headless {
		return nil
	}
	c.mu.Lock()
	plan := c.planMode
	c.mu.Unlock()
	if !plan {
		return nil
	}
	proposal := lastAssistantText(c.History())
	if proposal == "" {
		return nil // no substantive proposal to gate
	}
	// The plan is already visible as the assistant's answer, so the request
	// carries no subject — it's purely the gate.
	allow, _, err := c.requestApproval(ctx, planApprovalTool, "", nil)
	if err != nil {
		return err
	}
	if !allow {
		return nil // keep planning; plan mode stays on
	}
	c.SetPlanMode(false)
	todoArgs := c.seedPlanTodos(proposal)
	execStart := c.sessionMessageCount()
	// The plan is the go-ahead: don't re-prompt for each write of the approved
	// work. Auto-approve writers for the duration of this execution turn only; a
	// later turn (even "continue") falls back to the normal per-tool approval.
	c.approval.setPlanAutoApprove(true)
	defer c.approval.setPlanAutoApprove(false)
	err = func() error {
		return o.runComposedSyntheticTurn(ctx, planApprovedMessage)
	}()
	if err != nil {
		return err
	}
	if todoArgs != "" && !c.hasTodoUpdateSince(execStart) {
		c.completePlanTodos(todoArgs)
	}
	return nil
}

func (o *turnOrchestrator) runGoalLoopWithRawDisplay(ctx context.Context, input, raw, display string) error {
	if err := o.runTurnWithRawDisplay(ctx, input, raw, display); err != nil {
		if ctx.Err() != nil {
			o.c.stopGoal(GoalStatusStopped)
		}
		return err
	}
	return o.continueGoal(ctx, false)
}

func (o *turnOrchestrator) runEditedGoalLoopWithRawDisplay(ctx context.Context, input, raw, display, original string) error {
	if err := o.runEditedTurnWithRawDisplay(ctx, input, raw, display, original); err != nil {
		if ctx.Err() != nil {
			o.c.stopGoal(GoalStatusStopped)
		}
		return err
	}
	return o.continueGoal(ctx, false)
}

func (o *turnOrchestrator) runHeadlessGoalLoopWithRaw(ctx context.Context, input, raw string) error {
	if err := o.runOrchestratedTurn(ctx, orchestratedTurn{input: input, raw: raw, headless: true}); err != nil {
		if ctx.Err() != nil {
			o.c.stopGoal(GoalStatusStopped)
		}
		return err
	}
	return o.continueGoal(ctx, true)
}

func (o *turnOrchestrator) continueGoal(ctx context.Context, headless bool) error {
	c := o.c
	selfCheckTurn := false
	for {
		cont := o.advanceGoalAfterTurn(selfCheckTurn)
		if !cont {
			return nil
		}
		if err := ctx.Err(); err != nil {
			c.stopGoal(GoalStatusStopped)
			return err
		}
		turn := goalContinueTurn
		intercepted := false
		if msg, ok := c.goals.takeIntercept(); ok {
			turn = msg
			intercepted = true
			if strings.Contains(msg, "AutoResearch readiness check failed") {
				c.notice("autoresearch readiness blocked completion")
			} else {
				c.notice("goal intercept: incomplete work or verification remains")
			}
		}
		ran, err := o.runObservedSyntheticTurn(ctx, turn, turn, "", headless)
		if err != nil {
			if ctx.Err() != nil {
				c.stopGoal(GoalStatusStopped)
			}
			return err
		}
		if !ran {
			if intercepted {
				runtime := c.goalRuntimeProjection()
				path, data, revision, ok := c.goals.requeueIntercept(turn, runtime)
				c.persistGoalState(path, data, revision, ok)
			}
			return nil
		}
		selfCheckTurn = turn == goalSelfCheckTurn
	}
}

func (o *turnOrchestrator) advanceGoalAfterTurn(selfCheckTurn bool) bool {
	c := o.c
	// Gather every input the FSM needs off the goal lock: parse the marker,
	// snapshot the executor's todos + readiness, and check tool activity. None
	// of these touch goal state, so the machine's critical section stays pure.
	status, reason, _ := parseGoalStatusMarker(lastAssistantText(c.History()))
	autoResearchTaskID := c.goals.currentAutoResearchTaskID()
	var readiness string
	if c.executor != nil {
		readiness = c.executor.GoalReadinessFailure()
	}
	if arReadiness := c.autoResearchReadinessFailure(); arReadiness != "" {
		if readiness != "" {
			readiness += "\n" + arReadiness
		} else {
			readiness = arReadiness
		}
	}
	runtime := c.goalRuntimeProjection()
	res := c.goals.advance(goalAdvanceInput{
		status:           status,
		reason:           reason,
		toolCalled:       c.toolWasCalledLastTurn(),
		selfCheckTurn:    selfCheckTurn,
		todos:            runtime.todos,
		readiness:        readiness,
		planMode:         runtime.planMode,
		messageCount:     runtime.messageCount,
		transcriptDigest: runtime.transcriptDigest,
		durableEvidence:  runtime.durableEvidence,
	})
	c.persistGoalState(res.path, res.data, res.revision, res.ok)
	if res.notice != "" {
		c.finalizeAutoResearchTask(autoResearchTaskID, res.notice)
		c.notice(res.notice)
	}
	return res.cont
}

func (c *Controller) finalizeAutoResearchTask(taskID, notice string) {
	if c.autoResearch == nil || strings.TrimSpace(taskID) == "" {
		return
	}
	switch {
	case notice == goalCompleteNotice:
		status := autoresearch.StatusComplete
		if _, err := c.autoResearch.UpdateProgress(taskID, autoresearch.ProgressPatch{Status: &status}); err != nil {
			c.notice("autoresearch task completion update failed: " + err.Error())
			return
		}
		c.notice("autoresearch task completed: " + taskID)
	case strings.HasPrefix(notice, "goal blocked: ") || notice == "goal continuation limit reached":
		status := autoresearch.StatusBlocked
		reason := strings.TrimPrefix(notice, "goal blocked: ")
		if reason == "" {
			reason = notice
		}
		if _, err := c.autoResearch.UpdateProgress(taskID, autoresearch.ProgressPatch{Status: &status, BlockedReason: &reason}); err != nil {
			c.notice("autoresearch task blocked update failed: " + err.Error())
			return
		}
		c.notice("autoresearch task blocked: " + taskID)
	}
}
