# M3 Desktop 性能预算审计

日期：2026-07-12

状态：已交付；commit `e147854`

## 缺口

M2 收官时 production build 虽成功，但入口 JS 为 1,103,017 B、初始 JS 为 1,342,548 B、初始 CSS 为 625,595 B，并持续报告超大 chunk 与无效 dynamic import。只有 source-level `bundle-contract.test.ts`，没有读取真实 `dist` 的硬预算，代码回归仍可悄悄放大首屏。

## 实现

- `App.tsx` 按真实打开状态懒加载 Approval、Ask、Todo、回退/清理、Command Palette、Shortcuts、Onboarding、Heartbeat、Context/Workspace 与既有 History/Settings；未打开时不创建 lazy element。
- Heartbeat 的 26 KB 源样式跟随功能 chunk，不再进入首屏 CSS；ToolCard 复用已经静态存在的 bridge，移除无效 dynamic import 警告。
- 删除未使用的 `@gsap/react` 与 Flip 注册；`ScrollToPlugin` 注册下沉到唯一消费者 `useScrollManager`，保留滚动行为。
- 新增 `bundle-budget.json` 与 `check-bundle-budget.mjs`，构建后从 `dist/index.html` 解析真实 entry、modulepreload 与 stylesheet，并递归测量最大 JS；本地引用必须留在 dist 内。
- 预算固定为 entry JS 650,000 B、初始 JS 1,250,000 B、初始 CSS 620,000 B、最大 JS asset 725,000 B、初始 JS 文件 6 个。Vite warning limit 与严格的最大 asset 预算对齐。
- `smoke_desktop_native.py` 的证据 schema 升至 v2，分别记录首次可见、首次响应和连续三次响应的稳定时间；`--max-startup-seconds` 超限以 `startup-budget` 明确失败。Windows candidate workflow 固定使用 8 秒冷启动预算。

当前 production 结果：

```text
entry JS        621,270 B   (-43.6%)
initial JS    1,209,699 B   (-9.9%) across 5 files
initial CSS     607,374 B   (-2.9%) across 1 file
largest JS      704,186 B   lazy Mermaid dependency, within 725,000 B
```

入口与共享 i18n 分块后的总量都纳入 initial JS 预算，不用“entry 变小”掩盖预加载总量。

## 参考与取舍

DeepSeek Reasonix 上游只按需加载 History/Settings，仍保留 ToolCard 无效 dynamic import 和 600 KB warning；本批扩展其行为合同，不复制另一套前端。MiMo Code 的动态加载主要用于服务端、worker 与可选 observability，没有同构 Desktop 预算。Reames-Lite 的 visual smoke 用于证据分层参考；其 Electron shell 不作为 Wails 实现来源。

## 当前证据

```text
corepack pnpm typecheck                                      PASS
corepack pnpm test:typecheck                                 PASS
node scripts/test-bundle-budget.mjs                          PASS (4)
tsx src/__tests__/bundle-contract.test.ts                    PASS (15)
corepack pnpm test:all                                       PASS
corepack pnpm build                                          PASS
node scripts/check-bundle-budget.mjs                         PASS
in-app Browser http://127.0.0.1:5173                         PASS
Wails v2.12.0 production Windows build                       PASS
python -m unittest scripts.test_smoke_desktop_native -v      PASS (20)
smoke_desktop_native.py --max-startup-seconds 8              PASS
go build ./... / go vet ./... / go test ./internal/...       PASS
desktop/go test ./...                                        PASS
CI-scoped Python contracts                                   PASS (71, 2 skips)
docs/public/deploy/release/tool contracts                     PASS
verify-baseline.ps1 -SkipFrontendHint                         PASS
six-target CGO_ENABLED=0 cross-compile                       PASS
check_upstreams.py                                            PASS (changed_count=9)
```

真实浏览器点击覆盖命令面板、快捷键设置、自动化面板、右侧概览和文件视图的首次打开与关闭；页面 warning/error 日志为 0。该证据使用 Vite dev 的确定性 mock bridge，证明 lazy chunk 可加载和交互，但不冒充真实 provider 证据。

最新源码重建的 Windows production Wails 可执行文件 SHA-256 为 `8F94E326543AC2BDDAB8EB2983E1CBAFDC6BE63C07950A148C771438601812CC`，原生 smoke 实测首次可见与首次响应均为 1.016 秒、稳定响应为 2.016 秒，满足 8 秒冷启动预算；12 秒观察期内进程存活、窗口持续响应、隔离 HOME 外无状态变化，清理成功。JSON 证据写入本地 `artifacts/desktop-native-startup-budget.json`，不纳入 Git。

该原生证据只证明当前 Windows/amd64 环境的冷启动门槛，不外推为热启动或 Linux/macOS 性能结论。热启动 harness、Linux/macOS candidate 启动预算和原生 Wails 功能点击仍留在 M3 后续批次。

commit `e147854` 已推送至 `main`。普通 CI run `29196957695` 的 8 个 jobs 全部成功，CodeQL run `29196957665` 的 Go、JavaScript/TypeScript、Actions 3 个分析全部成功；Node 20 action 弃用与缓存恢复提示均为非阻断 annotation。本批没有为纯证据再次 push，也未触发昂贵的三平台 Desktop candidate。
