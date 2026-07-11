# 下一位模型接手交接文档

日期：2026-07-11

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
- M2 进行中：依赖棘轮、结构化错误、CLI 会话恢复、版本化 command/event/display DTO 和 prompt metadata 边界已关闭；剩余会话持久化、装配与设置边界待继续收口。
- M6 进行中：Gateway service、headless smoke 和 feedback 本地闭环已具备；真实 IM 与干净云节点仍缺外部证据。

唯一执行顺序以 `docs/DEVELOPMENT_PLAN.md` 为准。

## 当前 M2 批次

详见 `docs/audits/2026-07-11-m2-event-transcript-metadata.md`（上一批命令协议见 `docs/audits/2026-07-11-m2-command-control.md`）：

- `eventwire` 固定 `version=1`，补齐 event source、独立 cache diagnostics 和 session cache counters；Desktop reducer 实际消费 cache update。
- 新增展示安全 `control.TranscriptMessage`；Serve history 与 ACP replay 不再消费 `provider.Message`，system、合成恢复指令、compose 控制块和 referenced-context payload 不再作为远端历史输出。
- Agent/Coordinator 在 Provider interface 前剥离 citation/edit/original 等本地 metadata；OpenAI/Anthropic wire bytes 和 Agent 双轮 cache diagnostics 回归证明它们不改变请求或缓存前缀。
- Gateway prompt 保留显式群聊参与者标签，但排除 connection/domain/chat/user/operator/message ID；ACP replay 的 `provider` 直连和依赖 allowlist 基线同步删除。

## 本批关键本地证据

```text
go build ./...                                      PASS
go vet ./...                                        PASS
go test ./internal/... -count=1 -timeout 10m        PASS
desktop/go test ./... -count=1 -timeout 10m         PASS (190.3s wall time)
desktop/frontend/corepack pnpm test:all              PASS
desktop/frontend/corepack pnpm build                 PASS (既有 chunk/dynamic-import 警告)
Python Desktop/upstream contracts                    PASS (44 tests, 1 skip)
upstream issue + builtin/public/release contracts    PASS
.\scripts\verify-baseline.ps1 -SkipFrontendHint      PASS
```

event/transcript/metadata 批次不改变 Wails UI 或安装工件，未重复触发 Desktop candidate；上一批 production Windows schema v3 candidate 已全绿。当前远端 `main` 为 `f1da4d4`，普通 CI run `29137582416` 为 8/8、CodeQL run `29137582392` 为 3/3。

## 下一执行顺序

1. 继续扩展本地未提交的会话适配器批次：CLI/Bot/Serve/ACP/Desktop 的列表、恢复、租约、cleanup、trash/recovery GC 和设置 rebuild 已迁入 control，本轮十条生产 `agent` allowlist 边已删除，详见 `docs/audits/2026-07-11-m2-session-adapter-boundary.md`。
2. 完成 Desktop prompt/tab 或剩余装配/provider 设置边界的下一条完整纵向路径；全量验证后再集中 commit/push，避免紧跟上一批重复消耗 CI。
3. 然后进入干净云节点 CLI + Gateway + feedback 运维闭环与真实飞书回环。

## 长期未关闭项

- M2 剩余会话持久化、装配与设置边界收口。
- 干净 Linux/云节点的 CLI、Gateway、feedback、日志、备份、升级回滚。
- 真实飞书/Lark 文本、审批、取消与恢复回环。
- plugin 权限 manifest、内容完整性和安装预览。
- 生产签名、notarization、provenance 与 updater 信任链，保持 `external-blocked`。

长期 GOAL 尚未完成。即使本批全绿，也只能声明该批验收完成，不能声明整个项目完成。

## 当前未提交批次验证

会话适配器边界批次已通过 root build/vet/internal 全测、Desktop 全测、前端 `test:all`/build、Python/Node/工具/文档/公开/发布合同及 `verify-baseline.ps1`。工作树仍保留该批生产代码、测试和文档，必须显式暂存并集中提交；受保护文件继续排除。
