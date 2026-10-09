# VideoDelite 仓库结构导览

本文说明当前代码库的主要目录、客户端处理链路，以及常见改动的入口。构建产物、依赖缓存和用户数据不纳入目录图。

## 顶层结构

```text
VideoDelite/
├── app.go / main.go / cli.go       # Wails 应用装配、前端绑定、无头 CLI
├── platform_*.go / webview2_*.go  # 平台适配与 Windows WebView2 检查
├── internal/                       # 桌面客户端领域逻辑
├── frontend/                       # Vue 3 + TypeScript + Vite
├── server/                         # Gin 账号与授权 API、管理界面
├── cmd/videoserver/                # 账号服务启动入口和配置模板
├── tools/                          # 验证矩阵与图标工具
├── build/                          # 构建、安装包、签名和部署脚本
├── installer/                      # NSIS 安装程序
└── docs/                           # 产品需求、指南、运维和验证资料
```

## 客户端模块

| 路径 | 职责 |
|---|---|
| `internal/media/` | 调用 FFprobe，解析媒体流、时长、分辨率、位深、HDR、字幕和章节等信息。 |
| `internal/encoder/` | Simple 码率策略、硬件编码器探测，以及把统一配置转换成 FFmpeg 参数。 |
| `internal/task/` | 任务队列、状态流转、并发控制、暂停 / 取消 / 重试、失败回退和历史写入。 |
| `internal/ffmpegx/` | 管理 FFmpeg 子进程、解析进度并处理暂停 / 恢复。 |
| `internal/validation/` | 编码后再次调用 FFprobe；不满足硬性条件的输出不会标记为完成。 |
| `internal/history/`、`internal/settings/`、`internal/db/` | 使用本机 SQLite 保存历史、偏好和应用数据。 |
| `internal/auth/`、`internal/account/` | 授权门控、账号 API 客户端与本地授权状态。 |
| `internal/logging/`、`internal/paths/`、`internal/proc/`、`internal/errs/` | 日志、用户数据目录、子进程封装和错误分类。 |

## 前端与 Go 的连接

`frontend/src/services/wails.ts` 封装前端对 Go 绑定的调用；`app.go` 提供薄的应用门面，主要业务逻辑位于 `internal/`。

| 前端路径 | 职责 |
|---|---|
| `frontend/src/views/` | Home、Tasks、History、Settings、About 页面。 |
| `frontend/src/stores/` | Pinia 应用设置与任务状态。 |
| `frontend/src/i18n/` | 中文和英文界面文案。 |
| `frontend/src/types/` | 与 Go 绑定数据结构对应的 TypeScript 类型。 |

## 一条 Simple 任务的调用链

```text
HomeView
  → app.go: ScanPaths / AnalyzeFiles
  → internal/media: FFprobe 分析
  → internal/task: 入队与调度
  → internal/encoder: 选择编码器并构造参数
  → internal/ffmpegx: 运行 FFmpeg 并推送进度
  → internal/validation: FFprobe 检查输出
  → internal/history: 保存本地历史
```

账号服务端位于 `server/` 和 `cmd/videoserver/`，与本地媒体管线分开。授权状态只用于决定是否可创建 Professional 任务；Simple 模式无需登录。

## 常见改动入口

| 想调整 | 从这里开始 |
|---|---|
| Simple 档位和码率 | `internal/encoder/bitrate.go` |
| 编码器探测 / FFmpeg 参数 | `internal/encoder/hardware.go`、`internal/encoder/builder.go` |
| 任务状态与调度 | `internal/task/` |
| 输出成功判定 | `internal/validation/validation.go` |
| 页面、翻译、主题 | `frontend/src/views/`、`frontend/src/i18n/`、`frontend/src/style.css` |
| 账号 API 与管理台 | `server/handlers.go`、`server/adminui.html` |
| 默认设置与数据目录 | `internal/settings/`、`internal/paths/` |
| 安装与发行打包 | `installer/installer.nsi`、`build/package.ps1` |

## 文档目录

- `docs/product/`：产品需求、账号与本地处理边界、授权状态机和 MVP 交付记录。
- `docs/guides/`：开发学习指南与本文档。
- `docs/operations/`：服务端部署手册。
- `docs/reports/`：编码能力和媒体保留实测报告。
- `docs/PRIVACY-POLICY.md`、`docs/RELEASE-NOTES.md`：隐私政策和版本记录。

完整文档导航见 [docs/README.md](../README.md)。
