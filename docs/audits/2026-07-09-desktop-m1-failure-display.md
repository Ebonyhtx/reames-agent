# Desktop M1 frontend failure display audit

> Date: 2026-07-09  
> Scope: Desktop frontend failure visibility and idle-state recovery

## Conclusion

The Desktop frontend now has a reducer-level failure display contract for the M1 failure scenarios already covered at the Controller layer.

The frontend must not leave users staring at a stuck composer when the backend reports a provider failure, tool timeout, or blocked write operation. It must show a visible warning/error state and clear the running/stop affordance when the turn is over.

## Evidence

Automated coverage:

- `desktop/frontend/src/__tests__/use-controller-failure-display.test.ts`
  - provider auth failure before streaming:
    - flushes the optimistic user message;
    - shows a warning notice;
    - preserves the actionable API key hint;
    - clears `running`, `pendingPrompt` and `cancellable`.
  - provider 429-style failure after partial output:
    - preserves partial assistant context;
    - finalizes streaming;
    - shows a warning notice;
    - clears running/turn/stop state.
  - tool timeout:
    - renders the tool card as `error`;
    - keeps timeout text in the tool error;
    - allows the model to explain the timeout;
    - clears running/stop state after `turn_done`.
  - denied write approval:
    - clears approval and pending prompt state after the blocked tool result;
    - does not leave a running tool card;
    - clears running state.
- `desktop/frontend/package.json`
  - includes the test in both `test` and `test:all`.

Local validation:

```powershell
Push-Location desktop/frontend
corepack pnpm exec tsx src/__tests__/use-controller-failure-display.test.ts
corepack pnpm exec tsc --noEmit -p tsconfig.test.json
Pop-Location
```

Result: passed locally.

## Boundary

This is not a native Wails screenshot/click smoke. It proves the frontend state machine will render the relevant warning/error state when it receives the correct Wails events.

Still missing:

- real desktop window observation of provider 429/5xx/invalid-key display;
- real desktop window observation of a timed-out tool card;
- real desktop stop button state during and after those failures.
