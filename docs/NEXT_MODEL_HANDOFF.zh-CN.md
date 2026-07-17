# 下一位模型接手交接文档

日期：2026-07-17

仓库：`F:\reames-agent`

分支：`main`

本页只记录当前接手边界。实际工作树、`docs/PROJECT.md`、`docs/DEVELOPMENT_PLAN.md`、
审计记录和 GitHub Actions 比本文更权威。

## 用户目标与交付节奏

持续大步推进到高可信可交付状态。参考 DeepSeek Reasonix、`F:\code-reference` 和
`F:\Reames-Lite` 时遵守 `docs/REFERENCE_GOVERNANCE.md`，只吸收适用机制。每个实质批次
同步实现、生产测试、文档和证据；充分本地验证后集中 commit/push，并守候远端 CI，避免
碎片 push 重复消耗资源。

单元/合同测试、localhost fixture、真实浏览器、原生 Desktop、远端 candidate 和真实
Provider/IM/云节点证据必须分层表述，不能互相冒充。

## 工作树纪律

每次开始工作先执行 `git status --short --branch`，区分 tracked 改动、生成产物和用户文件。
大批删除或清理必须使用显式路径和 clean clone 复验，不使用宽泛 `git clean -fdX`；提交前
确认只有当前批次文件进入索引。

## 已验证基线

- M0、M1、M2、M3、M4 已按路线图门槛关闭。
- 本批前远端 `main` 为 `bc40db5 feat: isolate writer subagents in managed worktrees`；当前 P2
  Guard/Safe Mode、Reasonix session reliability 与文档改动尚未提交，必须先以工作树和本批最终
  全量验证为准，不能沿用更早 CI 结果冒充当前 HEAD。
- 同批 workflow 已迁移到 Node.js 24 action majors；上述远端日志未再出现 Node.js 20
  弃用告警。public-readiness 合同扫描 `.yml/.yaml`，拒绝旧 major、未知 ref 和未经审计的
  commit pin。
- 最近完整 Desktop candidate `29378899444` 仍为三平台全绿；Windows 安装后 interaction、
  accessibility、native 和 plugin lifecycle 四条 smoke 均通过，且
  `boundary_changes=[]`、`errors=[]`。
- 初始迁移中隔离的 Hermes/Python runtime、Electron/TUI、旧 plugins/tests/package、
  `site/` 和 `workers/` 已完成依赖审计并从当前树删除；Git 历史和
  `F:\code-reference\Hermes` 保留参考。public-readiness 会阻止 legacy 根目录和运行品牌回归，
  详见 `docs/audits/2026-07-17-repository-cleanup.md`。

## M5 当前边界

已经关闭的可本地/可远端验证链路包括：

- 原生 plugin schema、语义版本、精确权限、不可变 generation、新安装默认禁用，以及
  install/update/rollback/uninstall 的 `preview -> planId -> apply`；
- Desktop、CLI、Bot、Serve/event wire 和 ACP 共用 fresh-human 结构化审批，自动策略和
  headless apply 不能代答；
- generation 变化或禁用会阻止新 work 起跑，串行 rebuild，并撤销旧 MCP/Hook/Skill runtime；
- package-owned Hook/MCP 使用最小环境、独立 state/tmp、严格 OS sandbox、敏感读取阻断和
  进程树回收；真实固定 revision `obra/superpowers` 已完成 Windows sandbox E2E；
- 无默认 endpoint/TOFU 的官方 `go-tuf/v2` registry client，绑定 full Git commit、canonical
  Git tree digest、manifest 权限和 provenance assertion；CLI/Desktop 可发现并展示落盘证据；
- 只读 `plugin registry audit` 从显式带外 root 重放连续 root 轮换，验证旧/新双阈值、角色
  key 隔离、到期窗口和完整 metadata/index/attestation 字节；成功 JSON 保留 public key ID、
  SHA-256 与 `externalRequired` 边界。
- Reasonix MCP identity P0 已按 Reames 边界收口：宿主本地 receipt、identity/capability drift、
  mutable launcher exact content lock、legacy seed 单次迁移、destructive fresh-human、Desktop
  reverify 和共享 Host sibling registry 刷新，见
`docs/audits/2026-07-17-m5-mcp-identity-trust.md`。

## P1/P2 当前边界

- P1 writer worktree isolation 已完成：writer-capable task/Skill/Subagent 使用独立
  `reames/subagent-*` branch/worktree、workspace/ref 跨进程锁和父会话统一交付事务；ambiguous
  post-apply crash 保持 `acceptance_interrupted`，不自动覆盖人工 drift。
- P2 offline Guard/Safe Mode 已实现：三平台 Desktop 入口默认经过 Guard；五分钟三次失败账本、
  30 秒健康观察期、配置快照、完整安装单元 pending transaction、Windows installer failure
  marker 和 mixed-install fail-closed 共用 `internal/repair`。
