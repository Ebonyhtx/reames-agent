# M3 Windows warm startup 审计

日期：2026-07-13

状态：启动预算校准已在远端 installer candidate 通过；同一 run 暴露的交互重启恢复竞态由后续批次收口

## 缺口

原生 Desktop smoke schema v2 只启动一次全新隔离 HOME，能证明冷启动，但路线图中的“热启动”没有第二个真实进程和独立预算。重复运行两条互不关联的命令也不能证明 WebView2 profile、Desktop 状态和 single-instance 资源在关闭后可被同一 HOME 安全复用。

## 实现

- `smoke_desktop_native.py` 的证据 schema 升至 v3。冷启动保持原有首次可见、首次响应、连续三次响应、全观察期存活和显式预算字段，不破坏既有消费者；本地源码 production 默认仍是 8 秒。
- 冷进程完成观察并关闭后，等待 1 秒让窗口、single-instance 锁和 WebView2 子资源释放；随后使用同一 `--home` 和 `REAMES_AGENT_HOME` 启动第二个真实进程。该过程复用首轮已生成的 Desktop 状态和 WebView2 profile，属于跨进程 warm relaunch，不冒充应用内热重载。
- warm 启动独立记录可见、响应、稳定响应、全程响应、退出码、预算和清理结果；默认/候选预算为 6 秒。早退、无响应、超预算、启动失败和清理失败使用 `warm-*` 稳定 failure kind，与冷启动故障可区分。
- 两次启动完成后再检查 HOME 文件、默认用户目录边界和临时目录清理。任何一轮失败都使单份 JSON 证据失败。真实 hosted Windows runner 首轮安装后 11.531 秒才首次响应，因此 candidate 使用 `--observation-seconds 20 --max-startup-seconds 15 --max-warm-startup-seconds 6`；该平台环境预算不替代本地源码 8/6 秒门槛。
- 抽出纯 `classify_startup_observation`，单元测试覆盖健康启动、warm 超预算、warm 早退和冷启动无响应；发布合同冻结 candidate 的观察窗和两条预算参数。

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
commit fb37db1 ordinary CI / CodeQL                            PASS (8/8, 3/3)
commit de893c0 ordinary CI / CodeQL                            PASS (8/8, 3/3)
```

该批源码 production Wails 可执行文件 SHA-256 为 `B2F956553FBA1EBAD12BAB9F34973F04A49B481B63C91F5F41BA66BF4DF08CD0`，大小 47,954,944 B。冷启动首次可见/响应为 0.500 秒、稳定响应为 1.516 秒；同 HOME warm relaunch 首次可见/响应为 0.500 秒、稳定响应为 1.516 秒。两轮各观察 10 秒，均保持单一可见窗口和最终响应，经 `WM_CLOSE` 清理；默认用户状态边界变化为 0，临时 HOME 最终删除。最新源码候选的后续测量见 `2026-07-13-m3-lazy-locale-budget.md`。

远端 `Desktop candidate` run `29209723618` 的 Windows job 安装并启动了真实 NSIS 产物，首次可见/响应为 11.531 秒；旧 12 秒观察期在收集连续三次响应前结束，因此以 `no-response` 失败。进程仍存活、窗口可关闭、状态边界变化为 0，说明这是 harness 观察窗与托管首次安装预算不匹配，不是早退或产品崩溃。该 run 的 Linux/macOS jobs 均成功。

随后把该远端 job 上传的同一 installer（SHA-256 `BF14D29A79D5F28D5A2C3BE201660A447E8EC405B94B31AD2411CFA5D3E981E6`）下载到本地 Windows，实际静默安装后用 20/15/6 秒参数复核：冷启动稳定响应 2.000 秒，warm 稳定响应 1.500 秒，状态边界变化为 0，安装后进程清理和卸载均成功。该证据先验证校准参数与真实候选工件；hosted runner 结果由下一段的后续 run 单独给出。

校准后的 commit `f502c66` 在 `Desktop candidate` run `29210320483` 上形成 hosted Windows 成功启动证据：cold 首次可见/响应 8.047 秒、稳定响应 9.047 秒，warm 首次可见/响应 1.015 秒、稳定响应 2.015 秒，15/6 秒预算均满足；两轮持续响应、清理成功、边界变化为 0。该 run 的 Linux/macOS jobs 也成功，普通 CI `29210316655` 8/8、CodeQL `29210316676` 3/3。

矩阵最终仍因后续 interaction smoke 的“重启后消息不可见”失败。启动 JSON 已独立通过，不能把交互失败误写成启动预算失败；交互根因、修复与后续 hosted 复核见 `2026-07-13-m3-desktop-restart-restore-race.md`。

最终修复后的 candidate run `29211681563` 三平台全部成功；Windows cold 首次/稳定响应 11.016/12.016 秒，warm 首次/稳定响应 1.000/2.000 秒，预算、持续响应、进程与临时目录清理、状态边界全部通过。随后 interaction smoke 也通过，因此该 run 同时给出启动和重启恢复的完整远端证据。
