# P5 受控 Theme Pack 设计审计

日期：2026-07-18

状态：仓库内实现与本地门槛通过；远端 CI、CodeQL 和三平台 Desktop candidate 待集中提交后核验

## 上游范围

本轮逐文件审查 DeepSeek Reasonix `7f00d2c260f2b7fff719ac8fccd50d4472cb1dcb` 的
`theme_pack.go`、`theme_store.go`、`theme_import.go`、`theme_image.go`、
`theme_contrast.go`、对应测试和 JSON Schema。该提交共改动 70 个文件、增加约 1.36 万行；
Reames 只迁移可独立证明的安全与交互机制，不整体移植实现。

## 采用与拒绝

| 上游机制 | Reames 决定 | 适配 |
|---|---|---|
| 不可执行 manifest 与 semantic token 白名单 | 采用 | 新建 `.reames-theme` schema v1；未知字段和 CSS 函数 fail closed |
| ZIP 根目录限定、路径穿越/symlink/大小限制 | 采用并加固 | 增加文件数、总展开量、压缩比、像素总量、大小写重复名和 Windows 设备名限制 |
| PNG/JPEG/WebP 内容嗅探 | 采用 | 扩展名、magic、解码格式、尺寸和 manifest SHA-256 必须一致 |
| 原子导入/替换 | 采用并加固 | immutable 内容寻址资产 + pack/state 原子文件 + durable transaction journal |
| 内容 digest URL | 采用并加固 | 使用完整 64 字符十六进制 SHA-256，不截短；响应 `immutable` 且 `nosniff` |
| `select != apply`、可撤销实时预览 | 采用 | Gallery 本地选择；Go 侧 preview lease 仅驻留内存；apply 才持久化 |
| crash/relaunch 回滚 | 采用 | preview 不写 state；启动只恢复最后一次原子提交的 applied id |
| Safe Mode Graphite | 采用 | Safe Mode 不枚举、不预览、不提供用户资产，强制 Graphite 与无背景 |
| base style 与 pack id 分离 | 采用 | `[desktop].theme_style` 只保存六个基础方向；state 只保存安装包 id |
| 对比度警告 | 采用 | 警告不阻断导入，但生产 CSS 的既有 305 项硬合同继续阻断回归 |
| Reasonix 官方主题、图片和品牌 | 拒绝 | 只使用 Reames 原创或许可证/来源/digest 可检查的资源 |
| Reasonix marketplace、endpoint、发布工作流 | 拒绝 | P5 完全离线；公开 registry 和签名运营不作为主题依赖 |
| Python 程序化图片生成链 | 拒绝 | 不引入第二工具链或运行时；生成方法只作为 provenance 文本 |
| manifest 中脚本、HTML、SVG、字体、远程 URL 资源 | 拒绝 | 包只允许 JSON 与最多两张本地 raster scene image |

## `.reames-theme` v1 契约

公开 schema 位于 `docs/theme-pack.schema.json`。核心字段为：

- `schemaVersion=1`、稳定小写 `id`、显示 `name`、包 `version`；
- `author`、`description`、SPDX/LicenseRef 风格 `license`；
- `provenance.kind=original|licensed`、必填 `source`、可选 HTTPS `sourceUrl` 与 `generatedWith`；
- `baseStyle` 只能是 Graphite/Aurora/Slate/Carbon/Nocturne/Amber；
- light/dark token 只能覆盖仓库声明的 semantic token；值只能是 `#RRGGBB` 或 `#RRGGBBAA`；
- recipe 只允许 compact/comfortable 与 square/soft/round；
- home/workspace scene 各自最多引用一个根目录 PNG/JPEG/WebP，并声明完整 SHA-256。

包内不得出现未知文件、子目录、绝对路径、父目录、symlink、硬编码远程资源或可执行内容。

## 限额

