# M3 主首启图与设置 CSS 拆分审计

日期：2026-07-13

状态：已交付（commit `7d07c89`；普通 CI `29216174519` 8/8、CodeQL `29216174514` 3/3）

比较基线：已交付 commit `bbdddde`；普通 CI `29214262280` 8/8、CodeQL `29214262276` 3/3

## 缺口

多语言按需加载关闭了词典对基础首启图的占用，但已交付基线仍有三块只在特定路径需要的成本进入 production initial graph：浏览器开发模式的完整 mock bridge、由输入菜单使用的 TanStack 虚拟化实现，以及设置中心约 120 KB 源码 CSS。它们分别服务 localhost 开发兜底、`@`/`/` 菜单首次打开和设置中心首次打开，不应由所有原生 Desktop 用户在首帧承担。

只看 entry 文件会掩盖共享 chunk 的重新分组，单纯拆出文件也可能被静态 import 或传递依赖重新提升到 initial graph。因此本批把拆分机制、真实 manifest 递归图和首次使用预算作为同一个合同，不以人为切碎 chunk 或牺牲首屏可用性换取表面数字。

## 实现

- `bridge.ts` 保留稳定的 production binding/interface，通过缓存的动态 import 首次加载 `bridgeMock.ts`。异步边界完成后再次检查 Wails binding/runtime，使启动期间较晚注入的真实原生绑定仍优先于 browser mock；事件、updater 订阅与取消语义有独立回归。
- `VirtualMenu.tsx` 成为轻量、保留泛型调用面的 Suspense 包装器；真正实现和 `@tanstack/react-virtual` 位于动态 `VirtualMenuImpl.tsx`。Composer、SlashMenu 与 FileReferenceMenu 均只静态依赖轻量包装器，避免任一入口把 TanStack 重新提升到首启图。
- 设置中心的完整基础样式从全局 `styles.css` 移入 `SettingsPanel.css`，由新的 lazy `SettingsPanelRoute.tsx` 与设置实现一起装载；组件测试可继续直接导入不含 CSS side effect 的 `SettingsPanel.tsx`，不需要为 Node 测试环境伪造 CSS loader。共享 responsive `.drawer--wide`、App BotDetail 规则、全局管理模态外壳，以及更晚、更高 specificity 的主题/Creation 覆盖继续留在 `styles.css`，避免异步 CSS 到达顺序改变级联；默认设置布局已在真实浏览器检查最终计算样式。
- production 预算脚本递归遍历 Vite manifest 的静态 `imports`，分别构建并去重 base initial、最坏本地化、browser mock 首次使用、VirtualMenu 首次使用和 Settings 首次打开图。`bridgeMock.ts`、`VirtualMenuImpl.tsx` 和 `SettingsPanelRoute.tsx` 三个延迟入口必须是精确命名的 `isDynamicEntry`，它们的直接 JS/CSS 不得泄漏到 initial graph；缺失路由或预算时失败关闭。
- source contracts 同时守卫 mock 只能动态导入、TanStack 只能由实现 chunk 引入、Settings route 拥有延迟 CSS 而实现组件不直接导入 CSS，以及两个 CSS 文件都进入语法和 z-index 检查。预算 fixture 覆盖传递依赖、设置 CSS、静态提升拒绝、缺失 locale 数量和路径逃逸。

## 结果与预算

```text
                                      bbdddde  7d07c89 production        变化
entry JS                              623,577 B      624,876 B      +1,299 B
base initial JS                       984,616 B      865,678 B    -118,938 B (-12.1%)
worst localized startup            1,100,036 B      981,098 B    -118,938 B (-10.8%)
initial CSS                           611,424 B      511,305 B    -100,119 B (-16.4%)
initial JS files                              6              5
browser mock first-use JS                    n/a      960,568 B
VirtualMenu first-use JS                     n/a      890,931 B
Settings first-open JS + CSS                 n/a    1,053,773 B + 611,477 B
largest JS                            704,186 B      704,186 B             0
```

entry 因 bundler 对共享模块重新分组增加 1,299 B，但去重后的完整 base initial 图减少 118,938 B；这才是用户首启实际不再加载的 JS。设置中心 CSS 延迟后，关闭态不再承担 100,119 B CSS；首次打开设置时仍加载完整样式，未把成本伪装成消失。

当前硬门槛：

```text
entry JS                         <=   640,000 B
base initial JS                  <=   900,000 B / 5 files
worst localized startup          <= 1,000,000 B / exactly 2 locale files
initial CSS                      <=   525,000 B
browser mock first-use JS        <=   975,000 B
VirtualMenu first-use JS         <=   905,000 B
Settings first-open JS           <= 1,075,000 B
Settings first-open CSS          <=   625,000 B
largest JS                       <=   725,000 B
```

