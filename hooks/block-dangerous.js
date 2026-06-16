#!/usr/bin/env node
// PreToolUse 钩子 — 阻止危险的 bash 命令
// exit 0 = 放行, exit 2 = 阻断

const payload = JSON.parse(require("fs").readFileSync(0, "utf8"));
const command = (payload.toolArgs && payload.toolArgs.command) || "";

// 危险命令模式列表（已优化，减少误判）
const dangerous = [
  { pattern: /\brm\s+[-]?rf?\b/i,       reason: "rm -rf 可能删除重要文件" },
  { pattern: /\brm\s+[-]?fr?\b/i,       reason: "rm -fr 可能删除重要文件" },
  { pattern: /\bdel\s+\/f\b/i,          reason: "强制删除文件" },
  { pattern: /\brmdir\s+\/s\b/i,        reason: "递归删除目录" },
  // format: 只匹配 "format C:" 或 "format /dev/xxx"（排除代码中的 format_xxx）
  { pattern: /\bformat(\s+[a-zA-Z]:|[\s]+\/dev)/i, reason: "格式化磁盘" },
  // shutdown: 只匹配带参数的 shutdown（排除代码中的 shutdown()）
  { pattern: /\bshutdown\s+(-|--|now|reboot|halt|\d)/i, reason: "关机命令" },
  { pattern: /\bgit\s+push\s+-f/i,      reason: "强制推送很危险" },
  { pattern: /\bgit\s+merge\b/i,        reason: "git merge 需要你手动确认" },
  { pattern: /\bgit\s+reset\s+HEAD/i,   reason: "git reset 可能丢失提交" },
  { pattern: /\bchmod\s+777\b/i,        reason: "chmod 777 权限过于开放" },
  { pattern: /\bchown\b/i,              reason: "修改文件所有者" },
  { pattern: /\bdd\s+if=/i,             reason: "dd 命令可能破坏数据" },
  // cp: 排除常见开发路径 /c/ /tmp/ /home/ /Users/ /mnt/
  { pattern: /\bcp\s+\/(?!c\/|tmp\/|home\/|Users\/|mnt\/)[a-z]/i, reason: "复制根目录文件危险" },
  { pattern: /\bsudo\b/i,               reason: "sudo 需要你手动确认" },
  { pattern: /\bcurl\s+.*\||\|\s*curl/i, reason: "管道到 curl 可能不安全" },
  { pattern: /\bbash\s+[<-]/i,          reason: "远程加载脚本执行" },
];

for (const { pattern, reason } of dangerous) {
  if (pattern.test(command)) {
    console.error(`\u26D4 已阻止危险命令: ${reason}`);
    console.error(`命令: ${command}`);
    process.exit(2);
  }
}

// 放行
process.exit(0);