| 项目 | 上限 |
|---|---:|
| ZIP 文件 | 36 MiB |
| manifest | 1 MiB |
| scene image | 16 MiB/张 |
| ZIP entry | 3（manifest + 两张 scene） |
| 总展开量 | 33 MiB |
| 单边尺寸 | 8192 px |
| 单图像素 | 24,000,000 |
| 高压缩 entry 比率 | 200:1（超过 1 MiB 时拒绝） |

读取时同时使用 ZIP 元数据限额和 `LimitReader` 实际字节限额，不能依赖攻击者可控的 header。

## 存储与事务

用户级根目录为 `REAMES_AGENT_HOME/theme-packs`：

```text
theme-packs/
  packs/<id>.json
  assets/sha256/<full-digest>.<png|jpg|webp>
  trash/
  state.json
  transaction.json
```

资产先按 digest 写入 immutable store；同 digest 已存在时重新校验内容。pack record、state 和 journal
均通过同目录临时文件、fsync 与原子 replace 写入。导入/替换 journal 记录目标 package digest、旧 record、
旧 state 与目标 state；
删除 journal 记录旧 state 和 tombstone。启动恢复按 journal 确定性完成或回滚，不把半写 manifest 暴露给 UI。
孤立资产不影响正确性，并在成功事务或启动恢复后 best-effort GC。

替换 active 用户主题时必须同时推进新的 pack digest 与 `appliedPackageDigest`。故障发生在新 pack 发布前则
恢复旧 pack/旧 state；发生在新 pack 发布后则补齐目标 state 并前滚。测试矩阵覆盖 `after-assets`、
`after-prepare`、`after-pack-write`、`after-install-state`、`after-delete-state` 与
`after-delete-rename`。

## 状态与恢复

- `state.json` 只包含 schema version、`appliedThemeId`、`appliedPackageDigest` 和递增 revision；基础 style 仍在 Desktop config。
- `previewThemeId` 仅在 Store 进程内存中；Cancel、关闭 Gallery 或新预览会恢复 applied 主题。
- relaunch 后内存 preview 消失，因此不会把未确认预览升级成用户选择。
- 删除 active pack 会先原子清空 applied state，再 tombstone pack；恢复逻辑只允许“已删且已清空”或“旧包与旧 state 完整存在”。
- active pack 缺失、损坏或 digest 不匹配时回退到配置的基础 style；Safe Mode 无条件回退 Graphite。

## 官方 Reames 目录

`internal/themepack/assets` 中的 `reames-dawn-horizon.jpg` 与 `reames-night-workshop.jpg` 是本轮生成并
视觉审查的原创资产。官方 pack `reames-dawn`、`reames-workshop` 及其完整摘要在 Go 二进制内嵌，
与用户 Store 分区；用户包不能冒用 ID、替换或删除官方包。官方包可以预览、应用和重启恢复，但 Safe Mode
同样不展示或提供其资产。提示词、生成记录、尺寸、转换命令、字节数和 SHA-256 见
`internal/themepack/assets/README.md`，用户合同见 `docs/THEME_PACKS.md`。

当前只有 Desktop 使用 Theme Store，且 Desktop 已有单实例门禁，因此并发写边界是进程内 mutex + durable
journal。没有把这一结论外推为通用跨进程原子性：若 CLI/Serve 将来获得主题写入口，必须先加入跨进程 Store lock。

## 2026-07-18 本地验收证据

- Root：`go build ./...`、`go vet ./...`、`go test ./internal/... -count=1 -timeout 300s`、
  `go test -race ./internal/themepack` 通过；六个 `CGO_ENABLED=0` 目标
  linux/amd64、linux/arm64、darwin/amd64、darwin/arm64、windows/amd64、windows/arm64 均构建通过。
