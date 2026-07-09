# Desktop M1 frontend workspace loop audit

> Date: 2026-07-09  
> Scope: Desktop frontend `useController` workspace/session state machine

## Conclusion

The Desktop frontend now has an M1-oriented workspace loop contract that complements the Go/Wails backend binding tests.

It verifies the React controller path for:

```text
startup with no active tab
→ EnsureBlankTab("project", workspace A)
→ SubmitToTab(tab A, prompt)
→ OpenProjectTab(workspace B, topic B)
→ OpenProjectTab(workspace A, topic A)
→ CancelTab(tab A)
```

The key safety property is that switching to workspace B does not inherit workspace A's running state or transcript, and stopping after returning to workspace A calls `CancelTab(tab A)` without cancelling workspace B.

## Evidence

Automated coverage:

- `desktop/frontend/src/__tests__/use-controller-m1-workspace-loop.test.tsx`
  - starts with no active tab;
  - verifies `EnsureBlankTab` receives the selected project workspace root;
  - verifies send uses `SubmitToTab` for the active workspace tab;
  - verifies workspace B opens with its own transcript and idle state;
  - verifies workspace A's optimistic running turn survives switching away and back;
  - verifies Stop calls `CancelTab` for workspace A and never for workspace B.
- `desktop/frontend/package.json`
  - includes the new test in both `test` and `test:all`.

Local validation:

```powershell
Push-Location desktop/frontend
corepack pnpm exec tsx src/__tests__/use-controller-m1-workspace-loop.test.tsx
corepack pnpm test:all
Pop-Location
```

Result: passed locally.

## Boundary

This is still a frontend state-machine and Wails-binding mock test. It does not replace the remaining native Wails window smoke:

- click/select a workspace in the real desktop app;
- send a real model turn and observe streaming;
- stop from the real UI;
- close/reopen the app and verify restored workspace/session state;
- run a file-writing approval flow through the actual window.