这些门槛同时限制首启与延迟路径：后续不能通过把生产代码塞进 browser mock、把菜单依赖推迟到不可接受，或让设置 CSS 无界增长来换取 base 数字。

## 参考与取舍

- DeepSeek Reasonix 当前 Desktop 仍以较大的单体前端为主；本批只保留其 Wails binding 与 browser fallback 语义，不复制单体加载方式。
- Kimi Code 的 component-scoped CSS 和 dialog/界面按使用路径装载方式提供机制参考；Reames 仍按现有 React/Vite 组件边界实现，并额外保留真实 manifest 预算。
- MiMo Code 的 base/theme/component 样式所有权启发了全局外壳、主题覆盖和设置组件 CSS 的分层；没有把其全部组件样式再次全局导入。
- `F:\Reames-Lite` 没有可直接复用的 Vite/Wails route graph 预算实现，本批不声明其为代码来源。

## 本地证据

本文编写时已通过：

```text
corepack pnpm typecheck                                      PASS
corepack pnpm test:typecheck                                 PASS
corepack pnpm test:all                                       PASS
tsx src/__tests__/bridge-lazy-mock.test.ts                   PASS (5)
tsx src/__tests__/bridge-mock-approval-mode.test.ts          PASS (4)
tsx src/__tests__/bundle-contract.test.ts                    PASS (26)
node scripts/test-bundle-budget.mjs                          PASS (6)
node scripts/check-bundle-budget.mjs                         PASS
CSS syntax + z-index checks (styles + SettingsPanel)         PASS
tsx src/__tests__/typography-overflow-contract.test.ts       PASS (75)
corepack pnpm build                                          PASS
go build ./...                                               PASS
go vet ./...                                                 PASS
go test ./internal/...                                       PASS
desktop: go test .                                           PASS
python -m unittest discover -s scripts -p 'test_*.py' -v     PASS (79, 2 skipped)
node scripts/test_upstream_watch_issue.mjs                   PASS (3)
docs/public/release/deploy/tool contracts                    PASS
Wails v2.12.0 production Windows build                       PASS
```

localhost browser dev mock 能在没有 Wails runtime 时完成启动；设置中心首次打开后延迟 CSS 已生效，最终布局为 grid、内容区可滚动，关闭路径正常；VirtualMenu 首次真实打开、键盘选择和关闭均通过，console 没有 warning/error。该证据证明浏览器 fallback、设置默认布局和菜单延迟实现可用，没有替代完整 `test:all`、原生 Wails production 或安装 candidate。

先前完整 `test:all` 暴露组件测试直接解析 CSS/TSX 的失败；`SettingsPanelRoute` 已把 CSS side effect 从实现组件移到真实 lazy route，修复后的完整 `test:all`、`pnpm build` 及 root/Go/Desktop/docs 门禁均已通过。

最终 Windows production 可执行文件大小为 48,045,568 B，SHA-256 为 `0F5E5BB8BCC4F7605D387F84D69079F54192A2E67C1F288056BBFBC8B6A12CE3`。隔离 HOME 冷启动首次可见/响应为 1.015 秒、稳定响应为 2.015 秒，满足 8 秒预算；同 HOME warm relaunch 首次可见/响应为 0.516 秒、稳定响应为 1.516 秒，满足 6 秒预算。进程、临时目录、默认状态边界和最终清理均通过。

原生 UIA interaction 完成 19 次 loopback provider 请求，覆盖成功、invalid key、rate limit、stream interruption/retry、permission denial/write blocked、tool timeout、stop/recovery 与会话持久化；设置中心 lazy route 在 production Wails 中真实打开并关闭。最终 boundary changes 与 errors 为空，进程和临时状态清理成功。

commit `7d07c89` 已推送至 `main`。普通 CI run `29216174519` 的 8 个 jobs 全部成功，CodeQL run `29216174514` 的 Go、JavaScript/TypeScript、Actions 3 个分析全部成功。本批没有为纯远端证据再次 push，也未触发昂贵的三平台 Desktop candidate。

## 证据边界

递归 manifest 预算证明构建产物的字节归属，source/unit contracts 证明导入和异步竞态语义，localhost 交互证明 browser mock 与延迟样式可用；它们都不能证明三平台安装包、签名/notarization、真实公网 Provider、真实 IM 或云节点行为。Windows High Contrast 与屏幕阅读器原生抽查仍属于后续手动或外部证据，禁止用 CSS `forced-colors` 合同冒充用户系统设置下的原生验证。
