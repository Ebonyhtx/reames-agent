# 2026-07-18 全参考上游最新版冻结审计

> 目的：在电脑清理或会话丢失前，把所有官方参考仓库的最新版、代码级差距和 Reames 决策固化到 Git。

## 冻结快照

全部本地镜像已在干净工作树上执行 `fetch --prune --tags` 和 `pull --ff-only`。11 个仓库均保持
clean；最终 reviewed SHA 由 `docs/upstreams/upstreams.lock.json` 负责。发生主分支变化的 6 个仓库：

| 项目 | 区间 | 提交 | 非 merge | 文件 | 变更量 |
|---|---|---:|---:|---:|---:|
| Reasonix | `3637d0f0..40ef98de` | 5 | 5 | 49 | `+1658/-410` |
| Hermes | `11d36232..4c96172d` | 62 | 57 | 213 | `+15094/-2087` |
| Codex | `b9680065..56395bdd` | 23 | 23 | 309 | `+12425/-1075` |
| MiMo Code | `b48fdba6..72e9002e` | 8 | 4 | 177 | `+17735/-33` |
| Scream Code | `b37627ad..6474e33a` | 7 | 7 | 113 | `+375/-237` |
| Claude Code | `67f390c9..07dcb0e1` | 1 | 1 | 2 | `+104/-27` |

Impeccable `8967edc9`、AgentArk `63985cf8`、Kimi Code `3086e470`、Grok Build
`98c3b243` 和 awesome-design-md `664b3e78` 无主分支差距。Grok Build 首次纳入另见
[`2026-07-18-grok-build-reference-intake.md`](2026-07-18-grok-build-reference-intake.md)。

## 代码级决策

### DeepSeek Reasonix

完整逐提交审计见
[`2026-07-18-reasonix-3637d0f-40ef98d.md`](2026-07-18-reasonix-3637d0f-40ef98d.md)。
直接采用右键文本粘贴/SSH 边界、assistant transcript hierarchy 与语义间距；站点、正式发布链、
registry UI、品牌资产拒绝或不适用。

### Hermes

本区间大部分增量属于 Hermes 的 Python/Electron/TUI、Nous subscription/billing、Codex app-server
transport 和 Dashboard 产品面，不进入 Reames Go/Wails 主树。逐提交源码分类结论：

- **采用**：`51e1fb8f`、`f3612328`、`edfa4cd9` 的 Windows UTF-8 BOM 信号。Reames
  `internal/cron.Open` 现容忍 `EF BB BF`，下一次保存写回 BOM-less JSON，并有回归测试。
- **已有等价或更强**：Codex cache-scope 边界不适用于 Reames Provider；Session/Controller 采用 typed
  event，不存在 Python block-list 无限循环；Go TLS 默认使用系统 root，不需要 Electron CA bridge；
  child registry 按 depth、显式 tool names、read-only/writer worktree 和 durable effects 构建，不存在
  Hermes mixed-platform bundle 重新暴露 blocked tool 的装配路径。
- **架构不适用**：node-pty spawn-helper mode、Electron Windows GPU sandbox fallback、tini shim、
  Honcho/Mem0、Python launcher、Dashboard PTY 单开、Nous billing/subscription。
- **延后到 M6 真实渠道证据**：image routing off event loop 和 gateway base_url/api_mode 切换只作为
  IM/远程 endpoint 回归信号；不复制 Python gateway runtime。
- **最终增量不适用**：`d93c9058`/`4c96172d` 为 `/browser connect` 增加 IPv4/IPv6 CDP 探测、
  非 CDP 端口占用识别、备用调试端口和有界等待。Reames 当前没有 CDP/browser-connect tool 或 Python
  TUI gateway，因此不制造同构功能；将“双栈 loopback + 端口占用者不能冒充协议就绪”保留为未来浏览器工具信号。

### OpenAI Codex

- `SessionEnd` hook：Reames 已有 `hook.Runner.SessionEnd` 并在 close/rotate 路径使用。
- bounded history batch、reverse search pagination：Reames 已有 prompt-history tape、session lazy load、
  cursor pagination、hot/warm/cold transcript 与 1k/10k turn 合同；occurrence search 仅作为未来 UX 候选。