- Safe Mode 不读取用户/项目 TOML 或 dotenv，不恢复旧 tab/session；Desktop 只建立 recovery-only
  shell，`boot.Build` 拒绝 Provider、Controller、工具和普通 Agent 装配，并禁用 MCP、plugin、Hook、
  Bot、LSP、planner、Guardian、subagent、Memory Compiler、update/telemetry/metrics 等运行面。
- `control.Controller.RecoveryStatus()`、Serve `/api/recovery`、Desktop `GetRecoveryStatus()`、
  Guard check 与 `gateway recovery-status` 共用同一报告；`gateway run` 在加载普通 runtime 前执行
  credential-free preflight。
- Reasonix 已审查到 `c966d0279629`，采用 `dae65e25` 的 stalled error-body deadline、verified
  snapshot fast path、damaged event-log salvage 和 last-click-wins；不直接跟随删除 Memory Compiler。
  Hermes/Codex/MiMo/Claude/Kimi/Scream Code 均逐项审查并显式接受到 lock 中记录的 SHA；没有使用
  `--accept-all`。Codex runtime projection 仅作为 P3 Recovery Center 参考，其他最新增量无采用项。

权威设计、运维与证据见 `docs/RECOVERY.zh-CN.md` 和
`docs/audits/2026-07-17-p2-offline-guard-safe-mode.md`。

本批本地门禁已经覆盖 root build/vet/internal 全测、Desktop build/vet/full test、前端
`test:all`/production build/bundle budget、126 项 Python 合同（2 skipped）、文档/公开/部署/
发布/Desktop artifact 合同、actionlint、恢复相关 Root 与 Desktop race、六目标 CLI + Guard
`CGO_ENABLED=0` 交叉编译；`git diff --check` 与 tracked 生成物检查均干净。`--no-local` clean clone
又从空 Frontend `node_modules` 重跑 Root/Desktop/Frontend/合同并保持 tracked clean；当前只剩单次
push 后的远端 CI/CodeQL 与安装态 candidate 证据。详细证据见：

- `docs/audits/2026-07-14-m5-plugin-lifecycle-trust.md`
- `docs/audits/2026-07-15-m5-plugin-process-isolation.md`
- `docs/audits/2026-07-16-m5-tuf-plugin-registry.md`
- `docs/audits/2026-07-16-m5-registry-operations-audit.md`
- `docs/audits/2026-07-17-m5-mcp-identity-trust.md`

## 未关闭边界

- M5 的真实运营公开 registry 仍为 `external-blocked`：生产 HTTPS endpoint、不同人员见证的
  离线 root/targets threshold ceremony、online role custody、HSM 或等价托管、freshness
  monitor、实际密钥轮换/compromise drill，以及声明 builder identity/SLSA level 时的独立
  DSSE/SLSA policy verifier，均不能由合成密钥或 localhost 冒充。
- package process 当前允许网络，且没有跨三平台统一硬 CPU/RSS 配额；用户手工 Hook/MCP
  与 LSP 仍是高权限进程。这是持续威胁模型限制，不把它误写成生产 registry 已完成或重新
  打开已验收的 M5 仓库内合同。
- M6 的 linger-enabled logout/reboot、干净云节点、真实飞书/QQ/微信回环和公开签名 release
  仍为 `external-blocked`。
- `bash`、MCP、外部 API 和后台 opaque side effect 不具备任意副作用 exactly-once。

## 下一执行顺序

1. 完成当前 P2 的 root/Desktop/frontend/race/六目标/合同/clean-clone 全量验证，清理生成产物，
   形成一个大提交并单次 push；随后等待普通 CI、CodeQL 和必要的 Desktop candidate，失败则在同一
   批次修复，不用碎片 push 消耗 CI。
2. P3 优先建立低噪音 Desktop Recovery Center，并用真实三平台签名安装包补升级失败、crash-loop
   自动回滚、Safe Mode 和恢复动作 smoke；继续只投影 `repair.Report`，不新增状态机。
3. 若生产 registry 的人员、密钥、域名和对象存储条件到位，按双语 runbook 执行真实仪式、
   发布、轮换和 compromise drill，并独立归档证据；未到位时保持明确阻塞，不降低门槛。
4. 取得干净云节点、真实 IM 应用或签名设施后，关闭 M6 的 logout/reboot、Gateway recovery
   preflight 实启、真实渠道回环和发布证据。

长期 GOAL 只有在代码、测试、文档一致、`main` 与最新 CI/CodeQL 全绿，且最近里程碑的所有
可执行事项关闭、剩余事项均准确标记为外部依赖时才能完成。
