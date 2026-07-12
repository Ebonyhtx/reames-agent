# M3 Desktop 重启恢复竞态审计

日期：2026-07-13

状态：两层生产修复、确定性测试与本地原生交互通过；hosted installer candidate 复核待完成

## 触发证据

Windows `Desktop candidate` run `29210320483` 的 native startup JSON 已通过 15/6 秒 cold/warm 预算，但随后的 interaction smoke 在完成 19 次 loopback 请求、五类失败恢复、审批拒绝、工具超时和停止后，重启 30 秒仍未显示原始用户与助手消息。canonical event log、session path、清理和默认状态边界均正常，因此问题集中在启动后的 tab/transcript 可见性，而不是数据未落盘。

## 根因与实现

- `startup()` 在 goroutine 中执行 `restoreOrBuildTabs()`，而前端 WebView bridge 可立即调用 `ListTabs()`。旧实现直接读取 `a.tabs`；当首次调用抢在恢复填充前时会返回空快照。如果随后 `agent:ready` 又早于前端订阅，当前进程就没有第二条可靠同步路径，持久会话会整轮不可见。
- 后端本来已有 `tabsRestored` channel，用于阻止 recovery GC 在恢复前观察空 tab 集合。commit `0cdfef1` 把同一门闩接到导出的 `ListTabs()`：进入过 startup 的实例必须等 tab 条目恢复；未进入 startup 的单测实例获得已关闭信号，不改变既有调用方式。
- run `29211086907` 证明仅关闭第一层仍不够：Windows startup 再次通过，interaction 仍在 19 请求后重启消息不可见。`tabsRestored` 在异步 `startTabControllerBuild()` 启动后即关闭，因此首份 tab metadata 仍可能是 `ready=false`；前端立即读取 history 会得到空结果，重叠的 `agent:ready` 又会加入同一个 in-flight hydrate，使空 transcript 成为最终状态。
- 前端 `syncActiveTabFromBackend()` 现在先发布 tab shell 与 `ready=false` metadata，再复用已有 `waitForTabReady()` 轮询权威 `ListTabs`；startup 使用与原生 interaction 单步一致的 30 秒有界等待，只在 `ready=true`、显式 `startupErr` 或超时后刷新 sessionPath/metadata 并启动第一次 history hydrate。ready event 与轮询并发时仍由现有 in-flight 合并保证恰好一次读取，tab 切换 guard 在等待后再次检查，避免晚到恢复覆盖用户选择；其他 fork 等路径仍保留原 6 秒门槛。
- 新并发测试先证明 `ListTabs()` 在门闩关闭前不会发布空快照，再填充 active tab、关闭门闩并验证调用立刻返回权威快照。既有 classic/workbench 重启恢复测试继续覆盖真实 session/workspace/history。
- interaction evidence reader 逐行重放 canonical event log；遇到并发追加中的尾部半行时保留已验证前缀并等待下一轮采样，不再因一次 JSON decode error 抹掉全部持久化证据。
- WebView2 偶发丢失 posted-key 与 SendInput Enter 时，仅对明确的 `UIA Enter did not submit composer` 错误回退到稳定 `composer-send` InvokePattern；焦点、控件缺失等其他错误仍 fail closed。两个单测分别冻结 fallback 和不吞错语义。

## 当前证据

```text
python -m unittest scripts.test_smoke_desktop_interaction -v  PASS (17, 1 skipped)
desktop targeted restart/ListTabs tests                       PASS
wails v2.12.0 production build windows/amd64                  PASS (49.3s)
production Windows UIA interaction smoke                      PASS
provider requests / scripted failure scenarios                PASS (19 / 5)
stop + same-session workspace/transcript restart recovery     PASS
cleanup / default-state boundary                              PASS / 0 changes
go build ./... / go vet ./... / go test ./internal/...        PASS
desktop/go vet ./... / go test ./...                          PASS
frontend typecheck / test:all / production build              PASS
smoke/docs/public/release contracts                            PASS
ready-event / missed-ready / tab-switch / new-session tests   PASS (6 / 8 / 61 / 15)
```

包含 30 秒 controller-ready 前端门槛的最新 production 可执行文件 SHA-256 为 `E744BD7705C71962873ED56BD10775AB7412ED517B51843DBBDD8C29808F2305`，大小 47,955,456 B。重启前后 session path 完全一致；用户 marker 和 loopback assistant response 在 UI 中恢复，五类失败场景均完成可见信号、idle 恢复与后续成功 turn；两个进程、临时 HOME 均成功清理，默认状态边界变化为 0。

partial fix commit `0cdfef1` 的普通 CI run `29211082959` 8/8、CodeQL run `29211082955` 3/3。candidate run `29211086907` 的 Linux/macOS jobs 成功，Windows native startup 也再次通过（cold 首次/稳定 5.032/6.032 秒，warm 首次/稳定 1.000/2.000 秒），但修复前 frontend 仍使 interaction recovery 失败。该失败是第二层竞态的远端触发证据，不是启动预算回归。

当前本地证据证明最新源码 production Wails 和确定性 loopback/UIA 路径，不冒充 hosted installer。下一次集中 push 后只复跑一次三平台 candidate，并以 Windows native 与 interaction 两份 JSON 作为远端关闭证据。