- plugin interstitial requirements：Reames package lifecycle 已有 manifest 权限、fresh-human、TUF、
  provenance 和 generation revocation，强于仅安装提示。
- sub-agent liveness：Reames durable child journal、interrupted/continue_from、budget tree 和 Desktop/Serve
  投影已覆盖；path-backed agent picker 保留为未来 Subagent profile UX 候选。
- realtime V3 handoff、remote executor proxy、SQLite connection、ChatGPT-branded build 和 audio protocols：
  无同构产品面或当前音频需求，不引入第二 app-server/runtime。
- permission instructions world state：Reames 继续以 permission kernel 为唯一决策源，UI/world state 不
  注入 cache-stable prompt。

### MiMo Code

4 个非 merge 提交引入 data-analytics、product-design、sales 大型技能 bundle、autocomplete 和 i18n。
它们是 M7 通用工作能力的内容/发现体验参考，不作为当前内置技能整体复制：17k 行生成内容、连接器
假设和 OpenCode runtime 不能绕过 Reames Plugin/Skill 权限、来源、digest、沙箱和 bundle budget。

### Scream Code

主题改成 acid yellow-green 和 pi-tui fork 不采用。9+ 项 TUI 修复完成逐项对照：Reames branch/session
切换已经清除 todo、chooser、pending approval、plan state 和旧 transcript，并通过 session epoch、tab
reducer 和 last-operation-wins 防 stale event；React/Wails 不使用其 render batcher、Disposable component
timer 或 reverse-RPC panel registry。loop validation、panel auto-dismiss、Escape dialog coverage、tool_result
union 和 typecheck 修复保留为 TUI 回归信号，不引入 TypeScript runtime。

### Claude Code

本区间只有 `CHANGELOG.md` 与 `feed.xml`，没有可审查源码变化。版本信号接受，无代码采用项。

## 追踪机制加固

Reasonix、Hermes、Codex、MiMo、Scream Code、AgentArk、Kimi Code 和 Grok Build 现统一启用
`diff=true`。以后 Upstream Watch 不只比较 GitHub 说明和 SHA，还会拉取 reviewed → latest 的真实
文件列表进行路径级风险分类；仍然只自动发现和建单，绝不自动 merge/cherry-pick。

## 冻结边界

- 本次接受 reviewed SHA 表示上述代码分类完成，不表示版本号、品牌、runtime 或依赖整体同步。
- `F:\code-reference` 丢失时可依据 manifest URL/branch 重新 clone；决策和精确 SHA 已在 Git 中。
- 用户服务器、真实 IM、生产 registry、签名/notarization 和外部 Provider 仍是显式外部证据，未用
  mock 或参考项目声称完成。
- 下一步不再重复审查本冻结点以前的提交，只比较新 lock → latest。

## 本地冻结证据

- Root：`go build ./...`、`go vet ./...`、`go test ./internal/... -count=1 -timeout 300s`。
- Desktop：独立 Go module 的 `go build ./...`、`go vet ./...`、`go test ./...`。
- Frontend：`corepack pnpm test:all`、production build 和 bundle budget。
- Race：`go test -race ./internal/cli ./internal/cron -count=1 -timeout 600s`。
- 上游/文档：Reasonix generation tests、19 项 upstream tests、Node Issue reconciliation、
  public-readiness、docs/deploy/release 合同、tool 文档合同。
- 脚本与运维：143 项 Python tests（2 skipped）、credential-free Gateway baseline smoke、
  actionlint v1.7.7、`bash -n scripts/desktop-build.sh`。
- 发布形态：linux/darwin/windows × amd64/arm64 的 CLI 与 Guard 共 12 个 `CGO_ENABLED=0` 构建。

冻结提交对象已通过 `git clone --no-local` 复验：Root/Desktop 全量、空 `node_modules` Frontend
install/test/build/bundle budget，以及 Reasonix/upstream/docs/public 合同均通过。远端完成声明只接受
最终 push 提交对应的 CI/CodeQL，不复用旧 run 或本地 mock。
