# VideoDelite MVP 交付计划书

> **文档性质：** MVP 交付基准 / 验收清单（最终版）
> **产品名称：** VideoDelite
> **交付版本：** v1.0.0-mvp-r3（新增：Debian Docker 生产部署、HTTPS 域名接入、QA 修复全记录）
> **交付日期：** 2026-10-04
> **学习指南：** 配套《docs/学习指南-从零到上线.md》（小白向完整复盘）

---

## 1. 系统架构总览（最终拓扑）

```
              外网用户（家庭电脑）
                    │ https://videodelite1.898280.xyz:28443
                    ▼
         ┌─────────────────────┐
         │  Cloudflare DNS     │  灰云（仅 DNS）
         │  A → <SERVER_PUBLIC_IP>  │
         └─────────┬───────────┘
                   ▼  路由器端口转发 28443→192.168.100.101:28443
         ┌──────────────────────────────────────┐
         │  Debian 服务器  192.168.100.101      │
         │  ┌────────────────────────────────┐  │
         │  │ nginx-proxy (Docker)           │  │  28443→443 SSL
         │  │ Let's Encrypt 双域名证书        │  │  acme.sh DNS-01 自动续期
         │  └───────────────┬────────────────┘  │
         │                  ▼                   │
         │  ┌────────────────────────────────┐  │
         │  │ videodelite-server (Docker)    │  │  8800 (HTTP, 仅内部)
         │  │ Go 账号/授权服务 + 管理台       │  │
         │  └───────────────┬────────────────┘  │
         │                  ▼                   │
         │  ┌────────────────────────────────┐  │
         │  │ SQL Server 2022 (Docker)       │  │  内部 1433
         │  └────────────────────────────────┘  │
         └──────────────────────────────────────┘
                    ▲
    内网用户（hosts 指向 192.168.100.101，绕开 ISP 封端口与 hairpin）
```

**域名规划：**

| 域名 | 用途 | 入口 |
|---|---|---|
| `videodelite1.898280.xyz` | 桌面客户端 API | `https://…:28443`（客户端内置默认） |
| `videodelite.898280.xyz` | 管理后台 | `https://…:28443/admin` |
| `mc.898280.xyz` | Minecraft（自用） | :61111 |

**内网 hosts（每台内网电脑一次）：** `192.168.100.101 videodelite1.898280.xyz videodelite.898280.xyz`

---

## 2. MVP 交付范围（主计划书 §119 冻结清单）

| # | MVP 要求 | 状态 | 证据 |
|---|---|---|---|
| 1 | Wails 2.15.0 + Vue 3 + Go 工程 | ✅ | `wails build` → `build/bin/VideoDelite.exe` |
| 2 | FFmpeg / FFprobe 集成 | ✅ | 自动解析（bundled bin/ → PATH） |
| 3 | 导入（文件/文件夹/递归/拖放） | ✅ | Home 拖放 + 选择器 + `ScanPaths` |
| 4 | Media Info（FFprobe 分析） | ✅ | 编码/分辨率/FPS/位深/HDR/音轨/字幕/章节/旋转 |
| 5 | Simple Mode（H.264/H.265 × 低/中/高 × MP4/MKV） | ✅ | 冻结码率表实测（720p30 High → 4500k） |
| 6 | 硬件编码 + CPU 回退 | ✅ | NVENC 11.9–16.4x；编码器错误单次回退 |
| 7 | AAC | ✅ | Simple 128k；Pro 64–512k / VBR Q1–5 |
| 8 | MP4 / MKV | ✅ | faststart + mov_text / matroska |
| 9 | Task Queue + 进度 + 暂停/恢复/取消/重试 | ✅ | 线程级暂停、实时进度事件 |
| 10 | 输出验证（FFprobe 强制） | ✅ | 硬检查不过即 Failed |
| 11 | History（本地 SQLite） | ✅ | 重启保留 |
| 12 | Professional Mode（CBR/VBR/CQ） | ✅ | CBR 10Mbps 实测 10.31Mbps |
| 13 | 账号/授权系统 | ✅ | Debian Docker 服务端全流程 PASS |
| 14 | 邮箱验证（Resend 真实发信） | ✅ | 域名发信 + 60s 限流 + 自动续期证书 |

**需求变更记录（经产品所有者批准）：**

