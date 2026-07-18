# Reames Agent 新会话无痛交接

> 日期：2026-07-18
>
> 仓库：`F:\reames-agent`
>
> 分支：`main`，不保留第二开发分支
>
> 用途：即使 Codex 会话、`F:\code-reference` 或本机缓存在电脑清理后丢失，新会话也只凭 Git
> 仓库恢复当前边界、证据和下一步。

## 1. 权威信息顺序

发生冲突时按以下顺序判断，不依赖旧聊天记录：

1. `git status --short --branch`、`git log -1 --oneline --decorate` 与远端 Actions；
2. `docs/PROJECT.md`：产品方向和当前事实；
3. `docs/DEVELOPMENT_PLAN.md`：执行顺序和关闭门槛；
4. `docs/REFERENCE_GOVERNANCE.md`、`docs/upstreams/upstreams.lock.json`：上游来源和 reviewed SHA；
5. `docs/audits/`：完成声明、限制和实际证据。

冻结提交就是“包含本文件最终版本的 `main` 提交”。提交不能可靠地自引用自身 SHA；新会话执行
`git log -1 --format="%H %s"` 即可取得精确值。不要用聊天摘要中的短 SHA 覆盖 Git。

## 2. 用户目标和工作节奏

持续把 Reames Agent 推进到高可信可交付状态；Reasonix 是唯一主源码上游，其他项目只吸收适用机制。
每个大批同步实现、测试、文档和证据；充分本地验证后集中 commit/push，避免碎片 push 浪费 CI。

永久边界：

- 不恢复启动、metrics、crash、performance 或用户使用数据的远端上传；
- 不使用用户服务器承担 Reames 遥测或反馈接收；反馈默认本地落盘、由用户显式导出；
- 不整体复制 Python/Electron/Rust runtime、品牌站点、生产 endpoint、xAI auth、online memory、
  managed policy、marketplace 或上游发布权限；
- Controller 保持传输无关，system prompt/tool schema 保持缓存稳定；
- Safe Mode、permission、sandbox、evidence、writer worktree 和 fresh-human 边界不能因参考项目而放宽。

## 3. 当前项目状态

- M0、M1、M2、M3、M4 已按路线图关闭。
- M5 所有仓库内、clean-clone 和 CI/CodeQL 可验证事项已关闭；真实运营 registry 的 endpoint、人员/HSM
  密钥仪式、轮换/compromise drill 与独立 DSSE/SLSA policy verifier 保持 `external-blocked`。
- P1 writer worktree、P2 Offline Guard/Safe Mode、P3 Recovery Center、P4 Reasonix 代际 parity、
  P5 受控 Theme Pack 已关闭。P5 最近远端证据：CI `29635818559`、CodeQL `29635818555`、
  Desktop candidate `29635823162` 全绿。
- P6 已在本批关闭：全部 11 个上游/参考仓库更新、代码级分类并冻结；Reasonix 最新 CLI 缺口和
  Hermes BOM 信号已适配。
- 当前树只保留 Go/Wails 产品；旧 Hermes/Python/Electron/TUI/plugin/test/package、`site/`、`workers/`
  已删除，public-readiness 会阻止其回归。
- 内置工具 24 个；CLI 与 Guard 均支持 linux/darwin/windows × amd64/arm64、`CGO_ENABLED=0`。

## 4. 上游冻结点

所有本地镜像在冻结时均 clean，并执行 `fetch --prune --tags`、`pull --ff-only`。本机镜像只是便利缓存；
丢失后按 manifest URL/branch 重新 clone，Git 内的 lock 和审计才是权威。

| 项目 | reviewed SHA | 决策角色 |
|---|---|---|
| DeepSeek Reasonix | `40ef98de92a30a273ee582ec682ab338483109d2` | 唯一主源码上游 |
| Hermes | `4c96172d9bee8542a356610802b9aabc1419f650` | Gateway/错误/运维参考 |
| Codex | `56395bddaf26eb2829387ca6a417bf9128e5b239` | 协议/Hook/LSP/交互参考 |
| MiMo Code | `72e9002e48a71b383b8851b23d65e30c692d68fb` | 设计/技能体验参考 |
| Impeccable | `8967edc988ee146823bca3c51fcf51296e9dec18` | 品牌设计语言参考 |
| Scream Code | `6474e33ad13ffcf11c8eb8a1691af943fe707b2d` | Goal/TUI/主题机制参考 |
| AgentArk | `63985cf819d1760f50f2a5c0dc11d82815e74623` | 安全架构参考 |
| Claude Code | `07dcb0e13580b21174ff1bf6a7e1d5ead3b61d60` | 插件生态 UX 参考 |
| Kimi Code | `3086e4703992fbbe7a41379405ee243713ad9ced` | Desktop Shell/权限文案参考 |
| Grok Build | `98c3b2438aa922fbbe6178a5c0a4c48f85edc8ce` | 安全/终端/ACP 机制参考 |
| awesome-design-md | `664b3e78fd1a298ba11973822da988483256d4b4` | 设计资料参考 |

Reasonix `3637d0f0..40ef98de` 共 5 个非 merge 提交、49 文件、`+1658/-410`；完整逐提交结论和机器账本：

- `docs/audits/2026-07-18-reasonix-3637d0f-40ef98d.md`
- `docs/upstreams/reviews/reasonix-generation-3637d0f-40ef98d.json`
- `docs/upstreams/reviews/reasonix-current.json`

