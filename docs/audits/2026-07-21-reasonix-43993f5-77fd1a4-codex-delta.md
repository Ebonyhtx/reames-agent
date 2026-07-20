# Reasonix `43993f5a..77fd1a47` 与 Codex `678157ac..eceb3eea` 代码级审计

日期：2026-07-21

## 范围与证据

- 一级主源码上游 Reasonix：10 个提交、131 个变化文件、`+6792/-2047`。
- 二级战略代码上游 Codex：7 个提交、71 个变化文件、`+887/-1040`。
- GitHub Git smart-HTTP 当时持续连接超时，因此没有用失败的本地 fetch 冒充审查；提交拓扑、逐提交文件、统计和 patch 来自 GitHub 官方 compare/commit API，并与 Upstream Watch 的 full SHA 对齐。
- Reasonix 机器账本：`docs/upstreams/reviews/reasonix-generation-43993f5-77fd1a4.json`。

## Reasonix 逐提交结论

| 提交 | 结论 | Reames 处理 |
|---|---|---|
| `464d4942` release notes | 不适用 | 不复制 Reasonix 发布说明或发布权限。 |
| `610a4c3d` workspace tree virtualization | 采用 | `WorkspacePanel` 为 TanStack virtualizer 增加 path-based `getItemKey`；展开、折叠、再次展开均验证唯一位置。 |
| `29151f33` ACP mid-turn steering | 采用并加固 | 新增 `_reames-agent/session/steer` vendor extension 与 capability advertisement；只有活动回合可 ACK。Agent 先补 `steerRunActive` 和退出 flush，取消/退出窗口中的 guidance 会持久化到会话并进入下一轮，而不是落入无人消费的队列。 |
| `8cb9e3c2` Remote SSH OpenSSH parity | 延后 P11 | credentials、agent socket、OpenSSH config 与 Desktop UX 属于受治理 Remote SSH 新安全面，不能绕过现有 Controller、permission、sandbox、evidence 与 host-key 门槛。 |
| `4cec2b0f` WebView2 stale proxy / remote image proxy | 延后 | Reames 的 Wails/WebView2 装配不包含同一 vendored patch；远程 Markdown 图片代理涉及 SSRF、DNS rebinding、SVG 清洗和系统代理，是 P3/P10 独立纵向能力，不能只复制前端 URL rewrite。 |
| `0051567c` cc-switch app flag | 采用并品牌适配 | SQLite 优先 `enabled_reames`，其次 `enabled_reasonix`，旧 schema 才回退 `enabled_codex`；legacy JSON 同样采用 Reames → Reasonix → Codex 优先级。 |
| `6fcd41d2` explicit Goal start | 采用 | 普通长文本不再隐式建立 Goal/AutoResearch 或恢复 task path；只有显式 `/goal`/前端 Goal 操作建立持久目标，Goal 内仍按内容或 `--research/--simple` 选择 AutoResearch。 |
| `8b44e4ce` current workspace diff | 延后 P9/Desktop | bounded diff、git authoritative/session fallback 是有效 UX 候选，但需要与 Reames checkpoint alias、symlink/reparse、binary、Wails DTO 和大文件预算独立验收。 |
| `6ded2b3c` Windows creation opener z-index | 不适用 | Reames 当前没有上游 `app--creation` selector；不添加无消费者 CSS。 |
| `77fd1a47` retire automatic Plan Mode | 部分拒绝 | Reames 保留默认 `off`、仅用户显式开启的 `auto_plan=on`；它不是普通提示的隐藏升级。Plan Mode 的 read-only shell、MCP reader trust、child writer 和 fresh approval 边界也强于上游历史实现，因此不整项删除。 |

## Codex 逐提交结论

| 提交 | 结论 | Reames 处理 |
|---|---|---|
| `bf3c1972` legacy exec allow migration | 采用安全不变量 | 权限层拒绝/忽略 shell、解释器、`git:*`、`rm:*`、package-manager-wide script 等可复用宽授权；`python -c ...` 之类的“总是允许”降为精确命令，不生成 `python -c:*`。runtime gate 仍二次过滤旧文件，避免只依赖一次性迁移 marker。 |
| `2deed3fb` zsh tied PATH snapshot | 当前无同构 runtime | Reames 不持久化 Codex shell snapshot；保留为未来 P9 headless shell-environment fixture。 |
| `86102db5` reject unknown history mode before tolerant parse | P9 合同 | App-Server/rollout loader 必须在容错跳过未知行前校验本线程首个 metadata 的 history mode；当前 ACP JSONL 没有该 wire enum，不伪装已采用。 |
| `221a3410` remove unused Rust helpers | 不适用 | 不复制 Rust runtime 清理。 |
| `2244d11a` inline visualization streaming | P9 合同 | 一旦流中出现 inline visualization directive，稳定前缀优化必须退回 source-wide rewrite；当前 Reames 不声明该 Codex directive wire。 |
| `ada5a79d` move deferred lifecycle payloads | 性能信号 | Reames Desktop emitter 已有 bounded queue/coalescing；未来 benchmark 检查大 tool payload 不被为“立即或延后”分支复制。 |
| `eceb3eea` flex-height cache | TUI 性能信号 | Bubble Tea 渲染器不同，不复制 Rust `Renderable`；保留 width-keyed desired-height cache 与多 frame pass benchmark 方法。 |

## 本批实现边界

- ACP extension 不进入 Provider schema，不改变 system prompt/tool order；steer 文本仅作为瞬态 user guidance 持久化和回放。
- 普通输入不再创建 Goal，不影响用户显式设置的长期 GOAL、`/goal --research` 或 host-owned AutoResearch 状态。
- unsafe permission allow 过滤只作用于 `allow`；`ask`/`deny` 仍完整保留，精确命令仍可由 fresh-human 明确保存。
- Remote SSH、WebView2 proxy、workspace current diff 和 Codex App-Server wire 均保持路线图候选，未用局部 patch 冒充完整能力。

## 定向验证

已通过：

```text
go test ./internal/agent ./internal/acp ./internal/control ./internal/config ./internal/permission -count=1
go vet  ./internal/agent ./internal/acp ./internal/control ./internal/config ./internal/permission
go test ./internal/gatewayservice -count=1
go vet  ./internal/gatewayservice
pnpm exec tsx src/__tests__/workspace-changes-errors.test.tsx
pnpm exec tsc --noEmit
git diff --check
```

完整仓库、clean clone、跨目标、CI/CodeQL 要等本集中批次继续累积并正式提交后执行；本审计不复用旧 HEAD 的绿色状态。