| 原冻结项 | 变更 | 日期 |
|---|---|---|
| §35 任务并发默认 1 | 简单模式**保持冻结逐个**；专业模式新增 逐个/×2/×3 选项（默认仍 1） | 2026-10-04 |

---

## 3. 架构红线执行情况（主计划书 §15/§29 · 边界规范 §27–§31）

| 红线 | 执行 |
|---|---|
| FFmpeg 模块不依赖 Account Service | ✅ `internal/task`、`internal/ffmpegx` 零账号依赖 |
| 授权变化不杀运行中 FFmpeg | ✅ 状态机只门控新任务（Enqueue 检查） |
| 视频数据不上传 | ✅ 服务端只收 Installation ID/邮箱/密码哈希/版本号 |
| 输出必须验证，exit 0 ≠ 成功 | ✅ validation 包硬检查 + 警告分级 |
| Simple 免登录 | ✅ UNAUTHENTICATED 下全部可用 |
| 不误删用户数据 | ✅ 清历史/退出/删账号/卸载均不动视频 |
| 本地日志不上传 | ✅ 7 天/500MB/20MB 上限，导出脱敏 |

---

## 4. 实测记录

### 4.1 Phase 0 技术验证矩阵（RTX 3050 · FFmpeg 9.0.2）

| Feature | CPU | NVENC | QSV | AMF |
|---|---|---|---|---|
| H.264 MP4+MKV | PASS | PASS | FAIL（无硬件） | FAIL |
| H.265 MP4+MKV | PASS | PASS | FAIL | FAIL |
| 10-bit HEVC | PASS | PASS | FAIL | FAIL |
| HDR10 | ✅（§4.3 修复后） | ✅（修复后） | FAIL | FAIL |
| AAC 128k | PASS | PASS | PASS | PASS |

### 4.2 Professional Demo（§134）

| Demo | 配置 | 结果 |
|---|---|---|
| CBR | H.265 NVENC CBR 10Mbps | 10.4MB/8s，实测 **10.31 Mbps** ✓ |
| VBR | 8/12 Mbps | 8.2MB ✓ |
| CQ | Q26 + MKV | 3.5MB ✓ |

### 4.3 媒体保留 QA（§135，tools/qamatrix 实测）

**10/10 PASS**（HDR10 色彩标记、10-bit、双音轨+语言、字幕、章节、旋转）。报告：`docs/QA-PRESERVATION.md`

### 4.4 稳定性 QA（§115–§116）

- 连续 10 任务（混合编码/画质）：10/10，19s，零残留进程
- 强杀主进程 → ffmpeg 孤儿 → **Job Object 修复**（KILL_ON_JOB_CLOSE）
- 重启孤儿清理（§38）→ 启动扫描 `.videodelite-temp` 自动删除

### 4.5 账号授权链路（SQL Server + Resend 真实发信）

register（真实邮件）→ verify → login（JWT+Refresh 轮换）→ device activate → license active → admin 审计完整

---

## 5. 交付物清单

```
VideoDelite/
├── build/bin/VideoDelite.exe                # 客户端（已签名）
├── build/dist/VideoDelite-1.0.0-setup.exe   # Windows 安装包（NSIS，已签名）
├── build/dist/VideoDelite-Server-deploy.zip # 服务端 Windows 部署包（备用）
├── build/package.ps1 / sign.ps1 / deploy-server.ps1
├── build/nginx/                             # 服务器 nginx 配置（仓库留档）
├── cmd/videoserver/                         # 服务端入口 + config
├── server/                                  # 服务端实现（含 adminui.html 管理台）
├── internal/                                # 客户端领域模块
├── frontend/                                # Vue3 前端（苹果风格）
├── Dockerfile / docker-compose.yml          # 服务端 Linux 部署
├── installer/                               # NSIS 脚本 + 第三方声明
├── tools/valmatrix, qamatrix, genicon       # 实测/图标工具
└── docs/                                    # 全部文档（含学习指南）
```

**服务器侧**（Debian `/home/debian/`）：
- `videodelite/`：docker compose（server + SQL Server）
- `nginx-proxy/`：反代容器（28443→443）+ conf.d + certs（acme.sh 自动续期写入）

---

## 5.1 全项目文件图鉴（107 个文件逐项说明）

### 总目录树（全景）

