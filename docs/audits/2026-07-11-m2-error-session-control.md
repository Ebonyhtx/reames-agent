# M2 结构化错误与会话恢复控制面审计

日期：2026-07-11

## 结论

本批关闭了 M2 的两条纵向路径：Desktop 已从“保存一个错误码但继续显示旧字符串”迁移为真正消费共享 `ErrorInfo`；CLI `/resume` 已从 `agent.SessionInfo` / `agent.LoadSession` 直连迁移到稳定 control DTO 和生命周期入口。M2 尚未完成，提交、取消、审批、状态与剩余 command/event DTO 仍按路线图继续收口。

## Desktop 结构化错误路径

- `turn_done.error` 的完整 code/category/message/retryable 进入 notice item；已知 code 使用中英繁中稳定文案，未知 code 才回退后端安全 message。
- category 决定严重度与动作：`auth` 打开模型设置，`retryable` 重试原始 submit prompt，`stream_interrupted` 使用显式续接 prompt，`cancelled` 不渲染为失败。
- 保留旧 `err` 字符串兼容旧事件，但已知结构化错误不再依赖字符串匹配，也不会把旧错误细节作为主文案。
- 动作暴露稳定 AutomationId：`error-action-settings-provider_auth`、`error-action-retry-stream_interrupted`；设置 modal 与关闭按钮也有稳定标识。

组件与 reducer 自动化覆盖本地化、旧字符串隔离、category 动作、canonical submit prompt、无旧 `err` 时的结构化事件以及取消语义。

## CLI 会话恢复边界

- 新增只含 Path、ModTime、Preview、Turns、TopicTitle、CustomTitle 的 `control.SessionInfo`，避免 CLI 绑定完整持久化模型。
- `control.ListSessions` 保留 agent store 的排序/过滤语义并映射为稳定 DTO。
- `ResumeSessionPath(path, beforeResume)` 先加载并验证目标，再执行 outgoing snapshot/lease rebind callback，最后切换 Controller；无效目标和租约失败均不会改变 active session。
- `internal/cli/resume.go` 与 `internal/cli/resume_picker.go` 已移除 `internal/agent` import，依赖棘轮同步删除两条 allowlist 边。

## Windows production Wails 证据

本地重新构建 production Wails：

```text
desktop/build/bin/reames-agent-desktop.exe
size: 47,906,304 bytes
SHA-256: 175B6BA8D5E18027108830FC87F32415F94A9BF50E3EDE3AB6256E1242E6FD09
```

执行 `scripts/smoke_desktop_interaction.py` schema v3，结果 `outcome=passed`、`errors=[]`、`boundary_changes=[]`：

- UIA 真实点击认证错误的“Open model settings”，确认模型设置打开，再通过稳定关闭按钮返回会话；
- UIA 真实点击流中断的“Continue response”，续接 prompt 进入真实 Desktop → Controller → OpenAI provider 路径，回答可见并持久化；
- `auth_settings_opened=true`、`stream_retry_invoked=true`；
- Provider 请求仍为 19 次，五类失败各自的 signal/idle/followup 全部为 true；
- 部分流输出、权限拒绝不落盘、工具超时、长命令 Stop、关闭重启与同 session/workspace 恢复继续通过；
- 使用隔离 home/workspace 和 localhost 合成 Provider，不读取用户密钥或默认用户状态。

证据文件为本地生成的 `artifacts/desktop-windows-interaction-smoke-m2-local.json`，不作为仓库源文件提交；远端 Windows candidate 将由同一脚本生成可下载的安装后证据。

## 证据边界

本审计证明 production Windows Wails/WebView2 中的错误动作和恢复路径，以及 CLI/control 的确定性单元与架构合同。localhost Provider 不冒充公网模型；本批也不证明真实 IM、云节点、签名或 notarization。长期 GOAL 和 M2 仍保持进行中。

## 本地验证

以下门禁全部通过：

```text
go build ./...
go vet ./...
go test ./internal/... -count=1 -timeout 10m
desktop/go test ./... -count=1 -timeout 10m
desktop/frontend/corepack pnpm test:all
desktop/frontend/corepack pnpm build
python -m unittest scripts.test_smoke_desktop_interaction scripts.test_smoke_desktop_native scripts.test_check_upstreams -v
node scripts/test_upstream_watch_issue.mjs
go test ./internal/tool -run TestBuiltinToolContractDocumentation -v -count=1
python scripts/smoke_desktop_interaction.py --exe desktop/build/bin/reames-agent-desktop.exe --artifact desktop/build/bin/reames-agent-desktop.exe --out artifacts/desktop-windows-interaction-smoke-m2-local.json --timeout-seconds 45
git diff --check
```

前端 production build 仍报告既有大 chunk 警告；构建成功，该性能债务留在 M3，不冒充本批阻断。
