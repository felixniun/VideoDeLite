# VideoDelite

> GitHub: https://github.com/felixniun/VideoDeLite


简单、快速、本地优先的 Windows 视频压缩工具。

> 名称：**VideoDelite** · 平台：Windows 10 (Build 19041+) / Windows 11
> 技术：Vue 3 + TypeScript + Go + **Wails v2.15.0** + FFmpeg / FFprobe
> 前端风格：Apple 简洁圆角设计（pill 按钮、大圆角卡片、系统蓝、Light/Dark）

## 核心原则（已冻结）

1. **本地处理**：视频读取、分析、编码、验证 100% 在本机完成，不上传任何视频数据。
2. **输出必须验证**：FFmpeg 退出码 0 ≠ 成功，必须通过 FFprobe 输出验证才标记完成。
3. **授权与编码解耦**：FFmpeg/TaskManager 零依赖账号系统；授权变化只影响新任务，绝不杀死正在运行的任务。
4. **Simple 模式免登录**；Professional 模式需授权（V1 MVP 阶段账户系统未开放，预留状态机）。

## 目录结构

```
VideoDelite/
├── main.go / app.go          # Wails 入口与绑定层（薄外观）
├── cli.go                    # --cli 无头模式（开发/CI 验证）
├── internal/
│   ├── media/                # FFprobe 分析器 + MediaInfo 模型
│   ├── encoder/              # 冻结码率表 + 编码器探测/选择 + 命令构建
│   ├── task/                 # 任务队列（单并发）、暂停/取消、CPU 回退
│   ├── ffmpegx/              # FFmpeg 进程管理、进度解析、线程级暂停
│   ├── validation/           # 输出 FFprobe 验证（硬检查 + 建议）
│   ├── history/ settings/    # SQLite 本地历史与设置
│   ├── logging/              # 本地日志（7 天 / 500MB / 单任务 20MB 上限）
│   ├── auth/                 # 授权状态机骨架（Phase 10 接入点）
│   └── errs/                 # 错误分类（决定是否 CPU 回退）
├── frontend/                 # Vue 3 + Pinia + 自研 i18n（zh-CN / en）
├── tools/valmatrix/          # Phase 0 实测验证矩阵生成器
├── tools/genicon/            # 应用图标生成器
└── docs/TECHNICAL-VALIDATION.md  # 实测结果（来自真实编码运行）
```

## Simple 模式（V1 MVP）

- H.264 / H.265 × Low / Mid / High × MP4 / MKV
- 码率表按计划书 §17 冻结；H.265 = H.264 × 75%（0.5 Mbps 粒度，§18）
- 超过 4K 按像素比例调整并封顶；低于 720p 按比例且下限 1 Mbps（§20/§21）
- AAC 128 kbps（§23）；硬件编码优先、自动 CPU 回退一次（§33/§34）
- 输出保持源文件夹结构，命名 `原名_low/mid/high.mp4`（§57/§58）

## 开发

```bash
# 依赖：Go 1.27+、Node 20+、Wails v2.15.0（go install github.com/wailsapp/wails/v2/cmd/wails@v2.15.0）、FFmpeg/FFprobe 在 PATH
wails dev            # 开发模式（热重载）
wails build          # 产出 build/bin/VideoDelite.exe
go run ./tools/valmatrix   # 重新生成技术验证矩阵（真实编码实测）

# 无头验证（驱动与 GUI 完全相同的管线）
./build/bin/VideoDelite.exe --cli analyze <video>
./build/bin/VideoDelite.exe --cli encode <video> --codec h265 --quality high --container mkv
```

## 用户数据位置

```
%LOCALAPPDATA%/VideoDelite/
├── videodelite.db    # 历史 + 设置（SQLite/WAL）
├── Logs/             # 本地日志，永不自动上传
└── Temp/ Cache/ Config/
```

卸载/清除历史均不会删除用户视频文件。

## 当前状态

- ✅ Phase 0 技术验证：本机（RTX 3050）CPU 与 NVENC 全部 PASS（含 10-bit），QSV/AMF 无硬件预期失败，详见 docs/TECHNICAL-VALIDATION.md
- ✅ MVP 闭环：导入 → FFprobe 分析 → Simple 配置 → 硬件编码 → 实时进度 → 输出验证 → 历史落库
- ✅ 苹果风格 UI：Home / Tasks / History / Settings / About，中英双语，Light/Dark
- ⏳ Phase 7+：Professional 模式完整参数面板、账号授权服务端、Installer、代码签名