```
VideoDelite/
│
├── 🖥️ 桌面客户端入口（根目录 Go 文件）
│   ├── main.go                     # 入口：WebView2 检测 → 初始化 App → Wails 窗口
│   ├── app.go                      # 绑定层（前端可调用的全部方法）
│   ├── cli.go                      # --cli 无头模式（analyze/encode 自动化验证）
│   ├── platform_windows.go         # 【Win】系统语言/主题读取
│   ├── platform_other.go           # 【非Win】空实现（跨平台编译）
│   ├── webview2_windows.go         # 【Win】WebView2 注册表检测 + 引导弹窗
│   ├── webview2_other.go           # 【非Win】空实现
│   ├── webviewdata.go              # WebView2 缓存重定向到 Data\WebView2
│   └── go.mod / go.sum / wails.json
│
├── 📦 internal/ —— 客户端核心（14 包 36 文件，外部不可引用）
│   ├── media/        analyzer.go + mediainfo.go          # FFprobe 分析 + 模型
│   ├── encoder/      bitrate.go + hardware.go + builder.go   # 冻结码率表 / 探测选择 / 命令构建
│   ├── task/         task.go + manager.go + helpers.go + disk_*.go   # 队列调度 / 磁盘检测
│   ├── ffmpegx/      runner.go + suspend_*.go + job_*.go      # 进程管理 / 线程暂停 / 崩溃绑命
│   ├── validation/   validation.go                     # 输出复检（exit 0 ≠ 成功）
│   ├── history/      history.go                        # SQLite 历史持久化
│   ├── settings/     settings.go                       # 设置 + API 地址迁移链 + 并行数
│   ├── logging/      logging.go                        # 7天/500MB/20MB 日志红线
│   ├── auth/         auth.go                           # 授权状态机（8 态）
│   ├── account/      account.go + helpers.go + credentials_*.go  # 账号客户端 + 凭据管理器
│   ├── db/           db.go                             # SQLite WAL + 迁移
│   ├── paths/        paths.go                          # 数据目录（安装目录优先）
│   ├── proc/         proc.go + hide_windows.go         # 子进程封装 + 防黑框
│   └── errs/         errs.go                           # 12 类错误码 + 回退判定
│
├── 🌐 frontend/ —— Vue3 界面
│   ├── index.html / package.json / vite.config.ts / tsconfig.json
│   ├── public/appicon.png
│   └── src/
│       ├── main.ts / App.vue / style.css        # 引导 / 骨架(自绘标题栏) / 设计系统
│       ├── router/index.ts                      # 5 页路由
│       ├── i18n/index.ts                        # 中英双语词典
│       ├── services/wails.ts                    # Go 桥接层
│       ├── stores/    app.ts + tasks.ts         # 设置状态 / 任务+实时进度
│       ├── types/index.ts                       # Go 结构体 TS 镜像
│       └── views/     Home / Tasks / History / Settings / About (.vue × 5)
│
├── 🗄️ server/ —— 账号服务端（Gin + Viper，标准库风格业务）
│   ├── handlers.go        # Gin 路由 + ginAuthed/ginAdmin 中间件 + UpdateRuntime 热更新
│   ├── store.go           # Store 接口 + AdminStats（双数据库开关）
│   ├── store_impl.go      # SQL 实现（MSSQL/SQLite 方言适配）
│   ├── placeholders.go    # ? → @pN 方言重写层
│   ├── auth.go            # Argon2id + JWT + Bearer
│   ├── mailer.go / resend.go   # SMTP 备选 / Resend 主通道
│   ├── adminui.go + adminui.html   # 管理台（统计卡+SVG图表，自包含）
│   └── server_test.go + mailer_test.go   # 集成测试（全生命周期）
│
├── 🚀 cmd/videoserver/ —— 服务端入口
│   ├── main.go            # Viper 装配 + 热更新监听 + 带超时的 http.Server
│   ├── config.go          # Viper：config.json + 环境变量 + 轮询热更新
│   ├── config.example.json # 模板（可提交）
│   └── config.json        # 【本地 gitignore】真实运行配置
│
├── 🐳 Linux 部署
│   ├── Dockerfile                    # 两阶段构建（GOPROXY 适配）
│   ├── docker-compose.yml            # server + SQL Server（凭据占位符）
│   └── .dockerignore
│
├── 📦 build/ —— 构建产物与脚本
│   ├── package.ps1 / sign.ps1 / deploy-server.ps1 / deploy-admin.sh   # 四个自动化脚本
│   ├── appicon.png / windows/(icon.ico, manifest, info.json)          # 图标与版本资源
│   ├── nginx/ (videodelite1.conf, videodelite.conf, compose)          # 反代配置留档
│   ├── keys/test-signing.pfx                          # 【本地 gitignore】测试证书
│   ├── bin/VideoDelite.exe + Data\                    # 【本地】绿色版 + 运行数据
│   ├── dist/…setup.exe / Server-deploy.zip / videoserver.exe   # 【本地】安装包/部署包
│   └── cache/ffmpeg-essentials + deploy-*.{json,yml}  # 【本地】ffmpeg 缓存 + 真实部署配置
│
├── 🧰 installer/ —— Windows 安装包
│   ├── installer.nsi                  # NSIS：双语向导/升级/卸载保留数据/Data 授权
│   └── THIRD-PARTY-NOTICES.txt        # FFmpeg GPL 来源声明
│
├── 🧪 tools/ —— 实测工具
│   ├── valmatrix/main.go              # Phase0 编码能力矩阵（真实编码）
│   ├── qamatrix/main.go               # Phase8 媒体保留矩阵（真实管线）
│   └── genicon/main.go                # 图标生成器
│
├── 📚 docs/
│   ├── 规划/  VideoLite计划书 + 授权状态机规范 + 边界规范 + MVP交付计划书（本文档）
│   ├── 学习指南-从零到上线.md              # 20 个真实坑复盘（小白向）
│   ├── DEPLOY-DEBIAN.md                   # 服务器部署手册
│   ├── TECHNICAL-VALIDATION.md / QA-PRESERVATION.md   # 实测报告 × 2
│   └── RELEASE-NOTES.md / PRIVACY-POLICY.md
│
├── 🐧 debian/                       # 【本地 gitignore】服务器部署副本（含真实凭据）
└── ⚙️ .gitignore / .dockerignore / .zcodeignore
```

