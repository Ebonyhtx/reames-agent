# 2026-07-17 P3 Recovery Center 与 Reasonix 最新增量方向审计

## 结论

本批优先完成 P3“Desktop Recovery Center 与发布资格闭环”，并把下一内部 GOAL 定为
P4“受控 Theme Pack”。Reasonix 仍是主源码上游，但采用方式保持人工、按缺口、按 Reames
边界重构；没有合并上游分支，也没有复制其品牌、图片、marketplace、npm/R2、签名或生产
release workflow。

本批直接采用两项可证明缺口：

1. Reasonix `d3cfa5c2`：DeepSeek thinking 可能在 reasoning stream 已给出答案后以
   `finish_reason=stop` 和空普通 content 结束。Reames 新增显式
   `ToolCallReasoningPolicy`，只在 DeepSeek thinking 协议启用时尊重该停止信号；其他 Provider
   继续执行空答案重试，避免把网关误报的 stop 当成完成。
2. Kimi `3086e470`：Auto/YOLO 文案不能宣称“跳过所有权限提示”。Reames 三语文案现在明确为
   “自动批准普通工具”，同时保留 deny、ask、Plan 和 fresh-trust 提示，并有合同测试防止回归。

Reasonix `3637d0f0` 的集中发布审批没有直接采用。Reames 当前没有生产发布基础设施，正确边界是
更小而不是更复杂：只允许无 secrets、`contents: read` 的 candidate workflow。发布合同现扫描全部
workflow，拒绝 production release 文件、写权限、GitHub Release、npm publish、非 snapshot
GoReleaser 和常见 release action。

## Reasonix 审查

本轮 reviewed point：

```text
DeepSeek Reasonix main-v2
reviewed: 3637d0f028bb8223d50ba9490a0ab5140eada4f3
```

### Theme Pack V2：`7f00d2c2`

单提交约 13,624 行，核心机制包括：

- 不可执行、版本化主题 manifest；
- semantic token allowlist；
- 原子导入/替换与内容寻址资产；
- ZIP/path traversal/symlink/文件大小/图片像素限制；
- select 与 apply 分离、可撤销实时预览；
- Safe Mode Graphite 回退；
- 对比度检查、官方/用户主题分区与三语 Gallery。

决定：不搬运整体实现。P4 分三层吸收：先安全主题契约和原子存储，再延迟加载 Gallery 与可撤销
预览，最后只加入 Reames 原创或许可证明确的官方资产。Recovery Center 和 Theme Gallery 都必须
保持 lazy chunk，不得消耗首启预算。

### DeepSeek reasoning-only stop：`d3cfa5c2`

上游证明 DeepSeek thinking 模式会出现非空 `reasoning_content`、空 `content`、
`finish_reason=stop` 的合法终止。Reames 原逻辑会注入可见答案重试，造成模型“任务结束后继续推理”
和额外成本。

Reames 采用：

- Provider 通过能力接口声明当前是否使用启用 thinking 的 DeepSeek 协议；
- Agent 只有在能力为真、usage 非空、finish reason 精确为 stop 且 reasoning 非空时接受；
- 非 DeepSeek、thinking disabled、无 usage、非 stop 或空 reasoning 都保留旧重试；
- Agent 与 OpenAI-compatible Provider 均有定向回归。

### 集中发布审批：`3637d0f0`

上游新增/重构 stable、Desktop、npm 与通用 release workflow，并集中校验 tag、GitHub 审批和
SignPath 人工确认。机制信号是“发布授权必须集中、可测试、不能由 tag 隐式越权”。

Reames 当前生产发布仍禁用，因此不复制这些 workflow，只采用更严格的前置棘轮：全 workflow 扫描、
唯一 candidate allowlist、发布写权限/动作 denylist。真实签名、notarization、受保护 environment、
域名和 registry 所有权到位后，才可另行设计 Reames 自有 canary/stable 流程。

## 其他参考项目

