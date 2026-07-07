# Reames Agent — 产品路线图

> 创建：2026-07-08
> 基座：DeepSeek Reasonix main-v2 (Go 1.25, Wails v2, React 19, MIT)

## 路线图总览

```
Phase 1: Fork + Rebrand + CI           ✅ 完成 (2026-07-08)
Phase 2: 云服务器部署能力               ⏳ 待执行
Phase 3: IM 通道 Gateway                ⏳ 待执行
Phase 4: 品牌与视觉系统重塑              ⏳ 待执行
Phase 5: 旧 Reames Lite 合约迁移        ⏳ 待执行
Phase 6: 全面验证与收口                  ⏳ 待执行
```

## 产品形态

| 形态 | 技术 | 目标 |
|---|---|---|
| CLI | Bubble Tea (Go) | 终端原生 AI 编程助手 |
| Desktop | Wails v2 + React 19 | 桌面 Agent 应用 |
| Web/Cloud | HTTP/SSE serve (Go) | Docker 部署，浏览器访问 |
| IM Gateway | Bot 平台适配器 | 飞书/微信/QQ 等 IM 交互 |

## 核心原则

1. **单二进制**：CGO_ENABLED=0，6 平台交叉编译
2. **缓存优先**：DeepSeek prefix cache 全链路稳定
3. **传输无关**：control.Controller 驱动 CLI/serve/Desktop 三个前端
4. **安全边界**：IM 入站需身份+信任等级，出站需审批
5. **中文优先**：zh-CN 是一等公民，en 是二等回退
6. **AI 不自言**：UI 中不出现代码/调试/基线/协议等工程术语
