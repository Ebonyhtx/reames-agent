// Run: tsx src/__tests__/use-controller-failure-display.test.ts

import { initialState, reducer } from "../lib/useController";
import type { WireEvent } from "../lib/types";

let passed = 0;
let failed = 0;

function ok(value: boolean, label: string) {
  if (value) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
  } else {
    process.stdout.write(`  FAIL  ${label}\n`);
    failed += 1;
  }
}

function eq(actual: unknown, expected: unknown, label: string) {
  ok(actual === expected, `${label}${actual === expected ? "" : `: expected ${JSON.stringify(expected)}, got ${JSON.stringify(actual)}`}`);
}

function apply(events: WireEvent[]) {
  return events.reduce((state, e) => reducer(state, { type: "event", e }), initialState);
}

console.log("\nuse controller failure display");

{
  const retrying = apply([
    { kind: "turn_started" },
    { kind: "retrying", retryAttempt: 1, retryMax: 10 },
    { kind: "phase", text: "waiting for provider" },
    { kind: "notice", level: "info", text: "background status" },
  ]);
  eq(retrying.retry?.attempt, 1, "retry status survives non-output phase and notice events");
  const recovered = reducer(retrying, { type: "event", e: { kind: "text", text: "recovered" } });
  eq(recovered.retry, undefined, "provider output clears retry status");
}

{
  const sent = reducer(initialState, { type: "user", text: "hello", seq: 0 });
  const failedBeforeStream = reducer(sent, {
    type: "event",
    e: {
      kind: "turn_done",
      err: "provider auth failed: set DEEPSEEK_API_KEY and retry",
      error: { code: "provider_auth", category: "auth", message: "Authentication failed.", retryable: false, httpStatus: 401 },
    },
  });
  const user = failedBeforeStream.items.find((item) => item.kind === "user");
  const notice = failedBeforeStream.items.find((item) => item.kind === "notice");
  eq(user?.kind === "user" && user.text, "hello", "turn_done error flushes the optimistic user message");
  eq(notice?.kind === "notice" && notice.level, "warn", "turn_done error renders a warning notice");
  eq(notice?.kind === "notice" && notice.code, "provider_auth", "turn_done notice keeps the structured error code");
  eq(notice?.kind === "notice" && notice.text, "Authentication failed.", "structured message replaces legacy string matching");
  eq(notice?.kind === "notice" && notice.error?.category, "auth", "turn_done notice keeps the structured error category");
  eq(failedBeforeStream.running, false, "turn_done error clears running state before stream starts");
  eq(failedBeforeStream.pendingPrompt, false, "turn_done error clears pendingPrompt");
  eq(failedBeforeStream.cancellable, false, "turn_done error clears the stop affordance");
}

{
  const state = apply([
    { kind: "turn_started" },
    { kind: "reasoning", reasoning: "thinking" },
    { kind: "message", text: "partial answer" },
    {
      kind: "turn_done",
      err: "openai-compatible provider returned HTTP 429: rate limited; wait and retry",
      error: { code: "provider_rate_limit", category: "retryable", message: "Rate limit reached.", retryable: true, httpStatus: 429 },
    },
  ]);
  const assistant = state.items.find((item) => item.kind === "assistant");
  const notice = state.items.find((item) => item.kind === "notice" && item.code === "provider_rate_limit");
  eq(assistant?.kind === "assistant" && assistant.text, "partial answer", "provider failure preserves partial assistant context");
  eq(assistant?.kind === "assistant" && assistant.streaming, false, "provider failure finalizes the assistant stream");
  eq(notice?.kind === "notice" && notice.level, "warn", "provider failure shows a warning notice");
  eq(notice?.kind === "notice" && notice.code, "provider_rate_limit", "provider failure exposes the structured rate-limit code");
  eq(notice?.kind === "notice" && notice.retryText, undefined, "retry action is withheld when no user prompt is available");
  eq(state.running, false, "provider failure clears running state");
  eq(state.turnActive, false, "provider failure clears active turn state");
  eq(state.cancellable, false, "provider failure clears stop state");
}

{
  const sent = reducer(initialState, { type: "user", text: "visible prompt", submitText: "provider prompt", seq: 0 });
  const retryable = reducer(sent, {
    type: "event",
    e: {
      kind: "turn_done",
      error: { code: "provider_timeout", category: "retryable", message: "Provider timed out.", retryable: true },
    },
  });
  const notice = retryable.items.find((item) => item.kind === "notice");
  eq(notice?.kind === "notice" && notice.retryText, "provider prompt", "retryable errors retain the canonical submit prompt");
  eq(notice?.kind === "notice" && notice.error?.code, "provider_timeout", "structured error renders without the legacy err field");
}

{
  const sent = reducer(initialState, { type: "user", text: "stop this", seq: 0 });
  const cancelled = reducer(sent, {
    type: "event",
    e: {
      kind: "turn_done",
      err: "context canceled",
      error: { code: "cancelled", category: "cancelled", message: "Cancelled.", retryable: false },
    },
  });
  eq(cancelled.items.some((item) => item.kind === "notice"), false, "user cancellation is not rendered as a failure");
  eq(cancelled.running, false, "cancelled turn still clears running state");
}

{
  const state = apply([
    { kind: "turn_started" },
    { kind: "tool_dispatch", tool: { id: "tool-timeout", name: "bash", args: "sleep 60", readOnly: false } },
    { kind: "tool_result", tool: { id: "tool-timeout", name: "bash", readOnly: false, err: "tool bash timed out after 50ms" } },
    { kind: "message", text: "The shell command timed out before it completed." },
    { kind: "turn_done" },
  ]);
  const tool = state.items.find((item) => item.kind === "tool");
  const assistant = state.items.find((item) => item.kind === "assistant");
  eq(tool?.kind === "tool" && tool.status, "error", "tool timeout renders the tool card as an error");
  ok(tool?.kind === "tool" && Boolean(tool.error?.includes("timed out")), "tool timeout keeps the tool error text");
  eq(assistant?.kind === "assistant" && assistant.text, "The shell command timed out before it completed.", "tool timeout can still end with a model explanation");
  eq(state.running, false, "tool timeout turn clears running state after turn_done");
  eq(state.cancellable, false, "tool timeout turn clears stop state after turn_done");
}

{
  const state = apply([
    { kind: "turn_started" },
    { kind: "approval_request", approval: { id: "write-1", tool: "write_file", subject: "write README.md" } },
    { kind: "tool_result", tool: { id: "write-1", name: "write_file", readOnly: false, err: "approval denied by user" } },
    { kind: "message", text: "I did not modify the file because permission was denied." },
    { kind: "turn_done" },
  ]);
  const tool = state.items.find((item) => item.kind === "tool");
  eq(state.approval, undefined, "turn_done clears an approval after the blocked tool result");
  eq(state.pendingPrompt, false, "turn_done clears pendingPrompt after the blocked tool result");
  ok(tool?.kind !== "tool" || tool.status !== "running", "denied approval does not leave a running tool card");
  eq(state.running, false, "denied approval turn clears running state");
}

console.log(`\n${passed} passed, ${failed} failed, ${passed + failed} total`);
if (failed > 0) process.exit(1);
// Keep the reducer and its rendered, actionable notice contract in the same
// aggregate test entry without growing the package script's hand-maintained list.
await import("./runtime-error-notice.test");