> 图例：🖥️ 客户端 · 📦 客户端核心 · 🌐 界面 · 🗄️ 服务端 · 🚀 服务端入口 · 🐳 Linux 部署 · 📦 构建 · 🧰 安装包 · 🧪 工具 · 📚 文档
> 【本地 gitignore】= 在您电脑上存在（产物/凭据），GitHub 仓库不含。

### 🖥️ 桌面客户端（根目录 Go 文件 + internal/）

| 文件 | 职责 |
|---|---|
| `main.go` | 程序入口：启动前检测 WebView2 → 初始化 App → 拉起 Wails 窗口 |
| `app.go` | 绑定层（前端可调的全部方法）：分析/建任务/历史/设置/账号/日志导出 |
| `cli.go` | `--cli` 无头模式：analyze/encode 命令（CI 与自动化测试用） |
| `platform_windows.go` | 【仅Windows】读系统语言、系统深浅色主题 |
| `platform_other.go` | 【非Windows】上面俩功能的空实现（保证跨平台可编译） |
| `webview2_windows.go` | 【仅Windows】启动前注册表检测 WebView2，缺失弹窗引导官网（禁止静默装） |
| `webview2_other.go` | 【非Windows】WebView2 检测空实现 |
| `webviewdata.go` | WebView2 缓存重定向到 `Data\WebView2`（不散落 Roaming） |
| `go.mod` / `go.sum` | Go 模块定义与依赖锁（模块名 videodelite） |
| `wails.json` | Wails 构建配置（产物名、前端构建命令、版本信息） |

### 📦 internal/ —— 客户端核心逻辑（14 个包，Go 惯例：外部不可引用）

