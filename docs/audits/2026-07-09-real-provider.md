# 真实 Provider 最小验证（2026-07-09）

## 范围

使用用户本机保存的 DeepSeek 官方凭据验证 Reames Agent 的真实 Provider 主链路。验证过程不读取、不输出、不提交 API Key。

## 配置诊断

`reames-agent doctor --json` 确认：

- 默认 Provider：`deepseek-flash`
- 协议：OpenAI-compatible
- 主机：`api.deepseek.com`
- 模型：`deepseek-v4-flash`
- 凭据槽位：`DEEPSEEK_API_KEY`
- `key_present = true`

## 最小请求

```powershell
.\bin\reames-agent.exe run --model deepseek-flash --max-steps 1 "请只回复：API连接成功"
```

结果：

- 进程退出码：0
- 模型回复：`API连接成功`
- 输入 token：14,161
- 缓存命中 token：14,080
- 新输入 token：81
- 输出 token：30（其中 reasoning 26）
- 费用：约 ¥0.0004

## 结论

以下能力已由真实服务证明：

1. 全局凭据文件解析；
2. Provider/模型选择；
3. DeepSeek 官方鉴权；
4. 请求与响应链路；
5. 可见 reasoning/usage 统计；
6. 稳定前缀缓存命中。

这只是 M1 的最小连接证据，不代表完整任务闭环。仍需在原生 Desktop 中验证流式显示、停止、工具审批、文件变更、回退和重启恢复。
