# M3 Windows 显示缩放持久化审计

日期：2026-07-13

状态：已交付（commit `9d8368c`；普通 CI `29203366720` 8/8、CodeQL `29203366703` 3/3）

## 缺口

Windows Wails 已能在启动时读取 `desktop-zoom.json` 并设置 WebView2 `ZoomFactor`，但设置页只写入“下次启动值”：没有可执行的重启入口，也没有区分当前已应用值与待应用值。范围滑块的连续 `onChange` 还会并发调用 Go 绑定，旧请求若更晚完成，可能覆盖用户最后一次选择；后端直接 `os.WriteFile`，进程中断时也可能留下截断 JSON。

## 实现

- 新增 `createZoomWriteQueue`，同一时刻只运行一个 Wails 写入；拖动期间的待处理值合并为最新选择，所有调用者最终观察同一个已持久化值，失败后清空旧队列并允许重试。
- 设置页分别跟踪启动时已应用、当前选择、最后持久化三个值；写入期间显示保存状态，成功后只在持久化值不同于启动值时显示“重启后应用”，改回启动值会取消待重启状态。
- “立即重启”调用既有 `RestartApplication` 绑定；写入失败回滚到最后成功值并进入现有设置错误通道。范围控件补百分比 `aria-valuetext`，中英繁三语文案保持同构。
- Go 端拒绝 NaN/正负无穷，继续把有限值限制在 50%-200%；写入在互斥区内复用 `internal/fileutil.AtomicWriteFile`，以 0600 权限完成 fsync + 原子替换，避免配置撕裂。

## 参考与取舍

DeepSeek Reasonix 提供了 WebView2 启动缩放、持久化文件和进程重启绑定，是本路径的源码上游；其前端仍并发写入且没有重启动作，本批在现有 Wails 边界上补齐一致性和用户闭环。没有引入 Electron 的即时缩放 API，也没有用 CSS `zoom` 模拟 WebView2：这会制造窗口拖拽/坐标空间的第二套语义。`useWailsResizeFix` 继续处理非 100% WebView2 缩放下的 frameless resize 坐标。

## 当前证据

```text
desktop/go test . -run TestDesktopZoomFactor -count=1        PASS
corepack pnpm typecheck                                      PASS
corepack pnpm test:dpi-scale                                 PASS (12)
tsx src/__tests__/settings-refresh-snapshot.test.tsx         PASS (37)
corepack pnpm test:all                                       PASS
corepack pnpm build                                          PASS (bundle budget enforced)
desktop/go test ./... -count=1 -timeout 10m                  PASS
go build ./... / go vet ./... / go test ./internal/...       PASS
docs/public readiness contracts                              PASS
in-app Browser http://127.0.0.1:5173                         PASS
```

本批 production 产物为 entry JS 623,888 B、initial JS 1,213,105 B（5 files）、initial CSS 607,569 B、largest JS 704,186 B，全部低于既有硬预算。

Go 测试覆盖默认值、持久化、上下界、非有限值拒绝、旧值保留和损坏/越界文件回退。前端队列测试使用可控异步门验证“首个写入执行中时，中间值不落盘、最后值最后落盘”，并覆盖失败后恢复。设置组件测试证明待重启提示和原生重启绑定调用。

真实浏览器在设置 → 外观中把显示缩放从 100% 调到 105%，观察到 slider=105 和“重启后应用 105%”状态，点击“立即重启”后无 warning/error；mock bridge 不退出浏览器。随后恢复到 100%，状态回到“已在启动时应用”。该证据证明 React 交互与 mock 绑定合同，不冒充 production Wails 真正进程退出/重启；原生启动时应用证据沿用既有 Windows candidate，而本批原生重启点击仍待后续抽查。
