# 下一位模型接手交接文档

日期：2026-07-12

仓库：`F:\reames-agent`

工作分支：`main`

本页只记录当前接手边界。代码、`git status`、`docs/PROJECT.md`、`docs/DEVELOPMENT_PLAN.md` 和最新远端 CI 结果优先级更高。

## 用户目标与节奏

持续把 Reames Agent 推进到高可信可交付状态；参考 DeepSeek Reasonix、`F:\code-reference` 和 `F:\Reames-Lite` 吸收机制，但不盲目复制。每批同步代码、测试和文档，形成足够大的本地成果后再集中 commit/push，避免碎片 push 重复消耗 CI。

结论必须区分单元/合同测试、localhost 模拟、原生 Desktop、远端 candidate 与真实 Provider/IM/云服务证据，不能用其中一层替代另一层。

## 受保护文件

以下是用户或其他会话的未跟踪文件，禁止修改、暂存或提交：

```text
.agents/
docs/audits/2026-07-09-reference-feature-gap-map.md
```

只使用显式 `git add -- <paths>`，禁止 `git add .` 或 `git add -A`。

## 当前项目状态

- M0 已关闭：普通 CI、CodeQL、六目标 CLI candidate、三平台 Desktop candidate 和原生安装 smoke 均有历史远端证据。
- M1 已关闭：真实 Provider、原生会话/工作区/停止、文件审批/落盘/回退、重启恢复，以及 401/429/断流/权限拒绝/工具超时均有分层证据。
- M2 已关闭：依赖棘轮 allowlist 已归零，结构化错误、版本化 command/event/display DTO、prompt metadata、会话持久化、Desktop/ACP/CLI 装配和终端渲染边界均已收口；完整本地门禁及远端 CI/CodeQL 已通过。
- M3 进行中：首批关闭态/次级界面拆包和真实产物 bundle 预算已形成，入口 chunk 下降 43.6%，初始 JS 下降 9.9%；Windows production Wails 已建立 8 秒冷启动门槛，当前稳定响应实测 2.016 秒。
- M6 进行中：Gateway service、headless smoke 和 feedback 本地闭环已具备；真实 IM 与干净云节点仍缺外部证据。

唯一执行顺序以 `docs/DEVELOPMENT_PLAN.md` 为准。

## 最新已交付 M2 收官批次

- compile-time provider 注册与 review subagent 装配归入 boot，CLI resume/rebuild 改用 opaque control handle，ANSI 输出归入 termrender。
- TUI replay/copy/export 使用展示安全 transcript，并修复 hidden synthetic user 截断复制范围；模型切换使用新 controller system prompt。
- transport allowlist 归零，Desktop、CLI、Serve、Bot 与 ACP 的受守卫生产文件累计收缩四十条 `agent/provider/tool` import。

详见 `docs/audits/2026-07-12-m2-transport-boundary-closeout.md`。commit `453a51c` 已推送；本地全量门禁通过，CI run `29195337394` 为 8/8、CodeQL run `29195337395` 为 3/3。

## 当前 M3 候选批次关键证据

```text
go build ./...                                           PASS
go vet ./...                                             PASS
go test ./internal/... -count=1 -timeout 10m             PASS
desktop/go test ./... -count=1 -timeout 10m              PASS (206.7s wall time)
desktop/frontend/corepack pnpm test:all                   PASS
desktop/frontend/corepack pnpm build                      PASS (bundle budget enforced)
Python Desktop/upstream/installer contracts               PASS (71 tests, 2 platform skips)
docs/public/deploy/release/tool contracts                  PASS
Wails v2.12.0 production Windows build                    PASS
Windows native startup budget (8s)                        PASS (stable 2.016s)
six-target CGO_ENABLED=0 cross-compile                    PASS
.\scripts\verify-baseline.ps1 -SkipFrontendHint           PASS
python scripts/check_upstreams.py                          PASS (changed_count=9)
```

当前本地候选改变 Wails UI 与 Windows native smoke schema，尚未触发远端 Desktop candidate。上一批 production Windows schema v3 interaction candidate 已全绿。当前远端 `main` 仍为 `453a51c`，普通 CI run `29195337394` 为 8/8、CodeQL run `29195337395` 为 3/3。

## 下一执行顺序

1. 显式暂存当前 M3 性能候选，集中提交并单次 push；守候普通 CI 与 CodeQL，不为纯证据另做 push。
2. 冷启动硬门槛已经建立；热启动与 Linux/macOS candidate 预算继续后续 M3 批次。
3. 随后继续 M3 原生日用化/可访问性，再进入干净云节点 CLI + Gateway + feedback 运维闭环与真实飞书回环。

## 长期未关闭项

- M3 原生 Desktop 日用化、可访问性和启动/bundle 性能门槛。
- 干净 Linux/云节点的 CLI、Gateway、feedback、日志、备份、升级回滚。
- 真实飞书/Lark 文本、审批、取消与恢复回环。
- plugin 权限 manifest、内容完整性和安装预览。
- 生产签名、notarization、provenance 与 updater 信任链，保持 `external-blocked`。

长期 GOAL 尚未完成。即使本批全绿，也只能声明该批验收完成，不能声明整个项目完成。

## 当前未提交 M3 批次

M2 收官远端证据与路线图状态回填已合并进当前 M3 批次。Desktop 前端已按需拆分 Approval/Ask/Todo/回退/清理、命令面板、快捷键、Onboarding、Heartbeat、Context/Workspace 等界面，移除未使用的 `@gsap/react`/Flip，新增真实 dist 预算与四项夹具；完整 `pnpm test:all`、production build 和本地浏览器点击已通过。最终 production 数据为 entry JS 621,270 B、initial JS 1,209,699 B、initial CSS 607,374 B、largest JS 704,186 B、initial files 5。Windows production Wails 重建成功，schema v2 原生 smoke 在隔离 HOME 下测得首次可见/响应 1.016 秒、稳定响应 2.016 秒，满足 8 秒预算且无边界变化。完整本地门禁均已通过；commit、单次 push 与远端证据尚待完成，受保护文件继续排除。
