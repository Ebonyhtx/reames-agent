# Desktop M1 bridge audit

Date: 2026-07-09

## Scope

M1 requires a native Desktop real-task loop: create/select a workspace, send a turn, stop it, then move on to file-writing approval, persistence, and recovery.

This audit records the automated bridge evidence added before the manual/native Wails click test. It does not claim the full M1 Desktop E2E is complete.

## What was verified

Added `TestSubmitAndCancelForTabStayBoundDuringActiveTabSwitching` and
`TestDesktopBoundWorkspaceSessionSubmitStopPath` in `desktop/app_test.go`.
The frontend-facing state machine is additionally covered by
`desktop/frontend/src/__tests__/use-controller-m1-workspace-loop.test.tsx`;
see `2026-07-09-desktop-m1-frontend-workspace-loop.md`.

The first test uses real `control.Controller` instances with fake blocking runners, so no real provider key or network call is involved. It verifies:

- `SubmitToTab("project-a", prompt)` routes the model turn to project A's controller.
- Project B's controller does not receive the prompt.
- The controller workspace root remains bound to project A.
- The UI active tab can switch to project B while project A is still running.
- `CancelTab("project-a")` cancels the original project A turn, not the currently active project B tab.
- Project B remains idle and the active tab remains project B after background cancellation.

The second test exercises the Wails-bound backend path the frontend calls for
the M1 create/select/send/stop loop:

- `EnsureBlankTab("project", root)` creates a pinned blank project session for two distinct workspaces.
- `OpenProjectTab(root, topicID)` selects the existing workspace/topic tab instead of creating a duplicate.
- `SubmitToTab(tabID, prompt)` starts the turn on the selected project workspace.
- Switching to another project via `OpenProjectTab` while the first turn is running keeps the first turn bound to its original controller and workspace root.
- `CancelTab(tabID)` stops the background project turn without cancelling or starting the newly-active project tab.

This closes a key bridge-risk class for Desktop M1: frontend tab focus changes must not reroute an in-flight task or cancel the wrong workspace session.

## Verification

```powershell
Push-Location desktop
go test . -run 'TestDesktopBoundWorkspaceSessionSubmitStopPath|TestSubmitAndCancelForTabStayBoundDuringActiveTabSwitching' -count=1 -timeout 180s
Pop-Location
```

Result: passed locally.

## Remaining M1 evidence

- Native Wails UI smoke: create/select workspace, send, stop.
- Real provider Desktop turn with streaming visible in the UI.
- Tool approval and patch preview for a file-writing task.
- File落盘、checkpoint/rewind and restart/session restore.
- Failure-path automation: disconnect, rate limit, invalid key, permission denial, tool timeout.
