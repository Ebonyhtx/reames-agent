# 下一位模型接手交接文档

日期：2026-07-10
仓库：`F:\reames-agent`
主线：`main`

本页是当前事实快照；代码、Git 状态、`docs/DEVELOPMENT_PLAN.md` 和远端 CI 结果优先级更高。

## 用户目标与工作方式

持续把 Reames Agent 推进到高可信可交付状态：保持 CI 可用、同步清洁文档和路线图，参考 DeepSeek Reasonix、`F:\code-reference` 与 `F:\Reames-Lite` 吸收机制优点。每批应形成足够大的实质改动，完成代码、测试、文档和验证后再一次性提交/push，避免频繁消耗 CI。

沟通使用中文。结论必须区分单元测试、本地原生实测、远端 candidate 和真实外部服务证据，不能把局部通过描述为整个项目完成。

## 受保护的工作区边界

以下文件属于用户或其他会话，禁止修改、暂存或提交：

```text
docs/audits/2026-07-09-reference-feature-gap-map.md
.agents/BULK_EXECUTION_BRIEF.zh-CN.md
.agents/BULK_EXECUTION_STATUS.zh-CN.md
```

始终使用显式 `git add <path...>`，不要使用 `git add .` 或 `git add -A`。

## 项目源流

- 源码上游：`F:\code-reference\DeepSeek-Reasonix`，对应 `esengine/DeepSeek-Reasonix` 的 `main-v2`。
- 机制参考：`F:\code-reference` 下 Hermes、Codex CLI、MiMo Code、Impeccable、Scream Code、AgentArk、Claude Code、Kimi Code。
- 旧版契约参考：`F:\Reames-Lite`。
- 治理规则：`docs/REFERENCE_GOVERNANCE.md` 与 `docs/upstreams/README.md`。

其他参考项目只吸收机制与体验，不直接盲目复制；上游更新先研究和形成采用建议，再决定是否进入主线。

## 已关闭的基线

M0“基线可信”已经关闭，关键远端证据：

```text
普通 CI run 29072070070：8/8 jobs success
CodeQL run 29072070100：Go、JavaScript/TypeScript、Actions success
Desktop candidate run 29070966084：Windows、Linux、macOS 3/3 success
```

三平台候选已覆盖：

- Linux 安装真实 `.deb`，在 Xvfb 验证可见窗口；
- macOS 挂载真实 `.dmg`、复制并校验 universal `.app` 后启动；
- Windows 静默安装真实 NSIS，验证 HKCU 注册、update helper、安装后二进制启动，再静默卸载并检查清理。

最近相关主线提交：

```text
18c9cc4 test: stabilize Desktop teardown timing gate
ad8be7d test: smoke Windows Desktop installer lifecycle
8297c7d fix: verify macOS candidate architectures
09661f2 test: smoke native Desktop candidate installs
83d0f70 test: make Desktop smoke close deterministically
58d1d33 fix: harden isolated Desktop native smoke
```

## 当前批次：Windows 原生交互闭环

用户指出先前 UI 自动化一直被 API key 引导层挡住。当前批次据此关闭了三个真实问题：

1. `[desktop].onboarding_dismissed` 持久化“暂时跳过”，重启不再反复显示引导；真实凭据判断保持原语义。
2. 显式 `--home` / `REAMES_AGENT_HOME` 同时把 WebView2 user data 隔离到 `<home>/webview2`，避免 localStorage、cookie 和缓存跨 home 泄漏；普通启动保留 Wails 默认路径以兼容已有用户。
3. 新增截图无关的 Windows UIA 驱动和交互 smoke，不再依赖会返回 `0x80004002` 的 frameless 截图通道。

本地原生实测已通过：

```text
隔离无密钥启动
→ 选择预置项目并新建会话
→ UIA 输入 marker
→ keyless 127.0.0.1 OpenAI-compatible SSE round-trip
→ user/assistant 写入 canonical .events.jsonl
→ 输入 !Start-Sleep -Seconds 30
→ Stop 出现、InvokePattern 取消、Stop 消失
→ WM_CLOSE 后重启
→ 同一 workspace、session path、user/assistant 消息恢复
→ 默认用户状态无变化、临时夹具删除
```