| 项目 | reviewed | 变化 | 决策 |
|---|---|---|---|
| Hermes | `bcea5371c819` | Desktop session-state 单一 store 投影、profile hover prewarm、layout-thrash 优化、session-scope fast mode、best-effort single-writer fence、review-store 测试；末尾两提交仅 formatter 输出 | Reames 已有 Controller/tab profile/epoch 与 Go 静态单二进制边界；作为性能、单一状态源和 version-skew 回归信号，不引入 Python/Electron runtime |
| Codex | `24e9b849fad8` | external config import source 修正；thread MCP connection 集中到 `McpRuntime`；metadata-only 调用只读 rollout header，doctor 最多扫描 64 个非空头部记录并忽略元数据后的损坏尾部 | Reames MCP 生命周期已由 Controller/Host 统一管理；session 列表已有独立 sidecar、bounded index 与长会话 benchmark。保留“连接所有权集中、元数据读取不得扫描全文或受坏尾部拖累”的回归信号，不引入 Rust runtime |
| MiMo | `b48fdba6a15a` | PPTX skill 使用绝对脚本路径并明确 preview service 只适合 CLI | 作为 M7 文档/演示工作流约束，不引入其 JS tool-script runtime |
| Impeccable | `8967edc988ee` | 非 monorepo 嵌套 product 的 `--target` 上下文解析与各 provider 生成副本同步 | Reames 当前没有同构 design-skill target resolver；保留为未来主题/设计 skill 的路径解析测试信号，不移植生成副本 |
| Kimi | `3086e4703992` | 统一 Auto/YOLO 权限文案；VSCode 不越过引擎 blanket approval | 文案准确性已采用；Reames 本身仍以 permission kernel 为唯一决策源 |

接受 reviewed SHA 只表示审查分类完成，不表示 vendor 参考项目的运行时或依赖。

## P3 实现状态

Recovery Center 的单一调用链是：

```text
RecoveryCenter (lazy React chunk)
  -> Desktop RunRecoveryAction
     -> control.Controller.RunRecoveryAction (normal mode)
        or repair.ExecuteAction (recovery-only Safe Mode)
  -> fresh redacted repair.Report
```

已实现动作：配置 repair、snapshot restore、exact undo、verified pending-update rollback、
tabs/projects/window/zoom rebuild、managed plugin disable。更新/undo 使用上一报告的精确 identity，
跨进程 action lock 与底层 store lock 防止 stale UI 或竞争操作误改新状态。所有展示路径和 secret-like
文本在 Go/Wails 边界脱敏，前端请求序号保证最后操作优先。

三平台 candidate 已接入安装后 recovery smoke，检查：

- Guard 存在且可执行；
- recovery-only Safe Mode 到达 ready；
- 损坏 config 与合成 `.env` 字节不变；
- config repair 后 exact undo；
- tabs/projects/window/zoom 只 quarantine；
- 最终 Guard 无 error finding；
- 默认用户状态边界零变化；
- 失败时清理整棵进程树。

最新本地 Windows Wails Desktop/Guard 真实 smoke 通过。Linux/macOS/Windows 远端 candidate 只能在本批
push 后形成；真实签名/notarization、公开 release 升级失败与断电点回滚仍为 `external-blocked`。

## 已执行验证

- Root：`go build ./...`、`go vet ./...`、`go test ./internal/... -count=1 -timeout 300s` 全部通过。
- Desktop：`go build ./...`、`go vet ./...`、`go test . -count=1 -timeout 300s` 通过，主包约 185 秒。
- Frontend：`pnpm test:all` 与 `pnpm build` 通过；Recovery Center 为独立约 34.22 kB chunk，
  localized initial JS 为 999,829 / 1,000,000 bytes。
- Race：`repair`、`guardcmd`、`provider/...`、`agent`、`control`、`pluginpkg`、`gatewayservice`，以及
  Desktop Recovery/SafeMode/Guard/Update 定向 race 通过。
- 六目标：linux/darwin/windows × amd64/arm64 的 CLI 与 Guard 均以 `CGO_ENABLED=0` 编译通过。
- 合同：Python discovery 133 tests / 2 platform skips；docs/deploy/release/public、Desktop artifact、
  tool documentation、upstream issue reconciliation、actionlint v1.7.7、`bash -n desktop-build.sh` 通过。
- 真实本地 Windows Wails Desktop/Guard recovery smoke 再次通过。
- 最新本地上游镜像已 fast-forward，并逐项写入 reviewed lock；未使用 `--accept-all`。

`--no-local` clean clone 仍在首次大提交后执行；远端 CI/CodeQL/candidate 只能在单次 push 后形成。

## 下一 GOAL

P3 通过本地全量并集中单次 push，随后等待 CI、CodeQL 和三平台 Desktop candidate。全部远端仓库内
门槛全绿后，创建 P4 GOAL：交付受控 Theme Pack 的安全内核、原子存储、按需 Gallery、可撤销预览、
Safe Mode 回退和原创资产 provenance；任何 P3 远端失败都优先于 P4 功能。
