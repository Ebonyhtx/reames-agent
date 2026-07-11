# M2 prompt 与设置边界审计

日期：2026-07-11

状态：本地全量验证完成，待集中提交与单次推送

## 当前结论

本批继续关闭 prompt、展示和 provider 装配纵向路径：

- 删除 Desktop 自有 `session_prompt.go`，新增 `Controller.AdoptHistoryWithCurrentSystemPrompt`、loaded-history 兼容入口与 opaque `SessionHistorySnapshot`；兼容磁盘基线继续阻止 stale history 覆盖更新尾部，已有 system prompt 按新 runtime 刷新，legacy system-less transcript 不被静默插入新消息；
- Desktop 字节级测试继续证明相同 prompt 的 rebind 请求与 uninterrupted baseline 完全一致，真实 prompt 漂移会明确改变前缀；
- Memory suggestions 通过 `control.LoadTranscript` 读取持久化展示安全投影，不再扫描 system、合成恢复指令、compose 控制块或 referenced-context payload；
- Settings 使用 opaque history snapshot 和 `control.RegisteredProviderKinds`，不再编译依赖 `provider.Message` 或 registry；
- Serve session title provider 由 `boot.SessionTitleGenerator` 装配和调用，明确不把后台标题用量注入主会话事件流；输入限制、引号清理与 chunk error 降级已有单元测试；
- ACP 恢复 metadata title 改用 `control.TranscriptMessage`，隐藏 system、合成消息与 referenced-context payload，避免原始 prompt 内容落入 sidecar/session list；
- 依赖棘轮删除 Desktop session prompt、memory suggestions、settings、Serve title 与 ACP metadata 路径共七条 `agent/provider` 边；Serve、Bot 与 ACP service 已无 runtime 直连。

## 当前验证

```text
go build ./...                                                       PASS
go vet ./...                                                         PASS
go test ./internal/... -count=1 -timeout 10m                         PASS
desktop/go test ./... -count=1 -timeout 10m                          PASS (191.4s)
desktop/frontend/corepack pnpm test:all                               PASS
desktop/frontend/corepack pnpm build                                  PASS
Python Desktop/upstream/installer contracts                           PASS (69 tests, 2 platform skips)
Node upstream issue + docs/public/deploy/release contracts            PASS
go test ./internal/tool -run TestBuiltinToolContractDocumentation -v  PASS
python scripts/check_upstreams.py --out-dir artifacts/upstream-watch  PASS
six-target CGO_ENABLED=0 cross-compile                                PASS
.\scripts\verify-baseline.ps1 -SkipFrontendHint                       PASS
git diff --check                                                      PASS
```

定向测试曾发现 loaded legacy transcript 被新 API 插入 system message 的回归；实现已拆分 loaded-history 兼容策略，原失败用例与新增 control 回归均通过。前端 build 只有既有 dynamic-import/chunk-size 警告且成功。本批尚未推送，以上证据不得冒充远端 CI、真实 Provider 计费缓存或真实 IM 证据。
