# Reames Agent — v0.1.0 → v1.0.0 路线图

> 创建：2026-07-08 | 当前：v0.1.0-dev | 目标：可发布的 v1.0.0

---

## Phase A: 集成验证（1-2天）—— 把写了但没验证的模块串起来

### A1. 新工具接入 agent pipeline
| 任务 | 当前状态 | 要做的事 |
|---|---|---|
| `web_search` | 工具注册了，UT 过了 | 拿真实查询跑 DuckDuckGo，确认 HTML 解析能出结果 |
| `apply_patch` | 解析 + 应用逻辑有测试 | 拿 git diff 输出测试完整链路 |
| `list_jobs` | 注册了，contract 测试过了 | 启动 background bash，调 list_jobs 验证能列出 |

### A2. 安全模块接入 pipeline
| 任务 | 当前状态 | 要做的事 |
|---|---|---|
| `crypto` | AES/Argon2 正确 | 接入 `credentials.go`，让密钥加密存储而非明文 `.env` |
| `trust` | 函数写好了 | 在 `web_fetch` 结果返回前调 `WrapUntrusted`；在 agent 输出前调 `RedactSecrets` |

### A3. 基础设施接入 controller
| 任务 | 当前状态 | 要做的事 |
|---|---|---|
| `cron` | store + ticker 有测试 | 在 controller 启动时 `cron.Ticker(ctx, fn)` |
| `board` | `Build()` 有测试 | 在 serve `/api/board` 暴露端点 |
| `pending_snapshot` | 快照写/清/读有测试 | 启动时调 `LoadPendingSnapshots()` 提示用户上次未完成的 prompt |

### A4. 端到端验证（需要 API Key）
| 任务 | 验证方法 |
|---|---|
| CLI 交互 | `./reames-agent` 输入消息 → 收到回复 |
| serve 远程 | `./reames-agent serve` → 浏览器 localhost:8787 → 发消息 |
| IM 通道 | `./reames-agent gateway start --channels feishu` → 手机发消息 |
| Telegram | 注册 @BotFather bot → 填 token → `gateway start --channels telegram` |
| Docker | `docker build` → `docker run` → curl /health 返回 200 |

---

## Phase B: 质量收口（2-3天）—— 修 bug + 补测试 + 性能基线

### B1. 修复已知问题
| 问题 | 位置 |
|---|---|
| serve 测试 `TestServeIndexPagePassesLanguagePreferenceToClient` 失败 | `internal/serve/serve_test.go:324` |
| `control` 全量测试偶发超时 | `internal/control/controller_test.go` |
| `agent.go` 巨型文件（~10500行），需要文档化拆解方案 | `internal/agent/agent.go` |

### B2. 补测试覆盖
| 包 | 当前 | 目标 |
|---|---|---|
| `internal/crypto` | 4 tests | 加边界：短密文、损坏数据、大文件 |
| `internal/trust` | 3 tests | 加中文密钥检测、嵌套 HTML |
| `internal/board` | 3 tests | 加 mock controller 的完整 `Build()` 测试 |
| `internal/cron` | 3 tests | 加并发 Add/Remove、Ticker 触发测试 |
| `internal/control/pending_snapshot` | 3 tests | 加 approvalManager 集成测试 |

### B3. 性能基线
| 指标 | 目标 | 测量方法 |
|---|---|---|
| 二进制大小 | < 60MB | `ls -lh bin/reames-agent` |
| 冷启动时间 | < 200ms | `time ./reames-agent version` |
| 空闲内存 | < 50MB | 启动后 `ps` 或 `tasklist` |
| 全量测试时间 | < 5min | `go test ./internal/... -count=1` |

---

## Phase C: 发布准备（1天）

### C1. 发布产物
| 任务 | 详情 |
|---|---|
| 交叉编译验证 | `make cross` 产出 darwin/linux/windows x amd64/arm64 共 6 个二进制 |
| `.goreleaser.yaml` 更新 | 确认仓库名、二进制名、homepage、release notes 模板 |
| v0.1.0 git tag | `git tag v0.1.0 && git push origin v0.1.0` |

### C2. 文档最终审查
| 文档 | 检查项 |
|---|---|
| README.md | 安装步骤可执行？示例命令能跑？ |
| README.zh-CN.md | 与英文版内容一致？ |
| DEPLOY.md | Docker/systemd/nginx 命令可复制粘贴执行？ |
| docs/ARCHITECTURE.md | 模块图反映当前结构？ |

### C3. 清理收尾
| 任务 | 详情 |
|---|---|
| `desktop/build/` 残留文件名 | 重命名 `reasonix-desktop.svg`、`reasonix.desktop` |
| `site/` 目录 | 决定保留/删除/归档（Astro 文档站） |
| `workers/` 目录 | 决定是否保留 Cloudflare Workers |
| `.golangci.yml` | 跑一次 lint 确认无警告 |

---

## Phase D: 后续产品增强（v0.2.0+）

### D1. Desktop 体验
| 任务 | 详情 |
|---|---|
| 拆分 `styles.css`（727KB 单体） | 按组件拆为独立 CSS 文件 |
| 品牌 Logo 替换 | 设计 Reames Agent logo SVG |
| 设置面板简化 | `SettingsPanel.tsx` (298KB) 信息架构梳理 |

### D2. 更多渠道
| 平台 | 工作量 |
|---|---|
| Discord | Go SDK 集成，~300行 |
| Slack | Go SDK 集成，~300行 |
| DingTalk | 参照飞书适配，~400行 |

### D3. 高级功能
| 功能 | 源 | 工作量 |
|---|---|---|
| Wolfpack 并行子代理 | Scream Code | ~500行 |
| 自进化系统 (Replay Gate) | AgentArk | ~800行 |
| FTS5 内存搜索 | MiMo Code | ~600行（需引入 SQLite） |
| 工作区文件监听 | Codex | ~300行（需引入 fsnotify） |

---

## 里程碑

- [ ] **M1 — 能跑**：CLI + serve 端到端可用（Phase A4）
- [ ] **M2 — 干净**：0 已知 bug，测试全绿，品牌无残留（Phase B1）
- [ ] **M3 — 可信**：关键路径有测试覆盖，性能基线达标（Phase B2/B3）
- [ ] **M4 — 可发布**：交叉编译通过，文档齐全，tag 打好（Phase C）
