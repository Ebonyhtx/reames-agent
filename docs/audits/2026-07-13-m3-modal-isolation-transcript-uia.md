# M3 模态隔离、Transcript 与原生 UIA 可访问性审计

日期：2026-07-13

状态：实现已由 commit `827e0b4` 交付，普通 CI `29229513429` 8/8、CodeQL `29229513359` 3/3；安装器 accessibility 仍待 Windows interaction 前序门禁修复后复核

## 缺口

已交付的模态焦点首批统一了初始焦点、Tab 围栏和退出动画后的 opener 恢复，但 `aria-modal="true"` 本身不会自动把背景从辅助技术树中移除。嵌套模态、退出动画和快速关闭后重开还会让简单的 element/ref-count 栈误释放新实例。可见 Transcript 按 token 更新时也不能直接充当 live region，否则完整答复、工具卡和流式 token 可能被重复播报。

Windows 自动化此前只能证明基础 UIA 交互，未记录 ARIA role/properties、焦点、可聚焦和 offscreen 状态，也没有一个禁止坐标回退的严格可访问性链路。

## 实现

### 真正模态的共享隔离生命周期

- 七个真正模态暴露稳定边界：`command-palette-dialog`、`settings-dialog`、`history-dialog`、`shortcuts-cheatsheet-dialog`、`image-viewer-dialog`、`onboarding-dialog`、`heartbeat-dialog`。
- `useDialogFocus` 以每次打开独立 lease 管理顶层栈，不按 DOM element 去重；旧退出动画 cleanup 因此不能释放快速重开的新实例。新 lease 同时继承直接返回控件和上层 lease 的原始 opener 链：子模态正常关闭优先回父控件，父模态已退出或 StrictMode 重放 effect 时继续回到最初入口，而不是把焦点留在 `body`。
- 顶层 dialog 激活时沿 dialog 到 `body` 的路径逐层隔离兄弟分支，同时设置 `inert` 与 `aria-hidden="true"`；关闭时逐字恢复原有属性。MutationObserver 只响应路径父节点的 child-list 变化，使晚挂载 portal/sibling 也被隔离，但不会因 Transcript token 更新反复重算。
- 焦点先尝试移入 dialog，再启用隔离；若嵌套 dialog 挂在父层已 inert 的 portal 容器中，重算暴露子路径后再重试一次焦点。只有顶层模态处理 Escape，嵌套层一次只关闭一层。
- Settings 显式保存真实 `settings-open` opener，关闭后即使 lazy mount/退出动画跨帧也恢复到入口；History 增加顶层 Escape，重命名输入与 ContextMenu 不误关整个面板。Settings 内容区由重复 `main` 改为按当前 tab 命名的 section。
- `PromptShelf aria-modal=false` 继续作为非阻断审批/提问层，不进入背景隔离。

### Transcript 语义

- 主对话区使用唯一 `transcript-log`、`role=log`、本地化名称、`aria-live=off` 和运行/历史加载对应的 `aria-busy`；可见 token 继续实时更新，但不逐 token 触发 live announcement。
- 独立 `transcript-announcer` 使用 polite、atomic status，只在真实 `running → idle` 且出现新完成 assistant 时提交一次最终文本。启动/同会话 hydration、tab/session 切换、history append、rewind/undo、`/clear` session generation 与 assistant ID 复用都只更新基线，不回放历史。
- History 预览改用唯一 `history-preview-transcript-log` 并关闭 announcer，避免主 Transcript 同时挂载时出现重复 ID 或第二个 live region。

### Windows UIA 与 candidate 门禁

- `windows_uia.py` 新增 LocalizedControlType、HasKeyboardFocus、IsKeyboardFocusable、IsOffscreen、AriaRole、AriaProperties 读取，索引按 Windows SDK 固定为 22/26/27/38/45/46。
- 新增严格 `invoke_pattern()`：InvokePattern 缺失或调用失败立即报错，不允许降级为坐标点击；兼容旧交互 smoke 的 `invoke()` 只有在 pattern 不可用时保留原有 fallback。
- `smoke_desktop_accessibility.py` 使用隔离 synthetic HOME 和 production Wails executable，依次验证主 `main`、对话 log/status、skip link、composer、设置入口；严格 Invoke skip→composer、设置入口→dialog、关闭按钮→opener；dialog 打开时六个背景稳定 ID 必须从 UIA 树消失。
- WebView2 会省略默认 `aria-busy=false`，并不会把 `aria-modal=true` 串入 UIA AriaProperties。smoke 因此等待 hydration 后 `live=off` 且不再 `busy=true`，拒绝显式 `modal=false`，并以 `AriaRole=dialog`、关闭按钮焦点和背景树消失证明原生模态行为；DOM/Browser 合同单独锁定 `aria-modal=true`。
- 普通 CI 运行 UIA/Accessibility smoke 的确定性单测；人工 Desktop candidate 的 Windows job 在安装真实 NSIS 后运行严格 smoke，并上传 `desktop-windows-accessibility-smoke.json`。release contract 精确锁定 Windows step、artifact/exe 参数、输出路径与非零退出 fail-closed。

