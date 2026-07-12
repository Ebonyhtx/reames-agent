# M3 Windows warm startup 审计

日期：2026-07-13

状态：完整本地门禁和当前源码 production Wails 实测通过；commit 与远端 candidate 证据待完成

## 缺口

原生 Desktop smoke schema v2 只启动一次全新隔离 HOME，能证明冷启动，但路线图中的“热启动”没有第二个真实进程和独立预算。重复运行两条互不关联的命令也不能证明 WebView2 profile、Desktop 状态和 single-instance 资源在关闭后可被同一 HOME 安全复用。

## 实现

- `smoke_desktop_native.py` 的证据 schema 升至 v3。冷启动保持原有首次可见、首次响应、连续三次响应、全观察期存活和 8 秒预算字段，不破坏既有消费者。
- 冷进程完成观察并关闭后，等待 1 秒让窗口、single-instance 锁和 WebView2 子资源释放；随后使用同一 `--home` 和 `REAMES_AGENT_HOME` 启动第二个真实进程。该过程复用首轮已生成的 Desktop 状态和 WebView2 profile，属于跨进程 warm relaunch，不冒充应用内热重载。
- warm 启动独立记录可见、响应、稳定响应、全程响应、退出码、预算和清理结果；默认/候选预算为 6 秒。早退、无响应、超预算、启动失败和清理失败使用 `warm-*` 稳定 failure kind，与冷启动故障可区分。
- 两次启动完成后再检查 HOME 文件、默认用户目录边界和临时目录清理。任何一轮失败都使单份 JSON 证据失败；candidate workflow 显式传入 `--max-startup-seconds 8 --max-warm-startup-seconds 6`。
- 抽出纯 `classify_startup_observation`，单元测试覆盖健康启动、warm 超预算、warm 早退和冷启动无响应；发布合同冻结 workflow 中的两条预算参数。

## 当前证据

```text
python -m unittest scripts.test_smoke_desktop_native -v         PASS (21)
python scripts/check_release_contracts.py                       PASS
wails v2.12.0 production build windows/amd64                   PASS (48.7s)
smoke_desktop_native.py --observation-seconds 10 \
  --max-startup-seconds 8 --max-warm-startup-seconds 6         PASS
go build ./... / go vet ./... / go test ./internal/...         PASS
desktop/go test ./... -count=1 -timeout 10m                    PASS
frontend typecheck / test:all / production build               PASS
docs/public/release contracts                                  PASS
```

当前源码 production Wails 可执行文件 SHA-256 为 `B2F956553FBA1EBAD12BAB9F34973F04A49B481B63C91F5F41BA66BF4DF08CD0`，大小 47,954,944 B。冷启动首次可见/响应为 0.500 秒、稳定响应为 1.516 秒；同 HOME warm relaunch 首次可见/响应为 0.500 秒、稳定响应为 1.516 秒。两轮各观察 10 秒，均保持单一可见窗口和最终响应，经 `WM_CLOSE` 清理；默认用户状态边界变化为 0，临时 HOME 最终删除。

本地实测证明当前 Windows production 二进制与 schema v3 harness；它不是安装器 candidate，也不替代下一次远端 Windows runner 证据。Linux/macOS 仍使用通用 candidate smoke，尚未建立等价启动时间预算。
