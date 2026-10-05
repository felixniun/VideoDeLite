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
