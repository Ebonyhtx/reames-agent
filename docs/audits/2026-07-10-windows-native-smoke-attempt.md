# Windows 原生 Desktop smoke 尝试

日期：2026-07-10

基线提交：`92d79ae`

## 已取得证据

1. `corepack pnpm test:all` 与 `corepack pnpm build` 通过。
2. 使用固定版本 Wails CLI `v2.12.0` 从当前源码生成绑定并构建 Windows amd64 应用。
3. 构建产物为 `desktop/build/bin/reames-agent-desktop.exe`，大小 47,870,464 bytes，SHA-256 为 `A94B687006A7E1D25A6B96F99A68F25B4A2E21A1140666E71DF39396A0AB4CDE`。
4. 原生进程成功启动并保持 `Responding=True`，未发生启动崩溃；验证后已关闭进程。

构建命令：

```powershell
Push-Location desktop/frontend
corepack pnpm build
Pop-Location

$env:GOPROXY = 'https://goproxy.cn,direct'
$env:GOCACHE = '<workspace>/.tmp/wails-go-cache'
Push-Location desktop
go run github.com/wailsapp/wails/v2/cmd/wails@v2.12.0 build -clean -s
Pop-Location
```

`-s` 只跳过 Wails 对裸 `pnpm` 的再次调用；嵌入的 `frontend/dist` 来自前一步已通过的 production build。

## 未取得的证据

Windows 自动化在激活和被动捕获 frameless Wails 窗口时均返回：

```text
SetIsBorderRequired failed: 不支持此接口 (0x80004002)
```

按验证安全规则，第二次捕获失败后停止了所有窗口输入。因此本次没有执行新建会话、选择工作区、发送、停止或恢复点击流，也不能将 M1 原生点击项标记为完成。

应用没有独立的 `--home` 启动参数；本次启动读取并刷新了用户级 Desktop 状态文件。后续原生 smoke 应优先提供可隔离进程环境的启动方式，并使用支持 frameless WebView2 的捕获/点击通道；若改为人工验证，需记录 commit、步骤、结果和不含密钥的截图。