本地验证二进制 SHA-256：

```text
A0AD1AB8FA5EF7948279F64BBDAD6F8A1E905F0FD15A06909BF3F5923625D449
```

实现与证据入口：

- `scripts/windows_uia.py`
- `scripts/smoke_desktop_interaction.py`
- `scripts/test_smoke_desktop_interaction.py`
- `docs/audits/2026-07-10-windows-native-interaction-smoke.md`

`Desktop candidate` 的 Windows 安装阶段会运行这条交互 smoke，并上传 `desktop-windows-interaction-smoke.json`。loopback 证明本地完整传输与持久化，不代替 `docs/audits/2026-07-09-real-provider.md` 的真实公网 Provider 证据。

## 仍未关闭的关键缺口

1. M1 失败场景仍需扩展原生窗口证据：断流、429、无效密钥、权限拒绝和工具超时已有分层自动化，但还没有全部进入 native UI smoke。
2. M2 只完成依赖增长棘轮；提交、取消、审批、会话和状态 DTO 仍需按纵向路径收口。
3. 仍缺干净 Linux/阿里云 ECS 的真实安装、`REAMES_AGENT_HOME`、真实 API、Gateway service、CLI、日志、备份与回滚证据。
4. 飞书/Lark 的真实 bot 凭据、消息、回复、审批、取消和恢复尚未端到端实测。
5. Feedback 已有本地 ledger、serve API 和 CLI submit/summary/draft，但 Desktop/Gateway 的用户预览与 opt-in 入口仍未完成。
6. Upstream Watch 已能发现与生成 issue 草稿；自动研究差异、报告和建议补丁但不自动合并仍待推进。

路线优先级以 `docs/DEVELOPMENT_PLAN.md` 为准。当前推荐顺序：

```text
提交/取消/审批/会话/状态 control 边界纵向收口
→ 原生 Desktop 失败场景
→ 干净云节点 CLI + Gateway + feedback 运维闭环
→ 真实飞书文本/审批/取消/恢复回环
```

## 验证入口

根模块：

```powershell
go build ./...
go vet ./...
go test ./internal/... -count=1 -timeout 300s
```

Desktop 与前端：

```powershell
Push-Location desktop
go test . -count=1 -timeout 300s
Pop-Location

Push-Location desktop/frontend
corepack pnpm test:all
corepack pnpm build
Pop-Location
```

合同与原生交互：

```powershell
python -m unittest scripts.test_smoke_desktop_candidate scripts.test_smoke_desktop_native scripts.test_smoke_desktop_interaction -v
python scripts/check_release_contracts.py
python scripts/check_docs_contracts.py
python scripts/check_deploy_contracts.py
python scripts/check_public_readiness.py
python scripts/smoke_desktop_interaction.py --exe desktop/build/bin/reames-agent-desktop.exe --out artifacts/windows-interaction-smoke.json
git diff --check
```

上游追踪：

```powershell
python -m unittest scripts.test_check_upstreams -v
node scripts/test_upstream_watch_issue.mjs
python scripts/check_upstreams.py --out-dir artifacts/upstream-watch
```

## 提交前检查

1. `git status --short` 确认只包含本批文件和受保护的未跟踪文件。
2. 删除本地 `artifacts/` 调试产物与误生成的根目录 pnpm workspace 文件；不要删除受保护文件。
3. 显式列出暂存路径，检查 `git diff --cached --stat` 和 `git diff --cached --check`。
4. 大批提交后只 push 一次，观察普通 CI、CodeQL；修改 Desktop candidate 时手动触发一次 candidate 并等待三平台结束。

长期 GOAL 尚未完成；不要因为 M0 和 Windows 原生交互闭环通过就把整个项目标记完成。
