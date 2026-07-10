# 外部 Agent 批次验收

> 日期：2026-07-11
> 分支：`agent/full-delivery-program`
> 结论：保留可验证机制，撤回伪覆盖和未接入实现；本地基线通过，远端 CI 待本批 push 后确认

## 验收范围

外部 Agent 在 `origin/main` 之上提交了 23 个提交，初始差异约为 36 个文件、4172 行新增。验收不以提交数量为完成证据，而是逐项检查生产调用链、测试是否命中生产代码、文档声明与真实证据是否一致。

## 保留并修正

- `internal/provider/harness`：localhost OpenAI SSE 失败夹具；补空/非法脚本拒绝、1 MiB 请求上限、请求正文与 `stream` 记录、防可变快照。
- `internal/control/ErrorInfo`：错误 code/category/message/retryable 穿过共享 `eventwire`，同时保留旧 `err` 字符串；修正宽泛字符串匹配和旧 UI 的 code 前缀污染。
- Goal sidecar v1：保留 Todo，拒绝未来/畸形版本，终态恢复测试使用真实 v1 writer，生产写入改用 `fileutil.AtomicWriteFile`。
- Provider 真实验证脚本：只从进程环境读取 key；不保存 HTTP 错误正文；响应限制为 1 MiB；blocked 使用独立退出码 2。
- Upstream deep report 与本地 issue draft：比较时显式获取 base/head，补真实本地 Git 仓库测试、草稿路径清洗和 CI 接入。
- Controller 取消、Provider 失败恢复和 plugin lifecycle 测试：改为通道同步并检查生产 API 错误，不依赖固定 sleep 或忽略返回值。

## 撤回

- 未接入 `package.json` 且自建常量/函数的前端、Bot、Evidence、cache “合同测试”。
- 未进入飞书生产接收路径、协议依据不足的 webhook HMAC 实现。
- 解析器和安装器均不读取/执行的 plugin trust、permission、compatibility、integrity 字段。
- 与现有发布/部署门禁重复的浅层 cross-compile、Docker、SBOM 脚本，以及与 `NOTICE.md` 冲突的根 `NOTICE`。
- 未被 Goal 状态机调用的 transition 表和“enforcement”测试。

## 文档校准

- M1 失败场景恢复为未完成：进程内和 reducer 自动化已具备，原生 Wails 失败提示/恢复 smoke 仍缺。
- M2 ErrorInfo 记录为部分完成：wire contract 已建立，command DTO 和前端 category 路由仍未收口。
- `THREAT_MODEL.md` 改为“已实现核心路径 / 部分实现 / 外部阻塞”矩阵，不再声称插件签名、飞书 webhook 签名、始终启用 sandbox 或完整 Serve 加固已经完成。
- `PROJECT.md`、`DEVELOPMENT_PLAN.md`、`DOCS_INDEX.md` 和接手交接页同步到当前事实。

## 本地证据

以下命令在本批工作树通过：

```text
go build ./...
go vet ./...
go test ./internal/... -count=1 -timeout 300s
go test -race（取消传播、Provider harness、plugin 并发子集）
desktop: go test . -count=1 -timeout 300s
desktop/frontend: corepack pnpm test:all && corepack pnpm build
python 文档、公开、部署、发布合同
python installer/artifact/native smoke 合同：47 项通过，2 项按平台跳过
python upstream/provider verifier/issue draft：20 项通过
node scripts/test_upstream_watch_issue.mjs：3 项通过
python scripts/check_upstreams.py：远端扫描成功，changed_count=9
Linux/macOS/Windows amd64+arm64：6 目标交叉编译通过
scripts/verify-baseline.ps1 -SkipFrontendHint：通过，含 headless Gateway smoke
```

前端 production build 仍报告既有的超大 chunk 和 ineffective dynamic import 警告；它们没有导致本批失败，但属于 M3 性能债务。

## 未关闭边界

- 没有新增真实 API key 调用；既有真实 Provider 证据仍以 `2026-07-09-real-provider.md` 为准。
- 没有真实飞书/QQ/微信应用回环或公网云节点验证。
- 没有 Windows/macOS 生产签名、notarization 或公开 updater 链。
- 没有补齐原生 Wails 的 401/429/断流/权限拒绝/工具超时可见性 smoke。

这些项目必须保持未完成或 `external-blocked`，不能由 localhost mock、单元测试或文档替代。
