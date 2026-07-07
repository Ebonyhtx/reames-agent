# Phase 6: 全面验证与收口

> 状态：⏳ 待执行
> 触发条件：Phase 2-5 全部完成后执行

## 6.1 自动化验证

### 验证步骤

1. **编译检查**
   ```bash
   go build ./...
   go build -o bin/reames-agent.exe ./cmd/reames-agent
   cd desktop/frontend && npm run build
   ```

2. **全量单元测试**
   ```bash
   go test ./... -count=1 -timeout 600s
   cd desktop/frontend && npm test
   ```

3. **Lint 检查**
   ```bash
   golangci-lint run ./...
   cd desktop/frontend && npx tsc --noEmit
   ```

4. **品牌残留检查**
   ```bash
   grep -rn 'reasonix\|Reasonix' --include='*.go' --include='*.ts' --include='*.tsx' --include='*.css' -l | grep -v node_modules | grep -v reames-agent
   # 应为空
   ```

5. **交叉编译检查**
   ```bash
   make cross
   # 确认产出 6 平台二进制
   ```

### 验收标准

| # | 验收项 | 期望 |
|---|---|---|
| A1 | go build | 零错误 |
| A2 | npm build | 零错误 |
| A3 | go test ./... | 全 PASS |
| A4 | npm test | 全 PASS |
| A5 | golangci-lint | 零 warning |
| A6 | tsc --noEmit | 零错误 |
| A7 | 品牌残留 | 0 结果 |
| A8 | 交叉编译 | 6 平台二进制全部产出 |

## 6.2 功能手动验证

### 验证步骤

1. **CLI 交互**
   - `./bin/reames-agent` → 启动 TUI → 输入消息 → 收到回复
   - `/help` → 帮助菜单正确
   - `/model` → 模型切换正常
   - `Ctrl+C` → 取消当前请求

2. **Web/Cloud 访问**
   - `./bin/reames-agent serve` → 浏览器 `localhost:8787`
   - 输入消息 → 收到回复
   - 多会话创建和切换
   - Token 认证

3. **Desktop 应用**
   - 启动 Wails Desktop
   - 设置 API Key
   - 发送消息 → 收到回复
   - 设置面板、命令面板正常工作

4. **Docker 部署**
   - `docker build -t reames-agent .`
   - `docker run -p 8787:8787 reames-agent`
   - 浏览器访问 → 正常工作

5. **IM 通道**
   - `reames-agent gateway start --channels feishu`
   - 飞书发送消息 → Agent 回复
   - 审批卡片 → 双向同步

### 验收标准

| # | 验收项 | 方法 |
|---|---|---|
| M1 | CLI 正常交互 | 手动测试 |
| M2 | Web 正常访问 | 手动测试 |
| M3 | Desktop 正常启动 | 手动测试 |
| M4 | Docker 正常运行 | 手动测试 |
| M5 | IM 通道正常 | 手动测试（需测试 bot token） |

## 6.3 性能基线

### 验证步骤

1. **二进制大小**
   ```bash
   ls -lh bin/reames-agent.exe     # 目标 < 60MB
   ls -lh bin/reames-agent-plugin-example.exe
   ```

2. **启动时间**
   ```bash
   time ./bin/reames-agent version  # 目标 < 200ms
   ```

3. **内存占用**
   ```bash
   # 空闲时 RSS 目标 < 50MB
   ```

### 验收标准

| # | 验收项 | 期望 |
|---|---|---|
| B1 | 二进制大小 | < 60MB |
| B2 | 启动时间 | < 200ms |
| B3 | 空闲内存 | < 50MB |

## 6.4 Git 最终提交

```bash
git add -A
git commit -m "Release: Reames Agent v0.1.0

Phases 1-6 complete:
- Phase 1: Fork + Rebrand + CI
- Phase 2: Cloud deployment (Docker + serve)
- Phase 3: IM Gateway (Feishu/WeChat/QQ + extensible)
- Phase 4: Brand visual system
- Phase 5: Reames Lite contract migration
- Phase 6: Full verification

Baseline: DeepSeek Reasonix main-v2 @ 07c65c2 (MIT)"
git tag v0.1.0
```

## 总回归检查

```bash
# Final gate — 全部必须通过
go build ./...                                         # 1. 编译
go test ./... -count=1 -timeout 600s                   # 2. 全量测试
cd desktop/frontend && npm run build && npm test       # 3. 前端
make cross                                              # 4. 交叉编译
grep -rn 'reasonix' --include='*.go' --include='*.ts' --include='*.tsx' -l | grep -v node_modules | grep -v reames-agent | wc -l  # 5. 应为 0
docker build -t reames-agent .                          # 6. Docker
```
