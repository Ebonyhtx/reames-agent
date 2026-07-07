# Phase 1: Fork + Rebrand + CI — 完成记录

> 执行日期：2026-07-08
> 状态：✅ 完成
> 基座：DeepSeek Reasonix main-v2 @ 07c65c2

## 执行步骤

### 1.1 仓库初始化
- 创建 `F:\Reames-Agent` 新仓库
- 从 `F:\code-reference\DeepSeek-Reasonix` 复制全部源文件
- 移除原 `.git` 目录，初始化新 git 仓库
- 初始提交：`Initial import from DeepSeek Reasonix main-v2 @ 07c65c2`

### 1.2 Go 模块重命名
- `go.mod`: `module reasonix` → `module reames-agent`
- `desktop/go.mod`: `module reasonix/desktop` → `module reames-agent/desktop`
- `desktop/go.mod`: `require reasonix` → `require reames-agent`
- `desktop/go.mod`: `replace reasonix` → `replace reames-agent`
- 492 个 .go 文件 import 路径全部替换

### 1.3 配置路径和文件名替换
- 配置目录：`~/.reasonix` → `~/.reames-agent`
- Windows：`AppData/Roaming/reasonix` → `AppData/Roaming/reames-agent`
- 配置文件：`reasonix.toml` → `reames-agent.toml`
- 环境变量：`REASONIX_HOME` → `REAMES_AGENT_HOME`
- 内部目录：`.reasonix/` → `.reames-agent/`

### 1.4 二进制和品牌名替换
- 二进制名：`reasonix` → `reames-agent`
- 品牌名：`Reasonix` → `Reames Agent`
- 入口目录：`cmd/reasonix/` → `cmd/reames-agent/`
- 插件示例：`cmd/reasonix-plugin-example/` → `cmd/reames-agent-plugin-example/`
- 更新助手：`reasonix-update-helper` → `reames-agent-update-helper`

### 1.5 Go 标识符修复
- CamelCase 变量：`reames-agentHome` → `reamesAgentHome`
- CGo 函数：`reasonix_main_heartbeat` → `reames_agent_main_heartbeat`
- ObjC 函数：`reasonixApplicationShouldTerminate` → `reamesAgentApplicationShouldTerminate`
- 环境变量前缀过滤：`REASONIX_` → `REAMES_AGENT_`
- 指令提取：`"Reasonix host checks"` → `"Reames Agent host checks"`

### 1.6 前端文件替换
- `desktop/frontend/package.json`：包名更新
- `desktop/frontend/src/locales/zh.ts`：28 处 Reames Agent 引用
- `desktop/frontend/src/locales/en.ts`：全部 Reasonix → Reames Agent
- `desktop/frontend/src/locales/zh-TW.ts`：全部 Reasonix → Reames Agent
- `desktop/frontend/src/styles.css`：品牌注释更新
- 所有 `.ts`/`.tsx` 文件：字符串和标识符更新

### 1.7 构建配置文件
- `Makefile`：目标和二进制名更新
- `.goreleaser.yaml`：二进制名和元数据更新
- `desktop/wails.json`：应用名更新
- `Dockerfile`：二进制名更新
- `.github/workflows/`：CI/CD 配置更新
- `desktop/build/`：NSIS 安装器和 nfpm 包配置

### 1.8 文档和根文件
- `README.md`、`README.zh-CN.md`：品牌更新
- `REASONIX.md` → `REAMES_AGENT.md`
- `reasonix.example.toml` → `reames-agent.example.toml`
- `CHANGELOG.md`：新增 Reames Agent 初始版本记录
- `site/`：域名和品牌引用更新（占位）

## 验收标准

| # | 验收项 | 命令 | 结果 |
|---|---|---|---|
| A1 | go build 零错误 | `go build ./...` | ✅ PASS |
| A2 | 二进制产出 | `go build -o bin/reames-agent.exe ./cmd/reames-agent` | ✅ 58MB |
| A3 | 全仓库零残留 reasonix | `grep -rn 'reasonix' --include='*' -l \| grep -v '.git/' \| grep -v 'node_modules' \| wc -l` | ✅ 0 |
| A4 | Go 模块名正确 | `head -1 go.mod` | ✅ `module reames-agent` |
| A5 | 核心测试通过 | `go test ./internal/config/... ./internal/agent/... ./internal/provider/... ./internal/control/... ./internal/tool/... ./internal/plugin/... ./internal/instruction/...` | ✅ ALL OK |
| A6 | 前端 locales 更新 | `grep -c 'Reames Agent' desktop/frontend/src/locales/zh.ts` | ✅ 28 |

## 回归测试

```bash
# 每次提交前运行
go build ./...                                  # 编译检查
go test ./internal/... -count=1 -timeout 300s   # 全量单元测试
grep -rn 'reasonix' --include='*.go' -l | grep -v 'reames-agent' | wc -l  # 应为 0
```

## Git 记录

```
29e4fb6 Rebrand: Reasonix → Reames Agent
ef3a132 Initial import from DeepSeek Reasonix main-v2 @ 07c65c2
```
