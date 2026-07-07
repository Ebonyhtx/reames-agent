# 📦 代码融合参考 — Reames Agent (Full)

> **用途**: Reames-Lite 开发时的代码移植参考
> **来源**: https://github.com/Ebonyhtx/reames-agent
> **注意**: 这是完整版 REAMES，基于 Hermes fork，含大量历史代码。
>          Reames-Lite 只提取核心逻辑，不直接复制。

## 可移植模块

| 模块 | 文件 | 说明 |
|------|------|------|
| 记忆系统 | `agent/reames_memory.py` | L0-L3 四层记忆，可独立提取 |
| 缓存优化 | `agent/deepseek_cache.py` | `ensure_cache_stable` + `CacheStats` |
| 状态栏 | `agent/status_bar.py` | Reasonix 风格信息栏 |
| 压缩器 | `agent/context_compressor.py` | cache_first 模式 |
| 安装脚本 | `install.ps1` | Windows 一键安装参考 |

## 不要移植的

- `memory_tencentdb_core.py` — 死代码，TencentDB 遗留
- `plugins/` 大部分 — Hermes 插件体系，不在 Lite 架构中
- `gateway/` — 旧网关架构

克隆时间: 2026-06-17
