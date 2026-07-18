# 受控主题包

Reames Agent Desktop 支持离线 `.reames-theme` 主题包。主题包只能覆盖经过白名单约束的语义颜色、密度、圆角和最多两张本地栅格场景图；不能携带脚本、HTML、SVG、字体、任意 CSS、远程 URL 或其他可执行内容。

公开机器契约见 [theme-pack.schema.json](theme-pack.schema.json)，设计与上游取舍见 [P5 受控 Theme Pack 审计](audits/2026-07-18-p5-controlled-theme-pack-design.md)。

## 使用

1. 打开 Desktop 设置 → Appearance。
2. 选择内置基础风格、Reames 官方主题或已导入的用户主题。
3. 单击主题卡片只会开始临时预览；单击 Save 才会持久应用。
4. Cancel、关闭 Appearance、崩溃或重新启动都会撤销未保存预览，回到最后一次已提交主题。
5. Add 通过原生文件选择器导入 `.reames-theme`。同 ID 替换必须再次确认，确认令牌仅在当前进程短时有效且只能使用一次。
6. 只有用户主题可以删除。内置基础风格和 Reames 官方主题是只读的。

Safe Mode 不枚举用户主题、不恢复主题包、不提供任何主题图片，并始终使用 Graphite。退出 Safe Mode 后，正常模式仍会恢复最后一次已提交主题。

## 包结构

ZIP 根目录最多包含 `theme.json` 和两张场景图：

```text
theme.json
home.jpg
workspace.webp
```

扩展名必须是 `.reames-theme`。文件不能位于子目录，名称不能包含 `/`、`\`、驱动器前缀、父目录、Windows 设备名、大小写折叠重复项或符号链接。

最小的纯颜色主题示例：

```json
{
  "schemaVersion": 1,
  "id": "example-calm",
  "name": "Example Calm",
  "version": "1.0.0",
  "author": "Example Author",
  "description": "A local controlled theme",
  "license": "MIT",
  "provenance": {
    "kind": "original",
    "source": "Created by Example Author"
  },
  "baseStyle": "graphite",
  "tokens": {
    "light": {
      "bg": "#f6f7f8",
      "fg": "#1f2933",
      "accent": "#315f8c",
      "accentFg": "#ffffff"
    },
    "dark": {
      "bg": "#0c0d10",
      "fg": "#e9edf1",
      "accent": "#88baf0",
      "accentFg": "#07111c"
    }
  },
  "recipes": {
    "density": "comfortable",
    "corners": "soft"
  }
}
```

`id` 必须使用小写 ASCII、数字和连字符，不能冒用六个基础风格 ID、`reames-dawn`、`reames-workshop` 或 Windows 设备名。未知 JSON 字段会被拒绝。

## 场景图

场景图只允许 PNG、JPEG 或 WebP。manifest 必须声明文件的完整小写 SHA-256：

```json
{
  "scenes": {
    "home": {
      "image": {
        "file": "home.jpg",
        "sha256": "完整的 64 位十六进制 SHA-256"
      },
      "focusX": 0.5,
      "focusY": 0.5,
      "safeArea": "center",
      "opacity": 0.35,
      "overlayStrength": 0.6
    }
  }
}
```

导入时会同时检查扩展名、magic、实际解码格式、单边尺寸、总像素和摘要。运行时只通过本地完整摘要 URL `__reames_agent_theme_asset/<sha256>` 提供，并带 `nosniff` 与 immutable cache header；前端在写 CSS 前再次验证颜色和 URL 白名单。

## 硬限制

| 项目 | 上限 |
|---|---:|
| 归档 | 36 MiB |
| manifest | 1 MiB |
| 单张图片 | 16 MiB |
| ZIP entry | 3 |
| 总展开量 | 33 MiB |
| 图片单边 | 8192 px |
| 单图像素 | 24,000,000 |
| 大于 1 MiB entry 的压缩比 | 200:1 |

导入还会拒绝加密 ZIP、未知文件、重复文件、路径穿越、symlink、摘要不一致、像素炸弹和异常压缩元数据。

## 存储与恢复

用户主题位于 `REAMES_AGENT_HOME/theme-packs`：

```text
theme-packs/
  packs/<id>.json
  assets/sha256/<full-digest>.<ext>
  trash/
  state.json
  transaction.json
```

资源按内容寻址且每次读取都重新校验。pack、state 和 transaction 使用同目录临时文件、原子替换、文件同步与父目录同步。替换正在使用的主题时，pack 摘要和 active state 会作为同一可恢复事务推进；中断发生在新 pack 发布前则回滚旧版本，发生在发布后则前滚新版本。删除始终通过 tombstone 日志前滚。

当前只有受单实例保护的 Desktop 进程操作主题 Store，因此进程内互斥和 durable journal 是当前并发边界。若未来允许 CLI、Serve 或多个进程同时修改主题，必须先增加跨进程 Store lock，不能直接复用当前写路径。

## 官方 Reames 主题

官方主题直接嵌入 Go/Wails 二进制，不写入用户 Store，也不依赖服务器、registry 或 marketplace：

| ID | 场景 | 文件 | SHA-256 | 许可证 |
|---|---|---|---|---|
| `reames-dawn` | Home | `reames-dawn-horizon.jpg` | `91f060ed4e34cb5511a490187b9cfa1cd0dd7a255a1af542452cd25be1a8b899` | MIT |
| `reames-workshop` | Workspace | `reames-night-workshop.jpg` | `740439c941d931a9d2b064aef280f461f68e06c8df9ac5991c7b0f19d88ee6bf` | MIT |

两张图片均为 2026-07-18 为 Reames Agent 生成的原创资产，没有使用 Reasonix 的品牌、图片或 marketplace 内容。完整提示词、生成记录 ID、原始尺寸和转换方法见 [`internal/themepack/assets/README.md`](../internal/themepack/assets/README.md)。

## 许可证与来源

用户包必须声明 `license` 和 `provenance`。`provenance.kind` 只能是 `original` 或 `licensed`；外部来源 URL 只能使用无凭据的绝对 HTTPS URL。Reames Agent 验证字段和资源完整性，但不会替主题作者判断其许可证声明是否真实，导入前仍应核对来源和使用权。