全参考冻结和 Grok intake：

- `docs/audits/2026-07-18-upstream-reference-freeze.md`
- `docs/audits/2026-07-18-grok-build-reference-intake.md`

## 5. 本批代码变化

Reasonix `40ef98de` 的适用部分已按 Reames 状态机重构：

- TUI 捕获鼠标时，右键优先复制活动 transcript selection；无 selection 且 composer 可见时粘贴文本；
- SSH 环境不读取远端主机剪贴板冒充用户本地终端剪贴板，并显示三语提示；
- 右键文本重新进入统一 `tea.PasteMsg`，继续复用文件引用、长文本折叠、completion 和 repaint；
- assistant 回答增加稳定的 `Reames` identity/two-cell gutter；live 与 resume 使用同一投影；
- reasoning → answer、answer → usage receipt 增加语义间距，直接回答不增加首行空白。

Hermes 的 Windows BOM 信号已转成 Reames 修复：`internal/cron.Open` 接受 UTF-8 BOM，下一次成功保存
自动写回无 BOM JSON。Hermes 最终 `4c96172d` 的 CDP 双栈/端口占用修复因 Reames 没有同构
browser-connect runtime 而明确不适用。

上游追踪方面，Reasonix、Hermes、Codex、MiMo、Scream Code、AgentArk、Kimi Code、Grok Build 已启用
路径级 `diff=true`；以后只比较 lock → latest，仍然只自动发现/建单，不自动 merge/cherry-pick。

## 6. 本批本地验证

冻结提交形成前已通过：

- Root：`go build ./...`、`go vet ./...`、`go test ./internal/... -count=1 -timeout 300s`；
- Desktop：`go build ./...`、`go vet ./...`、`go test ./... -count=1 -timeout 300s`；
- Frontend：`corepack pnpm test:all`、`corepack pnpm build`、bundle budget；
- Race：`go test -race ./internal/cli ./internal/cron -count=1 -timeout 600s`；
- Python：143 项脚本/合同测试通过，2 项平台条件跳过；
- 上游：Reasonix generation、19 项 upstream、Node Issue reconciliation、显式逐项目接受；
- 治理：tool 文档、docs/public/deploy/release 合同、actionlint v1.7.7、Git Bash shell syntax；
- 运维：`verify-baseline.ps1` 与 credential-free Gateway smoke；
- 发布形态：六目标 CLI + 六目标 Guard，共 12 个 `CGO_ENABLED=0` 构建。
- clean clone：冻结提交对象通过 `git clone --no-local` 的 Root/Desktop 全量、空 `node_modules`
  Frontend install/test/build/bundle budget，以及 Reasonix/upstream/docs/public 合同。

远端完成声明必须使用最终 push 提交对应的 CI/CodeQL。为避免仅写回 run ID 又触发一次 CI，本文件不
硬编码本批 run ID；新会话使用：

```powershell
gh run list --commit (git rev-parse HEAD) --limit 20
```

## 7. 外部依赖和未关闭边界

以下不能用 mock、localhost 或测试密钥冒充完成：

- 生产 registry HTTPS endpoint、不同人员见证的离线 root/targets threshold ceremony、HSM/等价托管、
  freshness monitor、真实轮换与 compromise drill；
- 声明 builder identity/SLSA level 时的独立 DSSE/SLSA policy verifier；
- 干净 Linux 云节点上的 linger-enabled logout/reboot 与 Gateway recovery/system service 实启；
- 真实 Provider 和飞书/QQ/微信的文本、审批、取消、恢复回环；
- 公开签名 release、Windows/macOS signing/notarization 与真实升级失败/断电点演练；
- NVDA/Narrator 实际听感和 Windows High Contrast 人工验收。

这些是 `external-blocked`，不是仓库失败。没有真实 API key、IM 应用或云服务器时，继续完成仓库内
合同、fixture、fail-closed 和 redaction，但不得把它们写成生产证据。

## 8. 新会话启动顺序

```powershell
Set-Location F:\reames-agent
git status --short --branch
git branch --show-current
git log -1 --oneline --decorate
git fetch origin --prune
git rev-list --left-right --count main...origin/main
python scripts/check_upstreams.py --out-dir artifacts/upstream-watch
gh run list --commit (git rev-parse HEAD) --limit 20
```

判断规则：

1. 工作树应干净，当前分支应为 `main`，`main...origin/main` 应为 `0 0`；否则先审查，不 reset/丢弃。
2. Upstream Watch 若无新提交，不重开 P6；若有新提交，只审 lock → latest。
3. 本批 CI/CodeQL 若失败，先在同一批修复，不用碎片 push 消耗 CI。
4. 电脑清理后若 `F:\code-reference` 丢失，按 `docs/upstreams/upstreams.json` 重建；不要从旧聊天猜 SHA。
5. 远端全绿且用户未提供外部环境时，项目保持暂时冻结；下一主线是 M6 外部证据，不降低门槛。

## 9. Git 与清洁约束

- `artifacts/`、`bin/` 构建产物、Desktop `dist` 生成内容不提交；只提交权威审计和机器账本。
- 大批删除只用显式路径和预览；不执行宽泛 `git clean -fdX`。
- 不使用 `git reset --hard`、`git checkout --` 丢弃未知改动。
- 提交前运行 `git diff --check`、`git diff --cached --check`；push 后核对 CI/CodeQL。
- 当前仓库只维护 `main`；不要为会话交接额外制造长期分支。
