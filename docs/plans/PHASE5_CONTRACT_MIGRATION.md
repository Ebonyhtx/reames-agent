# Phase 5: 旧 Reames Lite 合约迁移

> 状态：⏳ 待执行
> 源：F:\Reames-Lite（Python 代码库，仅提取接口契约）

## 5.1 接口契约提取

### 改造步骤

1. **ReamesClient 方法清单**
   - 从 `F:\Reames-Lite\packages\core\src\reames\api\client.py` 提取所有公开方法
   - 对照 Reasonix `control.SessionAPI` 接口
   - 记录缺失项到 `docs/GAP_ANALYSIS.md`

2. **类型定义映射**
   - `F:\Reames-Lite\packages\core\src\reames\api\types.py` → Go struct 对照表
   - Message, Session, ChatResult, Usage 等

3. **Desktop API 协议**
   - `F:\Reames-Lite\docs\DESKTOP_API_PROTOCOL.md` → 检查 `internal/serve/` 是否覆盖所有端点
   - 缺失项记录

### 验收标准

| # | 验收项 | 方法 |
|---|---|---|
| A1 | GAP_ANALYSIS.md 产出 | 文件存在，列出所有差异 |
| A2 | ReamesClient 全部方法有对应 | 逐一标注：✅ 已有 / ⚠️ 部分 / ❌ 缺失 |
| A3 | 类型映射完整 | Message/Session/ChatResult/Usage 等核心类型已映射 |

## 5.2 关键合约补全

### 改造步骤

对于 GAP_ANALYSIS.md 中标记的缺失项，按优先级补全：

1. **P0 缺失** — 阻塞功能，立即补
2. **P1 缺失** — 重要但非阻塞
3. **P2 缺失** — 锦上添花

### 验收标准

| # | 验收项 | 方法 |
|---|---|---|
| A1 | P0 项全部关闭 | GAP_ANALYSIS.md 中 P0 计数为 0 |
| A2 | 每个补全有对应测试 | `go test` 覆盖新增代码 |

## 5.3 Reames Lite 归档

### 改造步骤

1. 确认 `F:\Reames-Agent` 可独立构建和运行
2. 将 `F:\Reames-Lite` 标记为 legacy
3. `F:\Reames-Lite\README.md` 添加归档说明，指向 `F:\Reames-Agent`
4. Git tag: `archived-2026-07-08`

### 验收标准

| # | 验收项 | 方法 |
|---|---|---|
| A1 | Reames Agent 独立运行 | 无需 Reames Lite 任何文件 |
| A2 | 归档说明清晰 | README 开头可见迁移指引 |

## 总回归检查

```bash
go build ./...                                    # 编译
go test ./internal/... -count=1 -timeout 300s     # 全量测试
cat docs/GAP_ANALYSIS.md | grep '❌' | wc -l      # P0 应为 0
```
