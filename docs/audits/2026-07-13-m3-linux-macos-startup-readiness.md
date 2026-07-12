# M3 Linux/macOS startup readiness 审计

日期：2026-07-13

状态：完整本地门禁与 workflow 门槛通过；真实 Linux/macOS candidate runner 证据待完成

## 缺口

Linux/macOS candidate 已安装或复制真实产物并观察 12 秒，但旧 schema v1 只证明进程没有退出；Linux 虽累计过三个可见窗口采样，却不要求采样连续，也不要求最终仍有窗口。macOS 完全没有 readiness 里程碑，因此不能支持“启动性能达标”声明。

## 实现

- `smoke_desktop_candidate.py` 证据 schema 升至 v2，保留 artifact/executable 哈希、观察期、窗口、清理和状态边界字段，并新增预算、首次状态就绪、首次可见窗口、连续三次稳定就绪、最终 readiness 与 readiness 类型。
- 两个平台都必须在显式 `--home`/`REAMES_AGENT_HOME` 内写出 `desktop-*` 状态文件，并连续三次、直到最终检查仍保持就绪。该信号证明 Desktop 后端和持久化边界已经初始化，不只是进程被操作系统创建。
- Linux readiness 同时要求 `xdotool` 在当前进程下找到可见的 “Reames Agent” X11 窗口；窗口或状态任一消失都会重置连续计数。macOS runner 暂无不依赖隐私授权的稳定窗口探针，因此只使用隔离状态 readiness，文档不得把它写成“窗口可见”。
- Linux/macOS workflow 均显式传入 10 秒预算，默认 12 秒观察期保留最后稳定性余量。早退、未就绪和超预算分别使用 `early-exit`、`startup-not-ready`、`startup-budget` failure kind。
- 发布合同冻结两个 `--max-startup-seconds 10` 参数；单元测试覆盖状态探针、状态+窗口联合里程碑、非连续采样、预算边界、失败分类和 schema 字段。

## 参考与取舍

DeepSeek Reasonix、Codex CLI、Kimi Code 与 Reames Lite 当前没有可直接复用的 Linux/macOS Desktop candidate 启动预算。实现沿用 Reames 已验证的 Windows 原生 smoke 原则：记录首次与稳定里程碑、要求最终仍就绪、对预算失败使用稳定分类，同时保留各平台能诚实观测的不同证据强度。

没有使用 macOS `System Events`/AppleScript 强行探测窗口，因为 CI runner 可能需要 Automation/Accessibility 授权，产生的失败会混入权限状态而非产品启动状态。后续若建立签名 runner 或无授权的 CoreGraphics 探针，可把 macOS readiness 从“状态”提升为“状态+窗口”，但本批不伪造该层。

## 当前证据

```text
python -m py_compile scripts/smoke_desktop_candidate.py        PASS
python -m unittest scripts.test_smoke_desktop_candidate -v     PASS (14)
python -m unittest scripts.test_smoke_desktop_native -v        PASS (21)
python scripts/check_release_contracts.py                      PASS
git diff --check                                               PASS
go build ./... / go vet ./... / go test ./internal/...         PASS
desktop/go test ./... -count=1 -timeout 10m                    PASS
frontend typecheck / test:all / production build               PASS
docs/public readiness contracts                                PASS
Linux/macOS native candidate runner                            PENDING
```

当前证据证明 schema、采样状态机、失败分类与 workflow 参数；Windows 主机不能替代 Linux/X11 或 macOS 原生 runner。只有新的 `Desktop candidate` 三平台 workflow 完成后，才能记录真实首次/稳定 readiness 时间并关闭路线图条目。

为减少远端试错，另只读下载并检查了历史 candidate run `29070966084` 的保留工件：Linux 与 macOS 两份 schema v1 证据均在隔离 HOME 根目录包含 `desktop-projects-legacy-recovered`、`desktop-tabs.json` 和 `desktop-window.json`，边界变化为 0；Linux 还已有可见窗口记录。该历史证据证明新状态探针与真实产物输出兼容，但旧 schema 没有采样时间，不能替代新 workflow 的预算结果。
