# Reames Agent 项目说明

> 状态：当前产品方向的权威说明
>
> 更新：2026-07-13

## 一句话定位

Reames Agent 是一个以 DeepSeek Reasonix 为工程底座、面向本地与远程工作的多平台通用 Agent。编程助手是第一条成熟主线，长期能力覆盖研究、文件与数据处理、自动化、记忆、插件和多渠道协作。

## 产品形态

同一套 Agent runtime 支撑四个入口：

| 入口 | 定位 |
|---|---|
| Desktop | 主产品；完整会话、工作区、审批、变更、设置和恢复体验 |
| CLI | 终端中的高效编程与自动化入口 |
| Server/Web | 本机、局域网和云端的 HTTP/SSE 服务 |
| Gateway | 飞书、QQ、微信、Telegram 等消息渠道 |

入口只负责交互和传输。会话、Agent loop、模型、工具、权限和事件语义应逐步收敛到 `internal/control` 后面的统一运行边界。

云端部署形态见 [CLOUD_AGENT_PLAN.md](CLOUD_AGENT_PLAN.md)：目标是在自有服务器上同时支持 SSH/CLI、HTTP/SSE、飞书等 IM 通道、后台上游研究 Worker，以及隐私保护的遥测反馈闭环。

## 项目来源

- [DeepSeek Reasonix](https://github.com/esengine/DeepSeek-Reasonix) `main-v2` 是源码主上游。
- `F:\Reames-Lite` 是前身，只保留公共边界、缓存纪律、中文体验和接口契约等思想。
- `F:\code-reference` 中的其他官方项目提供机制和体验参考，不作为可直接拼接的代码集合。
- 详细来源、许可证和升级规则见 [REFERENCE_GOVERNANCE.md](REFERENCE_GOVERNANCE.md)。

## 核心原则

1. **单一运行语义**：Desktop、CLI、Server 和 Gateway 不各自实现 Agent 行为。
2. **缓存稳定**：system prompt 与 tool schema 保持稳定；UI、渠道和诊断状态不得污染模型前缀。
3. **副作用受控**：文件、命令、网络和凭据操作经过权限、沙箱、脱敏与证据记录。
4. **状态可恢复**：会话、检查点、任务和后台作业必须有清晰的持久化与恢复语义。
5. **桌面优先但不绑死桌面**：桌面是主产品，核心能力仍保持传输无关。
6. **上游人工决策**：自动发现、分类和建单；由维护者审查、移植和接受版本。
7. **证据先于完成声明**：构建、测试、真实交互或发布证据齐备后，事项才算完成。

## 当前事实

- Go Agent 内核、CLI、HTTP/SSE、IM Gateway 和 Wails/React Desktop 已有较完整实现。
- 核心、Desktop 和前端已建立本地与远端 CI 基线，并有六目标 CLI candidate、三平台 Desktop candidate 及原生安装 smoke 记录。
- M1 真实任务闭环已关闭：真实 Provider、原生会话/工作区/停止、文件审批/落盘/回退、重启恢复以及五类原生失败恢复均有分层证据。
- 24 个内置工具，具备权限、沙箱、检查点、记忆、技能、插件、定时任务、LSP 和证据账本等模块。
- M3 已进入 Desktop 日用化：关闭态/次级界面已按需拆包并受真实产物硬预算保护，入口 JS 较基线下降 43.6%；Windows 本地源码 production Wails 受 8 秒冷启动和 6 秒同 HOME warm relaunch 门槛保护，托管 runner 首次安装候选按独立证据使用 15/6 秒门槛，当前本地两轮稳定响应均实测 1.516 秒；Linux/macOS candidate 已在真实 runner 通过 10 秒隔离状态 readiness 门槛，Linux 还验证持续可见窗口，macOS 只声明状态就绪；模态焦点、键盘围栏与读屏语义已统一，Windows 显示缩放具备原子持久化、最后选择胜出、待重启提示和直接重启闭环；六套视觉风格的深浅色、普通/创作模式已有可重复对比度和焦点环合同，主题切换导致入口重挂载时仍能恢复焦点。
- 当前最大风险不是“缺功能”，而是 Desktop 日用化细节、插件供应链、远程入口加固和真实 IM 回环尚未完全闭环；统一 control 边界已关闭并有远端 CI/CodeQL 证据，transport 对 `agent/provider/tool` 的生产直连为零，版本化 command/event/display DTO、prompt metadata、会话持久化/复制、Desktop session-store、ACP/CLI 装配和终端渲染路径已收口。
- `site/`、`workers/` 等遗留产品面仍需按运行引用、发布依赖和替代实现逐批判断，不能一次性盲删。

代码与测试驱动的初始接管结论见 [audits/2026-07-09-takeover.md](audits/2026-07-09-takeover.md)。

## 近期产品标准

桌面端应让用户完成一个完整任务闭环：

```text
创建/恢复会话
→ 选择工作区和模型
→ 提交任务与附件
→ 查看推理、工具进度和文件变更
→ 批准或拒绝副作用
→ 停止、继续、回退或恢复
→ 获得带证据的结果
```

主界面使用中文优先、低噪音的桌面产品语言。工程术语、迁移阶段和上游名称不应进入普通用户主流程。

## 明确不做

- 不自动合并或自动升级上游源码。
- 不用旧 Reames Lite 的 Python provider/cache 覆盖 Reasonix 现有管线。
- 不把桌面端做成 CLI 日志壳或工程状态仪表盘。
- 不为追求“全能”而复制整套参考项目。
- 不在没有行为测试或真实验证时宣称功能已经完成。

## 文档治理

- 产品方向只更新本文。
- 优先级、里程碑和验收只更新 [DEVELOPMENT_PLAN.md](DEVELOPMENT_PLAN.md)。
- 架构事实更新 [ARCHITECTURE.md](ARCHITECTURE.md)。
- 上游来源和接受规则更新 [REFERENCE_GOVERNANCE.md](REFERENCE_GOVERNANCE.md)。
- 一次性调查、审计和验证记录放在 `docs/audits/`，不得冒充当前计划。
- 已被替代或无法反映当前代码的文档直接删除；Git 历史承担归档职责。
