# 2026-07-18 Grok Build 参考项目首次纳入审计

> 范围：SpaceXAI `xai-org/grok-build` 首次公开源码至 GitHub `main`
> `98c3b2438aa922fbbe6178a5c0a4c48f85edc8ce`
>
> 结论：纳入 Reames 的高价值安全与交互机制参考；不成为第二主源码上游。

## 已核验来源

- 官方仓库：`https://github.com/xai-org/grok-build`
- 默认分支：`main`
- 本地只读研究镜像：`F:\code-reference\Grok-Build`
- 首次 baseline/reviewed：`98c3b2438aa922fbbe6178a5c0a4c48f85edc8ce`
- 上游内部源码标识：根目录 `SOURCE_REV` 为
  `124d85bc5dc6e7805560215fcc6d5413944920e1`
- 许可证：第一方代码 Apache-2.0；vendored/ported 代码另受
  `THIRD-PARTY-NOTICES` 和 crate-local notice 约束
- 技术栈：Rust 主体，少量 Python、Shell、JavaScript 和 PowerShell

仓库 2026-07-14 创建，公开 Git 历史当前只有三个提交：首次发布快照和两个
`Synced from monorepo` 批量同步提交。首次快照约 1.43M 行；后两次同步分别变化 117 和
225 个文件。仓库根 README 明确说明它从 SpaceXAI monorepo 周期同步，根目录
`SOURCE_REV` 记录内部源提交。因此后续审查必须按“完整同步快照差异”处理，不能把一个
GitHub sync commit 当成一个单一功能提交。

## 在 Reames 中的角色

Grok Build 的角色是 `security-interaction-reference`，研究重点为：

1. shell 命令解析、权限优先级、deny/ask/allow 与危险操作门禁；
2. OS sandbox、deny-path、child network 和继承边界；
3. session JSONL durability、acknowledged persistence、崩溃恢复和有界长生命周期状态；
4. subagent capability、persona、resume、background task 与 worktree isolation；
5. fullscreen TUI 的 queue/interject、clipboard、scrollback、PTY mode 恢复；
6. headless 输出合同、usage/cost 投影和 ACP 集成；
7. Plugin/Skill/Hook/MCP 的发现、信任、pin 和运行时撤销。

DeepSeek Reasonix `main-v2` 继续是唯一主源码上游。Grok Build 不参与整分支 merge，
也不改变 Reames 的 Go/Wails、`control.Controller`、cache-first、权限、checkpoint、evidence
和 Safe Mode 约束。

## 首次代码级分类

| 上游机制 | 决策 | Reames 适配边界 |
|---|---|---|
| `xai-grok-workspace/src/permission/*` 的命令分段、redirect/source/env gate | 高优先级候选 | 对照 Reames bash permission parser 和 managed command tests；只补可证明缺口，不复制 Rust parser |
| `xai-grok-hooks` matcher recompile fail closed | 候选 | 核对 Reames Hook matcher/config reload 在坏模式下是否扩大匹配面 |
| `xai-grok-sandbox` deny glob、child network inheritance | 候选/负面参考 | 吸收测试思想；Grok 在 macOS child-network 为 no-op，部分 built-in sandbox 应用失败只警告继续，不能降低 Reames fail-closed 目标 |
| durable session append、acknowledged persistence、subagent output 落盘与有界状态 | 高优先级候选 | 对照 Reames M4 commit anchor、child journal、interrupted/continue_from 和跨资源事务；只有真实恢复缺口才实现 |
| subagent capability/persona/worktree/resume | 等价性审查 | Reames 已有预算树、读写能力边界、writer worktree 和 durable child effects；persona input/output contract 可作为未来 UX 信号 |
| TUI queue/interject、clipboard confidence、PTY mode restore、sticky transcript paging | 高优先级体验候选 | 与 Reasonix `40ef98de` CLI 增量合并审查，避免形成第二套 CLI 状态机 |
| headless `--tools`/denylist、structured usage 与 ACP | 候选 | 必须汇入 `control.Controller` 和稳定 event/display DTO，不复制 leader/shell runtime |
| marketplace `require_sha` | 已有更强实现 | Reames 已有 TUF、full commit/tree digest、provenance、fresh-human 审批和生命周期撤销；不降级为可选 SHA pin |
| managed config signed claim/cache | 延后 | 只作为未来 enterprise policy 研究；不接 SpaceXAI 服务、deployment key、sidecar 或远程 fail-closed 主体 |
| xAI auth/model、voice、online memory embedding、dashboard、telemetry/OTEL/Mixpanel/Sentry | 拒绝 | Reames 不建立自有遥测或依赖用户服务器；Provider 与外部连接继续配置驱动、显式授权 |

## 两个必须保留的分歧

### Plan Mode 不能照搬

Grok Build 用户指南明确写明：Plan Mode 只阻止 edit tools，不检查 shell 重定向写入；父会话
Plan Mode 也不约束可写 subagent，且 child 可继承 always-approve。Reames 必须把这些行为视为
负面回归用例，继续由统一 planmode/permission 层阻断写路径，不能因上游实现而放宽。

### Sandbox 声明必须精确

Grok Build 的 child network 限制仅在 Linux 生效，macOS 为 no-op；in-process web/LLM 请求不受
child network 限制。部分 built-in profile 应用失败会告警后继续，只有显式 custom profile 的若干
错误会拒绝启动。Reames 后续若吸收其测试思想，文档和 UI 必须继续准确区分 OS、进程内网络、
child process 和 fail-closed 条件，不能宣称跨平台等价。

## 跟踪与验收

- `docs/upstreams/upstreams.json` 使用 `id=grok-build`、`diff=true`，使同步快照发生变化时进行
  路径级风险分类。
- `docs/upstreams/upstreams.lock.json` 保存首次 baseline/reviewed；后续只能在代码分类完成后
  使用 `python scripts/check_upstreams.py --accept grok-build` 推进。
- 当前 intake 不采用任何 Grok Build 代码，因此不新增运行依赖，也不产生 Apache/第三方代码归属
  变更。未来若复制非平凡实现，必须先逐文件核验 `THIRD-PARTY-NOTICES` 并更新 Reames NOTICE。
- P6 Reasonix `3637d0f0..40ef98de` 已完成；其中 CLI/TUI 变更与 Grok 的终端交互机制完成三方对照。
  后续只从新 lock → latest 形成增量候选，不用新增参考项目重新打开已冻结的主上游区间。

## 本批验证

```powershell
python -m unittest scripts.test_check_upstreams -v
node scripts/test_upstream_watch_issue.mjs
python scripts/check_upstreams.py --out-dir artifacts/upstream-watch
python scripts/check_public_readiness.py
python scripts/check_docs_contracts.py
```