| 包 / 文件 | 职责 |
|---|---|
| **media/** `mediainfo.go` | MediaInfo 数据模型 + ffprobe JSON 解析（编码/分辨率/HDR/音轨/字幕/章节/旋转） |
| **media/** `analyzer.go` | 分析器：调 ffprobe；ExpandPaths 递归扫描文件夹找视频 |
| **encoder/** `bitrate.go` | §17 冻结码率表：H.264 分档；H.265×0.75；超4K按像素比+封顶；<720p下限1Mbps |
| **encoder/** `hardware.go` | 编码器探测（真实跑 6 帧测试编码验证可用）+ 自动选择（硬件优先→CPU兜底） |
| **encoder/** `builder.go` | 命令构建器：Simple/Pro → 各编码器原生参数映射（含 x265 HDR VUI 修复） |
| **task/** `task.go` | 任务模型 + 状态集（等待/准备/编码/暂停/验证/完成/取消/失败） |
| **task/** `manager.go` | 队列调度（信号量并发1-3）、暂停/恢复/取消、CPU 回退、冲突策略、历史落库、孤儿 Temp 清扫 |
| **task/** `helpers.go` + `disk_windows/other.go` | 辅助函数；磁盘剩余空间检测（编码前预警） |
| **ffmpegx/** `runner.go` | 启动 ffmpeg：解析 `-progress` 实时进度；stderr 收集（供错误分类） |
| **ffmpegx/** `suspend_windows/other.go` | 线程级暂停/恢复（SuspendThread 内核调用） |
| **ffmpegx/** `job_windows/other.go` | Job Object 绑命：主程序崩溃 → OS 自动杀 ffmpeg（防孤儿锁文件） |
| **validation/** `validation.go` | 输出复检：容器/编码/分辨率/FPS/时长/音轨硬检查 + HDR/字幕/章节警告 |
| **history/** `history.go` | SQLite 持久化（输入输出/编码器/压缩率/耗时/状态） |
| **settings/** `settings.go` | 语言/主题/输出/冲突/默认编码/硬件开关/API地址（含历史迁移链）/并行数 |
| **logging/** `logging.go` | 应用日志 7 天/500MB 上限；任务日志 20MB 上限后只记错误 |
| **auth/** `auth.go` | 授权状态机（8 态）；Simple 永远可用；只门控新任务 |
| **account/** `account.go` | 注册/验证/登录/激活/登出/删号 + 启动离线恢复 + 后台同步 |
| **account/** `credentials_windows/other.go` | Token 存 Windows 凭据管理器（不明文落盘） |
| **db/** `db.go` | SQLite 打开（WAL）+ 建表迁移 |
| **paths/** `paths.go` | 数据目录：首选安装目录 `Data\`（可写检测），回退 %LOCALAPPDATA% |
| **proc/** `proc.go` + `hide_windows.go` | 子进程统一封装（二进制校验）+ CREATE_NO_WINDOW 防黑框 |
| **errs/** `errs.go` | 12 类错误码；判定是否允许触发 CPU 回退 |

### 🌐 frontend/src/ —— Vue3 界面

| 文件 | 职责 |
|---|---|
| `main.ts` / `App.vue` | 应用引导；无边框自绘标题栏 + 侧边导航骨架 |
| `style.css` | 设计系统：苹果配色变量、深浅双主题、pill 按钮、圆角卡片、color-scheme |
| `router/index.ts` | 5 页路由（懒加载） |
| `i18n/index.ts` | 自研双语词典（中英全量，含专业模式 21 组标签） |
| `services/wails.ts` | 桥接层：调 Go 绑定 / 订阅事件（浏览器开发时优雅降级） |
| `stores/app.ts`、`tasks.ts` | 主题/语言/设置；任务列表 + 实时进度事件订阅 |
| `types/index.ts` | Go 结构体的 TS 镜像（MediaInfo/TaskView/HistoryEntry…） |
| `views/HomeView.vue` | 拖放导入 + Simple 配置 + Professional 全参数面板（并行/预估大小） |
| `views/TasksView.vue` | 任务：实时进度条/速度/ETA、暂停恢复取消重试 |
| `views/HistoryView.vue` | 历史：压缩率徽章、清空（不动视频） |
| `views/SettingsView.vue` | 设置：语言/主题/输出/冲突/编码/日志/账号登录注册 |
| `views/AboutView.vue` | 关于：版本/隐私承诺/开源声明/手动检查更新 |

### 🗄️ server/ —— 账号服务端（net/http 标准库，无框架）

| 文件 | 职责 |
|---|---|
| `handlers.go` | **Gin 路由与全部 HTTP 端点**：注册/验证/登录/刷新/登出/激活/设备/License/删号/admin；`ginAuthed`（Bearer）/`ginAdmin`（X-Admin-Key）中间件；`UpdateRuntime` 热更新入口 |
| `store.go` | Store 接口定义 + AdminStats 类型（SQL Server / SQLite 双实现开关） |
| `store_impl.go` | 全部 SQL 实现（占位符/日期方言适配） |
| `placeholders.go` | `?` → `@pN` 方言重写层（MSSQL 兼容核心） |
| `auth.go` | Argon2id 密码哈希/校验 + JWT 签发/解析 + Bearer 鉴权 |
| `mailer.go` | SMTP 发信（备选通道，587/465） |
| `resend.go` | Resend HTTP API 发信（主通道） |
| `adminui.go` + `adminui.html` | 苹果深色管理台（自包含单文件）：6 统计卡 + SVG 注册趋势 + 事件分布 + 账号管理 |
| `server_test.go` | 集成测试（跑在 Gin 引擎上）：全生命周期 + Resend mock（运行时生成密码，零字面量凭据） |

### 🚀 cmd/videoserver/ —— 服务端入口

| 文件 | 职责 |
|---|---|
| `main.go` | 装配：Viper 读配置 → 建库 → 挂热更新监听 → 带 Read/Write 超时的 http.Server |
| `config.go` | **Viper 配置**：config.json + `VIDEODELITE_*` 环境变量（优先级更高）+ 默认值；`WatchConfig` 热更新（adminKey/邮件/邀请码开关改文件即生效，免重启）；连接串/监听地址属重启型 |
| `config.json` | 真实运行配置（**gitignore**，含密码/Key；服务器上挂载进容器支持热更新） |
| `config.example.json` | 模板：含 Cloudflare DNS 步骤注释（可提交） |

> **依赖新增（r3.1，生产已部署验证）**：服务端引入 **Gin**（HTTP 框架，+3MB 仅影响服务端镜像）与 **Viper**（配置+热更新）。客户端体积不受影响。
> **热更新实测**：改服务器 `config/config.json` 的 adminKey → **4 秒内免重启生效**（旧 key 403 / 新 key 200 / 恢复 200）。
> **热更新已知坑**：容器**单文件** bind-mount 绑定 inode，`sed -i` 换 inode 后容器读到的永远是旧内容 —— 必须挂载**目录**（`./config:/config:ro`），且应用层每 2 秒轮询直读文件（fsnotify 对该场景同样失效）。

### 🐳 Linux 部署 / 📦 build / 🧰 installer / 🧪 tools

| 文件 | 职责 |
|---|---|
| `Dockerfile` | 两阶段：golang:alpine 编译 → debian:bookworm-slim 运行（GOPROXY 国内适配） |
| `docker-compose.yml` | server + SQL Server2022 双容器（健康检查/数据卷/凭据占位符） |
| `build/package.ps1` | 一键出安装包：wails build → 收集 ffmpeg(essentials) → NSIS |
| `build/sign.ps1` | Authenticode 签名（测试证书，正式证书一键替换） |
| `build/deploy-server.ps1` | 打服务端 Windows 部署 zip |
| `build/deploy-admin.sh` | 管理台热更新脚本（SSH 部署用；密钥经 `VD_ADMIN_KEY` 环境变量传入，脚本零硬编码） |
| `build/cache/deploy-*.json/yml`、`vd_deploy.sh` | 服务器部署专用真实配置与脚本（**gitignore**，含真实凭据，仅本地留存） |
| `build/nginx/` | 反代配置留档（双域名 443 + acme 续期路径） |
| `build/keys/test-signing.pfx` | 测试签名证书（**gitignore**） |
| `build/bin`、`build/dist`、`build/cache` | 产物：绿色客户端 / NSIS 安装包 / ffmpeg 缓存（**gitignore**） |
| `installer/installer.nsi` | NSIS 脚本：双语向导/升级覆盖/卸载保留数据(勾选才删)/Data 目录授权 |
| `installer/THIRD-PARTY-NOTICES.txt` | FFmpeg GPL 来源声明（§80 红线） |
| `tools/valmatrix` | Phase0 矩阵：真实编码验证 4 编码器 × 10bit/HDR/AAC → 生成报告 |
| `tools/qamatrix` | Phase8 矩阵：生成测试源 → 走产品管线 → 对比保留 |
| `tools/genicon` | 程序化生成应用图标 |

### 📚 docs/ 与根配置

| 文件 | 职责 |
|---|---|
| `docs/规划/VideoLite计划书.md` | 产品宪法（140 章冻结需求） |
| `docs/规划/授权状态机规范.md` | 8 态状态机规则 |
| `docs/规划/账号与本地压缩边界规范.md` | 账号域/视频域隔离 |
| `docs/规划/VideoDelite MVP交付计划书.md` | 本文档 |
| `docs/学习指南-从零到上线.md` | 小白复盘：20 个真实坑（现象→根因→解决→教训） |
| `docs/DEPLOY-DEBIAN.md` | 服务器部署手册（镜像加速/防火墙/HTTPS/排障） |
| `docs/TECHNICAL-VALIDATION.md`、`QA-PRESERVATION.md` | 两份实测报告 |
| `docs/RELEASE-NOTES.md`、`PRIVACY-POLICY.md` | 版本说明 / 隐私政策 |
| `.gitignore` / `.dockerignore` / `.zcodeignore` | 排除凭据/产物/缓存/服务器副本 |

### 数据流（谁调谁）

```
用户拖入视频
   ↓
[frontend HomeView] → [wails.ts 桥] → [app.go 绑定]
   ↓
[media] ffprobe 分析 → MediaInfo
   ↓
[encoder] 码率表 → 探测选编码器 → BuildCommand 生成参数
   ↓
[task] 排队（并发1-3）→ [ffmpegx] 启动 ffmpeg（Job Object 绑命）
   ↓                        ↑ 实时进度事件
[validation] FFprobe 复检 ──失败──→ 任务 Failed
   ↓ 通过
[history] SQLite 落库 → TasksView 进度条

账号线（完全独立）：
[account] 登录/激活 → [auth] 状态机 → 只门控"能否新建 Professional 任务"
                          （绝不触碰运行中的 ffmpeg —— 红线）
```

### "想改 X 应该动哪里"速查

| 想改什么 | 去哪里 |
|---|---|
| 压缩画质档位/码率 | `internal/encoder/bitrate.go` |
| 新增编码器（如 AV1） | `hardware.go` 探测 + `builder.go` 参数映射 |
| 界面文案/新增语言 | `frontend/src/i18n/index.ts` |
| 界面配色/圆角 | `frontend/src/style.css`（变量区） |
| 授权规则 | `internal/auth/auth.go` |
| 服务端 API | `server/handlers.go` |
| 管理台界面 | `server/adminui.html`（自包含单文件） |
| 数据库表结构 | 客户端 `internal/db/db.go`；服务端 `server/store.go` 迁移段 |
| 安装包行为 | `installer/installer.nsi` |
| 默认 API 地址 | `internal/settings/settings.go`（含历史地址迁移 switch） |

## 6. 已知限制与运维备忘

| # | 事项 | 说明 |
|---|---|---|
| 1 | ISP 封端口 | 家宽入站 80/443/8080/8443 被封；HTTPS 走 28443。**换网络环境可能需调整** |
| 2 | DNS 缓存 | 切换灰云后部分 ISP DNS 缓存长达一天；内网固定 hosts 解决 |
| 3 | 凭据轮换 | CF Token、SQL SA 密码曾出现在对话记录，建议轮换（学习指南 §10） |
| 4 | Authenticode 正式证书 | 当前测试自签名；商用需 OV/EV（`build/sign.ps1` 一键替换） |
| 5 | HDR10 动态元数据 | 基础标记 PASS；动态元数据尽力而为（输出验证给警告） |
| 6 | QSV/AMF | 无对应硬件，UI 按 Detect+Disable+Reason 处理 |
| 7 | License 策略 | 验证即发（免费）；商业化需接权益流程 |
| 8 | 证书续期 | acme.sh cron 自动（DNS-01），钩子自动 reload nginx；`/root/.acme.sh/account.conf` 存有 CF Token |

## 7. 验收结论

主计划书 §119 MVP 冻结要求全部实现并有实测证据；QA 发现的 3 个产品缺陷（HDR VUI 丢失、孤儿 ffmpeg、孤儿 Temp）已修复并回归；服务端在 Debian Docker 生产运行，双域名 HTTPS（Let's Encrypt 自动续期）接入完成；架构红线零违反。

## 8. 后续方向

| 选项 | 内容 |
|---|---|
| A | Authenticode 正式证书 + 正式发布流程 |
| B | 管理后台增强 + License 权益（商业化预留） |
| C | 更多 QA（AV1、更多容器、更高并发压测） |
| D | 客户端更新检查（§77 已预留） |
