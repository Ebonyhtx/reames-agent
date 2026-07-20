# Hermes / MiMo 最终移动增量审计

日期：2026-07-20

## 范围与层级

本轮提交前扫描发现两个三级机制参考移动：

- Hermes：`dd418284db1804d33cd3d6d51c17bbfb1ad8f685..a7d7c02cb6db071eced4ac82e24f878588619600`，
  14 个非 merge 提交、64 个变化文件、约 `+2858/-283`；
- MiMo Code：`f24ce4eb7341bfba6bb608436c1d27a843508adf..ec413adeccfcb65ccb63a708bb6136644ea13c79`，
  主体为 `4f44dab00ac2eeae377a0c4000a1cc0987eb3fb6` 的 `learn-everything` Skill bundle。

Reasonix、Codex 与 Claude Code 本轮没有新提交。Hermes 和 MiMo 仍是三级机制参考，不承担协议或版本
parity；本审计只判断可验证机制是否适合 Reames 的单一 Go/Wails runtime。

## Hermes 结论

| 增量 | 机制 | Reames 决策 |
|---|---|---|
| `3d978935` custom endpoint settings | Desktop CRUD、连通性验证、模型发现和激活 | 已有更强等价。Reames Desktop 已支持自定义 Provider 编辑/删除、显式 `api_mode`、模型与 context window、`FetchProviderModels`、凭据 env 写入和只暴露 key 状态；不复制 Python web server/Electron 配置面，也不让读取 DTO 返回明文 key |
| `54459e76` model picker probe | 只探测当前 custom provider，并缓存发现模型 | 已有等价边界。Reames 只在用户显式刷新或保存 key 后探测目标 Provider，不在打开模型选择器时批量探测其他 endpoint；持久模型清单仍经显式设置保存事务，不采用 best-effort 静默改写配置 |
| `33d71d68` new-chat selector | 异步 profile 刷新不能覆盖用户刚选的 model/provider/effort/fast；发送时快照选择 | 进入 Desktop/P9 回归合同。当前未发现 Reames 的确定性复现，不因三级参考直接改写会话状态机；后续原生交互测试应覆盖“选择后立即发送”和慢设置刷新竞态 |
| `2f6a4e09` standard DSR | 识别无 `?` 的 `CSI row;col R`，同时保留 row 1 modified-F3 | 依赖层信号。Reames CLI 使用 Bubble Tea/终端依赖而非 Hermes Ink parser；只有出现同构输入失败或依赖测试缺口时才在上游/适配层修复 |
| `b1fb3c52..81423316` Desktop perf | production renderer、cold start、first token、唯一调试端口、连接门禁、组成指标、多次中位数与 warm-cache 口径 | 采用为 P9 原生性能证据方法。必须分别记录进程 spawn→CDP/driver/FCP、backend connected→first token、dev/prod 与冷/暖缓存；当前 Go emitter 微基准仍不能冒充 WebView frame pacing 或冷启动 |
| `bc6839aa` non-git package fallback | 无 Git 时写全零 commit，bootstrap 改为未固定 branch | 拒绝。Reames candidate/release 的 provenance、源码 revision 与安装事务必须可验证；不能为了 ZIP tree 构建成功把未知 revision 降级成可联网跟随的分支 |
| `07ba9e92` / `1cf2c763` dashboard | 搜索时保留当前 provider 的模型、modal 遮罩隔离 | 仅作为选择器和 overlay UX 回归信号，不复制 Hermes Dashboard runtime |
| `9b428ddd` / docs / formatter | Grok 搜索默认模型、文档链接和格式化 | 产品/仓库非同构，无 Reames 代码动作 |

## MiMo 结论

`learn-everything` 将长文档/主题教学组织为课程图、逐章 lesson loop、练习反馈、concept mastery、review
queue 与 error log；有文件工具时在章节边界写 `course-state.md`，无文件工具时输出可携带的 state block。

该提交只作为 Skill 机制候选：

- 可借鉴“Skill 声明能力、按阶段 checkpoint、恢复时先做短回顾、状态文件与对话便携块同构”的合同；
- Reames P9 必须先完成通用 Skill/Plugin 发现、权限、版本、撤销、会话投影和持久状态归属，不能先把一个
  教学 prompt bundle 当作 Skill runtime parity；
- 不复制 MiMo 的完整教学文本、目录或第二套 OpenCode runtime。未来若进入通用学习产品线，应以 Reames
  原生 Skill schema、memory/evidence 和用户授权状态目录重新实现并单独测试。

## 冻结结果

本轮没有需要进入当前 M6/P9 生产代码的紧急修复。Hermes 的 cold-start/first-token 方法和 selector race
进入路线图合同；MiMo 仅形成 checkpointed Skill state 候选；未知 revision fallback 明确拒绝。两个完整 SHA
通过绑定 `--accept-revision` 写入锁文件，本地镜像只允许 `--ff-only` 快进。