- Desktop Go：`go build ./...`、`go vet ./...`、`go test ./... -count=1 -timeout 300s` 和
  `go test -race . -run TestTheme` 通过。绑定测试覆盖两套官方包可见、preview/apply、官方包删除拒绝、
  active 官方包冷启动恢复、digest asset middleware 与 Safe Mode 抑制；所有 pack mutation Wails 绑定都在
  打开文件选择器或接触 Store 前由后端拒绝 Safe Mode，不能只依赖前端 disabled 状态。
- Frontend：最终定向 `test:theme-pack` 为 17/17，Appearance 交互合同为 4/4；`test:typecheck`、
  `typecheck`、`test:all` 和 `build` 通过。305 项主题对比度、CSS syntax、z-index 和既有前端回归保持通过。
- 真实 Chromium：Appearance 显示 6 套基础风格、`Reames Dawn`、`Reames Workshop` 和一个用户包；
  Dawn 预览实际投影 `data-theme-pack=reames-dawn`、Slate 与 `#e3a15f`，官方选择区无删除按钮；
  保存后保持 applied，用户包才出现删除按钮，关闭未提交用户预览恢复 Dawn，控制台 warning/error 为 0。
  该 smoke 发现并修复了“前端受控运行时只接受 user 包、官方包只选中但不投影 token”的回归，并加入官方包
  DOM 投影测试。
- Windows production Wails：`wails build -clean -platform windows/amd64` 生成 53,165,568 B 候选可执行文件，
  SHA-256 为 `C4666D13981E3EE850C9E3389A9D06EF4195E2A1F864D39FFB9607D90996814B`。结构化 native
  smoke 冷启动稳定响应 1.532 秒、同 home warm relaunch 1.515 秒，`boundary_changes=[]` 且无 error。
  隔离 home 也实际启动到无需 API key 的主工作台；强制 Safe Mode 实际进入 recovery-only 界面，明确显示
  不读取 API key、用户配置、Provider、MCP、插件、Hook 或 Agent runtime。Guard recovery smoke 同时通过
  config/credential 不读不改、精确 repair/undo、派生状态隔离和最终 clean，边界变化与错误均为 0。
- 当前 Computer Use 的 Windows Graphics Capture 对该 frameless Wails 窗口仍返回既有
  `SetIsBorderRequired 0x80004002`，因此本审计不虚构原生 Appearance 点击证据；UI 交互由真实 Chromium
  smoke 覆盖，Wails 桥接、重启和 Safe Mode 由 Desktop 绑定测试、真实 production 进程和结构化 native/recovery
  smoke 共同覆盖。
- 最终 bundle：entry 639,093 B；initial JS 879,895 B；localized initial 1,000,087 B；initial CSS
  524,736 B；browser mock 980,045 B；virtual menu 905,148 B；settings 1,059,157 B JS + 624,908 B CSS；
  largest JS 704,186 B。所有数值均低于既有硬预算，没有继续放宽门槛。

## 完成门槛

P5 只有在以下证据同时存在后关闭：恶意 ZIP/图像/路径矩阵、事务故障注入、preview/apply/relaunch
合同、Safe Mode 合同、Frontend 交互与 bundle budget、现有 305 项主题对比度门禁、Go race、六目标
CGO=0 构建、Desktop Go/前端全量、浏览器与三平台 candidate。真实签名、notarization、公开主题
registry 或 marketplace 保持 `external-blocked`，不使用用户服务器，也不恢复任何遥测上传。

主题启动恢复本身只在现有 initial graph 中增加一个动态入口触发器；Theme Pack runtime、CSS、Gallery 和
browser mock 数据均保持独立 lazy chunk。生产构建仍低于既有 640,000 B entry、900,000 B initial JS
和 525,000 B initial CSS 硬限额。为容纳新 Wails mock 方法和启动恢复触发器，仅将派生图预算按实测增量
各增加不超过 1,000 B：localized initial 1,001,000 B、browser mock 981,000 B、virtual menu
906,000 B；settings、largest asset、文件数和核心 initial 限额不放宽。