## 参考与取舍

- Kimi Code 的 `useDialogFocus` 与 dialog stack 提供焦点进出和顶层优先级思路；Reames 额外实现 Tab 围栏、lease、背景 `inert`/ARIA 隔离、lazy mount、动态 portal 和退出动画。
- MiMo Code 的隐藏区域 `inert` 切换提供机制参考，但没有嵌套栈、属性原值恢复或动态 sibling 处理，未直接复制。
- DeepSeek Reasonix 保留局部 dialog/ARIA 语义，没有可复用的共享背景隔离与 Windows UIA 严格门禁。
- `F:\Reames-Lite` 没有等价的 React modal lease 或 UIA 驱动实现，本批不声明其为代码来源。

## 本地证据

```text
corepack pnpm typecheck                                      PASS
corepack pnpm test:dialog-focus                              PASS (35 + 2)
tsx src/__tests__/transcript-grouping.test.ts                PASS (54)
corepack pnpm test:all                                       PASS
corepack pnpm build                                          PASS
python -m unittest scripts.test_windows_uia -v               PASS (7)
python -m unittest scripts.test_smoke_desktop_accessibility  PASS (10)
python -m unittest discover -s scripts -p 'test_*.py' -v     PASS (96, 2 skipped)
python scripts/check_release_contracts.py                    PASS
go build ./...                                               PASS
go vet ./...                                                 PASS
go test ./internal/... -count=1                              PASS
desktop: go test . -count=1                                  PASS
docs/public/release/deploy/tool contracts                    PASS
Wails v2.12.0 production Windows build                       PASS
```

最终 production bundle：entry 628,571 B；base initial JS 869,373 B / 5 files；最坏本地化首启 984,827 B；initial CSS 511,305 B；browser mock 964,263 B；VirtualMenu 894,626 B；Settings 1,057,628 B JS + 611,477 B CSS；largest JS 704,186 B，全部在硬预算内。

无热更新干扰的 localhost Browser 验证主 `main`、`transcript-log`、`transcript-announcer` 和 `settings-open` 均唯一；Settings 打开后六个背景入口都位于 inert/aria-hidden 分支，关闭按钮获得焦点；Settings 上再开 Command Palette 时只暴露顶层，第一次 Escape 只关闭 Palette，第二次关闭 Settings 并稳定恢复 `settings-open`；最终 warning/error 为 0。

production Wails executable 为 48,050,176 B，SHA-256 `8E008415A9D331ABFFA63864CD67B5818FFC74F8FD5D8984790C7371F8590CD7`。严格 accessibility smoke 的三次动作全部记录为 InvokePattern，skip→composer、dialog 初始关闭焦点、背景 UIA 树消失、关闭后 opener 恢复、临时目录清理和默认状态边界全部通过。native smoke 的 cold 首次可见/响应 0.500 秒、稳定 1.500 秒，warm 首次可见/响应 0.500 秒、稳定 1.500 秒，满足本地 8/6 秒预算。interaction smoke 完成 19 次 loopback provider 请求、五类失败恢复、停止、持久化和重启恢复，`boundary_changes=[]`、`errors=[]`。

## 远端 candidate 结果与后续修复

Desktop candidate `29229871453` attempt 1 的 Linux/macOS jobs 成功，Windows
安装、native cold/warm 启动也成功；interaction 已完成 19 请求、五类失败恢复、
审批拒绝、工具超时、停止、磁盘事件账本和清理，但重启 30 秒未在 UI 中显示
marker/assistant。attempt 2 重跑 Windows 后在同一点再次失败，因此不能归类为
偶发 runner 波动，后续 accessibility step 也没有执行。

当前批次把 pinned transcript 预载与 controller readiness 分离：恢复 tab 发布
`ready=false` metadata 后立即只读 session event log 并显示历史，composer 继续锁定；
ready event/轮询到达后复用已加载历史并补齐 meta/effort/jobs/context/checkpoints。
新增 ready-event 与 missed-ready 测试证明历史在 runtime 未就绪时可见、发送仍锁定、
ready 不等待 ancillary 且 history 只读取一次。新 production Wails executable 为
48,050,688 B，SHA-256
`A4D22842BB5C107AA1E9F6829947046338FBD15826AADF035AFCDD0234F4E8A0`；本地 native
cold/warm 均为 0.5 秒首响、1.5 秒稳定，严格 accessibility 3/3 InvokePattern，
interaction 19 请求、五类恢复、停止和重启恢复通过，边界变化与 errors 均为空。
该新提交尚需安装器 candidate 远端复核。

## 证据边界

组件/DOM 测试不等于原生 UIA；本地源码 production Wails 不等于安装器 candidate；workflow 接线和旧 commit 的失败 candidate 都不能证明新修复已远端通过。UIA 暴露 polite/atomic 属性也不等于 NVDA/Narrator 实际只朗读一次，`forced-colors` 合同不等于 Windows High Contrast 人工验证。本批没有使用真实 API key；loopback provider 不是真实公网 Provider。屏幕阅读器听感、High Contrast、签名安装器和新 commit 的 Windows installed candidate 仍需独立手动或外部证据。
