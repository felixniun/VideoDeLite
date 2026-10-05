# VideoLite V1.0 完整产品开发计划书

> **项目名称：** VideoLite  
> **项目类型：** Windows 本地视频压缩 / 转码工具  
> **当前阶段：** 需求冻结 → 技术验证 → 正式开发  
> **计划版本：** V1.0  
> **文档状态：** 开发基准文档 / Agent 执行基准  
> **目标平台：** Windows 10 / Windows 11  
> **首发语言：** 简体中文 / English  
> **核心技术：** Vue 3 + TypeScript + Go + Wails v2.15.0 + FFmpeg + FFprobe  
> **服务器：** Go + SQL Server  
> **核心原则：** 本地处理、本地视频不上传、输出必须验证

---

# 0. 文档使用规则

本文件是 VideoLite 当前的**主产品开发计划书**。

后续开发 Agent 必须：

1. 以本文件为主要产品依据。
2. 不重新询问已经明确的产品决策。
3. 不擅自增加首发核心功能。
4. 不因为实现困难而偷偷改变产品行为。
5. 如果技术上无法完全实现某项要求：
   - 必须明确指出；
   - 给出技术限制；
   - 给出可行替代方案；
   - 不得静默降低功能。
6. “技术验证”只允许验证实现方式，不允许擅自改变已经冻结的产品决策。
7. 如果未来确实需要改变已冻结需求，必须单独记录为需求变更。

---

# 1. 产品定位

VideoLite 是一款：

> **简单、快速、本地优先、硬件编码优先的 Windows 视频压缩与转码工具。**

主要解决：

- 视频文件过大
- 手机/相机视频压缩
- 批量视频压缩
- H.264 / H.265 转码
- GPU 硬件编码
- 音频/字幕/多音轨保留
- HDR / 10-bit / 色彩信息尽可能保留

---

# 2. V1.0 核心产品目标

VideoLite V1.0 必须实现：

```text
视频导入
    ↓
媒体分析
    ↓
编码配置
    ↓
硬件/CPU Encoder 选择
    ↓
FFmpeg 编码
    ↓
实时进度
    ↓
输出验证
    ↓
结果展示
    ↓
任务历史
```

这条链路必须稳定。

---

# 3. 产品模式

VideoLite 提供：

```text
Simple Mode
Professional Mode
```

## Simple Mode

面向普通用户：

- 少参数
- 自动选择 Encoder
- Low / Mid / High
- H.264 / H.265
- MP4 / MKV
- AAC

## Professional Mode

面向高级用户：

- 手动 Encoder
- CBR
- VBR
- CQ / Quality
- 视频 Bitrate
- Max Bitrate
- 音频参数
- Stream 控制
- Metadata 控制
- 更完整的编码参数

---

# 4. 首发平台

## 4.1 Windows

支持：

```text
Windows 10
Windows 11
```

## 4.2 最低 Windows Build

正式冻结：

```text
Windows 10 Build 19041
```

即 Windows 10 2004 及以上。

理由：

- 覆盖 Windows 10 主流现代运行环境；
- 避免支持过老 Windows 10 Build；
- 与 WebView2 / Wails / Go / FFmpeg 现代运行环境保持合理兼容。

如果未来测试发现某个组件要求更高 Build：

> 必须在 Release 前统一提高最低版本，不允许只针对某个功能隐式提高要求。

---

# 5. Wails 版本

## 正式冻结

```text
Wails v2.15.0
```

不采用 Wails v3 作为 V1 正式开发基础。

原因：

- Wails v2.15.0 当前为稳定版本；
- 官方文档明确列出 v2.15.0；
- Wails v3 当前仍属于 Beta；
- V1 产品优先考虑稳定性而不是抢先使用新框架。

官方当前文档也明确显示 v3 仍为 Beta，而 v2 为当前稳定版本。

后续如果 Wails v3 正式稳定，可以在 V2 评估迁移。

---

# 6. WebView2

Wails Windows 应用依赖 WebView2 Runtime。

VideoLite 采用：

```text
检测 WebView2
      ↓
存在
      ↓
正常启动

不存在
      ↓
明确提示用户
      ↓
提供官方安装方式
```

禁止：

```text
后台静默下载
后台静默安装
```

首发不内置一个长期固定 WebView2 Runtime。

---

# 7. 发布形式

正式冻结：

```text
Installer Only
```

不提供 Portable。

安装程序负责：

- 安装程序文件
- 创建快捷方式
- 卸载
- 升级
- 必要运行环境检测

---

# 8. 用户数据与程序文件分离

程序：

```text
Program Files/
```

用户数据：

```text
User Data/
├── Config/
├── History/
├── Logs/
├── Temp/
└── Cache/
```

用户视频：

```text
由用户自行管理
```

VideoLite 不把用户视频复制到自己的数据目录。

---

# 9. 语言

首发：

```text
简体中文
English
```

## 首次启动语言策略

冻结：

> **第一次启动时默认跟随 Windows 系统语言。**

映射：

```text
Windows zh-CN / zh-Hans
        ↓
简体中文

其他语言
        ↓
English
```

用户可以在设置中手动修改。

语言设置独立保存。

不会因为以后 Windows 系统语言改变而自动覆盖用户手动选择。

---

# 10. Theme

首发：

```text
Light
Dark
```

默认：

```text
跟随 Windows 系统主题
```

但用户可以手动切换。

一旦用户手动选择：

```text
Light / Dark
```

以后优先使用用户选择。

前端使用统一 Vue Theme 系统。

---

# 11. 输入文件

支持：

- 单文件
- 多文件
- 文件夹
- 文件夹递归扫描
- Drag & Drop

导入后：

```text
FFprobe
```

分析媒体。

---

# 12. 输出 Container

V1：

```text
MP4
MKV
```

不增加其他 Container。

---

# 13. 默认输出格式

冻结：

```text
MP4
H.264
AAC
```

即：

```text
MP4 + H.264 + AAC
```

这是首次使用默认配置。

---

# 14. Video Codec

V1：

```text
H.264
H.265
```

两种模式都支持。

---

# 15. Audio Codec

冻结：

```text
AAC
```

所有输出音频统一为 AAC。

---

# 16. Simple Mode 视频质量

提供：

```text
Low
Mid
High
```

三个等级。

编码器自动选择。

---

# 17. Simple Mode H.264 码率表

冻结：

| 分辨率 / FPS | Low | Mid | High |
|---|---:|---:|---:|
| 720p 24–30 | 1.8 Mbps | 3 Mbps | 4.5 Mbps |
| 720p 48–60 | 2.5 Mbps | 4 Mbps | 6 Mbps |
| 1080p 24–30 | 3.5 Mbps | 5 Mbps | 8 Mbps |
| 1080p 48–60 | 5 Mbps | 7 Mbps | 10 Mbps |
| 1440p 24–30 | 6 Mbps | 10 Mbps | 16 Mbps |
| 1440p 48–60 | 8 Mbps | 14 Mbps | 20 Mbps |
| 4K 24–30 | 10 Mbps | 16 Mbps | 25 Mbps |
| 4K 48–60 | 14 Mbps | 20 Mbps | 30 Mbps |

---

# 18. Simple Mode H.265 码率策略

为了避免 H.265 直接使用 H.264 相同码率导致压缩优势没有体现：

冻结：

> **H.265 Simple Mode 使用 H.264 对应档位约 75% 的目标码率。**

计算：

```text
H.265 bitrate
≈
H.264 bitrate × 0.75
```

最终进行 0.5 Mbps 粒度取整。

例如：

| H.264 | H.265 |
|---:|---:|
| 8 Mbps | 6 Mbps |
| 10 Mbps | 7.5 Mbps |
| 16 Mbps | 12 Mbps |
| 20 Mbps | 15 Mbps |
| 25 Mbps | 18.5 Mbps |
| 30 Mbps | 22.5 Mbps |

原则：

> 这是 Simple Mode 的默认 bitrate mapping，不代表 H.265 在所有视频内容上都能获得严格 25% 文件体积下降。

HDR / 10-bit / 特殊编码器：

> 如果硬件 Encoder 无法按照该 bitrate / pixel format 正常编码，应进入兼容性处理，而不是强制生成错误文件。

---

# 19. 表格之外的分辨率策略

输入可能出现：

- 360p
- 480p
- 540p
- 2160p
- 4320p
- 非标准宽高
- 自定义 FPS

冻结：

> **采用最近匹配分辨率 + 最近匹配 FPS 档位。**

例如：

```text
1920×1080
30fps
↓
1080p 24–30
```

例如：

```text
2560×1440
50fps
↓
1440p 48–60
```

---

# 20. 超出 4K 的视频

例如：

```text
5K
6K
8K
```

V1：

> 使用 4K 档位作为基础参考，但根据实际像素数量进行比例调整。

禁止简单认为：

```text
8K = 4K bitrate
```

推荐计算：

```text
Target Bitrate
=
4K Baseline
×
Input Pixel Count / 4K Pixel Count
```

同时设置合理上限，防止产生异常巨大的 bitrate。

具体上限必须在技术测试阶段验证。

---

# 21. 极低分辨率

例如：

```text
360p
480p
```

使用：

```text
720p Baseline
×
Pixel Ratio
```

同时设置最低 bitrate：

```text
1 Mbps
```

避免因为比例缩放产生过低 bitrate。

---

# 22. 非标准 FPS

例如：

```text
25fps
29.97fps
50fps
59.94fps
120fps
```

冻结：

```text
24–30
48–60
```

之外：

- 不主动改变 FPS；
- 尽可能保持源 FPS；
- 采用最近档位作为 bitrate baseline；
- Professional 模式允许用户进一步调整。

对于：

```text
90fps
120fps
240fps
```

必须进行技术兼容性判断。

不能为了套用表格强制转换 FPS。

---

# 23. Simple AAC 默认码率

冻结：

```text
AAC 128 kbps
```

Simple Mode 固定：

```text
AAC
128 kbps
```

不向普通用户暴露复杂参数。

---

# 24. Professional AAC

Professional：

```text
64 kbps
96 kbps
128 kbps
160 kbps
192 kbps
224 kbps
256 kbps
320 kbps
384 kbps
512 kbps
```

允许用户输入时只允许落在：

```text
64–512 kbps
```

范围。

超过范围：

```text
自动限制
```

或者提示错误。

---

# 25. Professional AAC Rate Control

支持：

```text
CBR
VBR / Quality
```

Simple 不开放。

Professional 开放。

---

# 26. Professional AAC VBR Quality

统一 UI：

```text
Quality 1
Quality 2
Quality 3
Quality 4
Quality 5
```

其中：

```text
1 = Lower Quality / Smaller
3 = Balanced
5 = Higher Quality / Larger
```

实际 FFmpeg 参数：

> 根据实际 AAC Encoder 映射。

前端不直接暴露 codec-specific 原始数值。

---

# 27. Professional Video Rate Control

正式冻结：

```text
CBR
VBR
CQ / Quality
```

---

# 28. CBR

显示：

```text
Bitrate
```

用户输入：

```text
5 Mbps
10 Mbps
20 Mbps
```

编码器根据能力生成对应 FFmpeg 参数。

---

# 29. VBR

显示：

```text
Average Bitrate
Max Bitrate
```

规则：

```text
Max Bitrate >= Average Bitrate
```

如果：

```text
Max < Average
```

直接提示。

---

# 30. CQ / Quality

Professional 使用统一：

```text
Quality
```

界面。

实际 backend 根据 Encoder 转换：

```text
NVENC → CQ / QP 类参数
x264 → CRF
x265 → CRF
QSV → 对应质量参数
AMF → 对应质量参数
```

不强制所有编码器使用相同 FFmpeg 参数名称。

---

# 31. CQ / Quality UI 范围

冻结：

```text
Quality: 0–51
```

但：

> 该数值不是所有 Encoder 的直接原生值。

Backend 必须建立 Encoder-specific mapping。

如果某 Encoder 原生范围不同：

```text
UI 0–51
↓
Backend Mapping
↓
Native Parameter
```

同时在 UI 中提供简短说明。

---

# 32. Professional Encoder

可选择：

```text
Auto
CPU Software
NVIDIA NVENC
Intel QSV
AMD AMF/VCN
```

仅显示当前系统实际支持的选项。

不可用：

```text
Disabled
+
Reason
```

---

# 33. 默认 Encoder

冻结：

> **优先硬件编码。**

优先级：

```text
Available Compatible Hardware Encoder
        ↓
CPU Software Encoder
```

具体 NVIDIA / Intel / AMD 不人为规定绝对优先级。

根据：

- 当前 GPU
- Codec
- Bit Depth
- HDR
- Driver
- Pixel Format

动态选择。

---

# 34. CPU Fallback

只在：

```text
Encoder-related failure
```

时触发。

流程：

```text
Hardware Encoder
      ↓
Failure
      ↓
Classify Error
      ↓
Encoder-related?
      ↓
Yes
      ↓
CPU fallback
```

最多自动 fallback 一次。

CPU 失败：

```text
Task Failed
```

---

# 35. Task 并发

V1 冻结：

> **默认只允许 1 个编码任务同时运行。**

即：

```text
Task 1 → Encoding
Task 2 → Waiting
Task 3 → Waiting
```

原因：

- GPU 编码资源复杂；
- 不同厂商 Encoder 行为不同；
- CPU / RAM / VRAM 消耗容易不可控；
- V1 首先保证稳定性。

后续版本可以增加：

```text
Concurrent Encoding
```

但不属于 V1。

---

# 36. Task Queue

状态：

```text
Waiting
Preparing
Encoding
Paused
Completed
Canceled
Failed
```

---

# 37. Pause / Resume

支持：

```text
Pause
Resume
Cancel
```

但：

> Pause / Resume 是进程级控制，不是 checkpoint。

---

# 38. 程序重启

不支持：

```text
重启程序
↓
恢复 FFmpeg
```

异常退出：

```text
清理本任务明确拥有的 Temp
```

不自动恢复。

---

# 39. Task History

历史本地保存。

包括：

- 输入文件信息
- 输出文件
- Encoder
- Codec
- Bitrate
- Duration
- Encoding Time
- Output Size
- Compression Ratio
- Status
- Error Summary

程序重启后历史仍存在。

---

# 40. Clear History

用户可以：

```text
Clear History
```

清理：

```text
History records
```

不会删除：

```text
已生成视频
源视频
```

---

# 41. 日志

日志：

```text
Local Only
```

保留：

```text
7 Days
```

自动清理。

---

# 42. 日志容量

冻结：

```text
Total Log Storage: 500 MB
```

当超过：

```text
500 MB
```

优先删除最旧日志。

---

# 43. 单任务日志上限

冻结：

```text
20 MB / Task
```

超过后：

```text
继续记录关键错误
停止记录高频重复 FFmpeg 输出
```

避免某个异常任务刷爆磁盘。

---

# 44. 日志导出

V1 支持：

```text
Export Logs
```

导出：

```text
ZIP
```

内容：

- Application logs
- Selected task logs
- System information
- Encoder detection information

默认：

> 不包含用户视频。

也不包含：

- 视频内容
- 视频文件本身

路径信息如果存在于诊断日志中：

> 导出前进行隐私处理，尽可能隐藏用户名及不必要的绝对路径。

---

# 45. Privacy

服务器绝对不接收：

- 视频
- 视频文件名
- 视频路径
- 视频 metadata
- FFmpeg command
- Compression behavior
- Diagnostic logs

服务器只处理必要：

- Account
- Device
- Authorization
- Security
- Version
- Email verification

---

# 46. Metadata

默认：

> 尽可能保留全部兼容 Metadata。

包括：

- Title
- Artist / Creator
- Creation Time
- Language
- Cover
- Camera information
- GPS
- Device information
- Other compatible metadata

---

# 47. Rotation

尽可能：

```text
Preserve Rotation Metadata
```

不主动进行：

```text
Pixel Rotation
```

除非容器/编码流程无法保证最终播放方向。

输出必须 FFprobe 验证。

---

# 48. HDR / 10-bit

尽可能保留：

- 10-bit
- HDR
- Color Space
- Transfer
- Primaries
- HDR metadata

不能只复制 metadata 就声称 HDR 完整保留。

必须检查：

```text
Source
→ Encoder
→ Pixel Format
→ Output
```

---

# 49. 字幕

尽可能保留所有兼容字幕轨道。

保留：

- Language
- Track Name
- Default flag

不兼容时：

```text
Warning
```

不得静默删除。

---

# 50. 多音轨

尽可能保留所有兼容音轨。

输出统一：

```text
AAC
```

保留：

- Track
- Language
- Name
- Default flag

---

# 51. Chapters

尽可能保留。

输出后使用 FFprobe 验证。

---

# 52. Output Validation

编码完成后必须：

```text
FFmpeg Exit Code
+
File Exists
+
File Size > 0
+
FFprobe
```

---

# 53. Validation 内容

检查：

### Container

- Format
- Duration

### Video

- Codec
- Resolution
- FPS
- Bitrate
- Pixel Format
- Bit Depth
- HDR
- Color
- Rotation

### Audio

- Number of tracks
- Codec
- Bitrate
- Sample Rate
- Channels
- Language

### Subtitle

- Track count
- Language
- Format

### Chapters

- Chapter count

### Metadata

- Key metadata

---

# 54. 输出验证失败

即使：

```text
FFmpeg exit code = 0
```

如果 FFprobe 验证失败：

```text
Task = Failed
```

不能告诉用户：

```text
Compression completed
```

---

# 55. 文件冲突

支持：

```text
Overwrite
Skip
Auto Number
Cancel
```

支持：

```text
Remember Globally
```

---

# 56. 输出目录

首次：

```text
Compressed
```

默认输出目录。

用户可以修改。

程序记忆最后一次选择。

---

# 57. 输出文件名

建议：

```text
OriginalName_low.mp4
OriginalName_mid.mp4
OriginalName_high.mp4
```

Professional 根据实际配置可以生成：

```text
OriginalName_compressed.mp4
```

---

# 58. 文件夹结构

保持原始目录结构。

例如：

```text
Videos/
├── Travel/
│   └── Japan.mp4
└── Camera/
    └── Sony.mp4
```

输出：

```text
Compressed/
├── Travel/
│   └── Japan.mp4
└── Camera/
    └── Sony.mp4
```

---

# 59. 源文件删除

默认：

```text
Preserve Source
```

只有：

```text
Encode Success
+
Validation Success
```

后才能允许删除。

---

# 60. UI 页面

```text
Home
Tasks
History
Settings
About
```

---

# 61. Home

主要区域：

```text
Drag & Drop
```

支持：

- 文件
- 文件夹

---

# 62. Simple UI

核心：

```text
Video
 ├── H.264
 └── H.265

Quality
 ├── Low
 ├── Mid
 └── High

Container
 ├── MP4
 └── MKV

Output Folder

[ Start Compression ]
```

---

# 63. Professional UI

### Video

- Codec
- Encoder
- Rate Control
- Bitrate
- Max Bitrate
- Quality
- Preset

### Audio

- AAC
- Bitrate
- CBR / VBR

### Streams

- Audio
- Subtitle

### Metadata

- Preserve Metadata

### Output

- Container
- Output Folder
- Conflict Strategy

---

# 64. Professional Preset

Encoder preset属于：

> Professional 功能。

V1 支持 encoder 能力范围内的常见 preset。

不强制所有 Encoder 使用相同名称。

---

# 65. 设置

### General

- Language
- Theme
- Output Directory
- Conflict Strategy

### Encoding

- Default Codec
- Hardware Encoding

### History

- Clear History

### Logs

- View Logs
- Export Logs

### Account

- Login
- Logout
- Professional Status

---

# 66. 本地存储

本地数据建议：

```text
SQLite
```

用于：

- Task History
- Settings
- Local application state

不建议 SQL Server 直接用于桌面客户端本地数据。

---

# 67. 后端

服务器：

```text
Go
SQL Server
```

负责：

- Account
- Registration
- Email Verification
- Authentication
- Device
- License
- Admin
- Security Audit
- Version

---

# 68. 账号

支持：

- Username
- Email
- Password
- Email Verification
- Invite Code

---

# 69. Password

禁止明文保存。

采用成熟密码哈希：

```text
Argon2id
```

服务器不保存：

```text
Plaintext Password
```

---

# 70. Device Activation

采用：

```text
Random Installation ID
```

禁止：

```text
Hardware Fingerprint
```

不绑定：

- CPU
- GPU
- Disk Serial
- MAC Address

---

# 71. Offline Authorization

授权成功后：

```text
Local Authorization
```

允许长期离线。

启动时不要求每次联网。

---

# 72. Logout

Logout 后：

```text
Professional Mode
```

隐藏或进入未授权状态。

---

# 73. Account Ban

管理员封禁：

```text
禁止新设备激活
```

已有本地授权不因为短暂网络不可用立即失效。

---

# 74. Account Delete

删除：

```text
Personal Data
```

保留：

```text
必要匿名授权 / 安全记录
```

---

# 75. 管理后台

V1：

```text
Accounts
Registration
Email Verification
Devices
App Versions
Security Audit
```

禁止统计用户：

- 视频
- 视频名称
- 压缩行为
- FFmpeg command

---

# 76. 网络策略

网络仅用于：

```text
Login
Register
Email Verification
License
Device Activation
Manual Update Check
```

不用于：

```text
Video Upload
Log Upload
Media Upload
Compression Analytics
```

---

# 77. 更新策略

冻结：

> **V1 不做强制自动更新。**

提供：

```text
Settings
→ About
→ Check for Updates
```

用户手动检查。

发现新版本：

```text
显示版本
显示更新说明
用户主动下载安装
```

不后台静默升级。

---

# 78. Release Channel

V1 只使用：

```text
Stable
```

不向普通用户开放：

```text
Beta
Nightly
Dev
```

开发团队内部可以自行使用开发构建。

未来可以增加 Beta Channel。

---

# 79. Code Signing

正式发布版本必须：

```text
Windows Authenticode
SHA-256
Timestamp
```

安装程序签名。

主程序签名。

FFmpeg / FFprobe 等随程序发布的二进制文件：

> 如果许可证和分发方式允许，也应进行来源和完整性校验；不能伪造第三方签名。

---

# 80. FFmpeg 分发

必须在项目中明确：

- FFmpeg License
- FFmpeg Build 来源
- 第三方组件许可证
- License 文件

不能把 FFmpeg 当作普通闭源代码隐藏来源。

---

# 81. 技术架构

```text
┌─────────────────────────────┐
│          Vue 3              │
│       TypeScript            │
├─────────────────────────────┤
│ UI / Store / i18n / Service │
└──────────────┬──────────────┘
               │ Wails
┌──────────────▼──────────────┐
│            Go               │
├─────────────────────────────┤
│ Task Manager                │
│ Media Analyzer              │
│ FFmpeg Manager              │
│ FFprobe Manager             │
│ Encoder Strategy            │
│ Validation                  │
│ History                     │
│ Logging                     │
│ Settings                    │
│ License                     │
└──────────────┬──────────────┘
               │
        ┌──────┴───────┐
        │              │
     FFmpeg         FFprobe
```

---

# 82. Frontend

技术：

```text
Vue 3
TypeScript
Composition API
```

建议：

```text
Pinia
Vue Router
i18n
```

具体 UI 组件库：

> 开发阶段选择一个稳定方案，不允许同时引入多个大型 UI Framework。

---

# 83. Backend

Go：

```text
Task
FFmpeg
FFprobe
Hardware
Validation
History
Settings
License
```

---

# 84. FFmpeg Command Builder

禁止前端直接拼 FFmpeg command。

必须：

```text
Frontend
 ↓
EncodingConfig
 ↓
Go
 ↓
Encoder Strategy
 ↓
Command Builder
 ↓
FFmpeg
```

---

# 85. EncodingConfig

建议：

```text
EncodingConfig {
    Container
    VideoCodec
    Encoder
    RateControl
    Bitrate
    MaxBitrate
    Quality
    AudioCodec
    AudioBitrate
    AudioRateControl
    PreserveMetadata
    PreserveSubtitles
    PreserveChapters
    PreserveAudioTracks
}
```

---

# 86. Encoder Strategy

统一：

```text
EncoderSelector
```

输入：

```text
MediaInfo
HardwareInfo
UserConfig
```

输出：

```text
SelectedEncoder
FinalEncodingParameters
```

---

# 87. Hardware Detection

统一：

```text
HardwareInfo
```

包含：

```text
Vendor
Model
Driver
NVENC
QSV
AMF
Supported Codecs
Supported Pixel Formats
Bit Depth
HDR capability
```

---

# 88. 错误分类

至少：

```text
InputFileError
OutputFileError
EncoderUnavailable
EncoderInitializationFailed
UnsupportedCodec
UnsupportedPixelFormat
DriverError
FFmpegArgumentError
DiskFull
PermissionDenied
ValidationFailed
UnknownError
```

---

# 89. Fallback 判断

只有：

```text
EncoderUnavailable
EncoderInitializationFailed
Driver/Encoder failure
```

等符合条件的错误：

```text
Hardware
→ CPU
```

参数错误不能无限 fallback。

---

# 90. Task Manager

必须负责：

- Queue
- Process lifecycle
- Progress
- Pause
- Resume
- Cancel
- Retry
- Fallback
- Validation
- History

---

# 91. Progress

FFmpeg：

```text
-progress
```

解析：

- frame
- fps
- bitrate
- total_size
- out_time
- speed
- progress

转换成统一：

```text
TaskProgress
```

---

# 92. Temp

每个 Task：

```text
Temp/
└── task-id/
```

异常时：

> 只清理属于当前任务的 Temp。

---

# 93. 磁盘空间

编码前检查：

```text
Available Disk Space
```

如果明显不足：

```text
Warning
```

编码过程中空间不足：

```text
Task Failed
```

不能生成损坏输出并标记成功。

---

# 94. 日志架构

```text
Application Log
Task Log
FFmpeg Log
Validation Log
Security Log
```

分类存储。

---

# 95. History 与 Log 分离

严格区分：

```text
Task History
Application Log
Temp
User Video
```

清除 History：

> 不删除视频。

清理 Logs：

> 不删除 History。

---

# 96. 已冻结的产品细节

本章节原本是待确认项目，现在已经全部冻结。

| 项目 | 最终决定 |
|---|---|
| Windows 最低版本 | Windows 10 Build 19041 |
| 首次语言 | 跟随 Windows |
| 用户手动语言 | 支持 |
| H.265 Simple bitrate | H.264 × 75% |
| 特殊分辨率 | 最近档位 + 比例调整 |
| 超 4K | 按像素比例调整 |
| 低于 720p | 按像素比例调整 + 最低 1 Mbps |
| 特殊 FPS | 最近 FPS 档位，不主动改变 FPS |
| Simple AAC | 128 kbps |
| Professional AAC | 64–512 kbps |
| AAC VBR | Quality 1–5 |
| Video CBR | 支持 |
| Video VBR | 支持 |
| Video CQ | 支持 |
| CQ UI | Quality 0–51 |
| Task 并发 | V1 默认 1 |
| Log 总容量 | 500 MB |
| 单 Task Log | 20 MB |
| Log Export | 支持 ZIP |
| Release Channel | Stable |
| Auto Update | 不做静默自动更新 |
| Update | 用户手动检查 |
| Code Signing | Authenticode + SHA-256 + Timestamp |
| Wails | v2.15.0 |
| WebView2 | 检测 + 用户明确处理 |

---

# 97. 哪些内容仍然只允许“技术验证”

下面内容可以验证，但：

> **不能因为验证结果不同就擅自改变产品需求。**

包括：

- FFmpeg 某 Encoder 的实际参数名称
- NVENC 的 CQ 映射
- QSV 的 Quality 映射
- AMF 的 Quality 映射
- HDR + 10-bit + Hardware Encoder 组合
- MP4 + H.265 + HDR
- MKV + 字幕
- AAC VBR 参数映射
- Windows 10 Build 19041 实际兼容性
- WebView2 安装情况
- GPU Driver 差异
- FFmpeg 各版本差异

如果某个功能：

```text
理论支持
但某个 Encoder 不支持
```

应该做：

```text
Capability Detection
+
UI Disable
+
Reason
```

而不是删除整个产品功能。

---

# 98. 技术验证矩阵

第一阶段必须建立：

```text
Encoder
×
Codec
×
Bit Depth
×
HDR
×
Container
×
Audio
×
Subtitle
```

至少测试：

```text
CPU
NVENC
QSV
AMF
```

与：

```text
H.264
H.265
```

组合。

---

# 99. 第一阶段开发顺序

严格建议：

```text
01 项目初始化
02 Wails
03 Vue UI
04 FFmpeg
05 FFprobe
06 Media Analyzer
07 Task Manager
08 Simple Mode
09 Hardware Detection
10 CPU Fallback
11 Output Validation
12 History
13 Professional Mode
14 Audio
15 Subtitle
16 Metadata
17 HDR/10-bit
18 Logs
19 Settings
20 Account
21 License
22 Installer
23 Code Signing
24 QA
25 Release
```

---

# 100. Phase 0：技术验证

优先级：

```text
P0
```

验证：

- Wails 2.15
- Windows 10 Build 19041
- Windows 11
- WebView2
- FFmpeg
- FFprobe
- NVENC
- QSV
- AMF
- H.264
- H.265
- AAC
- MP4
- MKV
- HDR
- 10-bit
- Subtitle
- Multi Audio
- Chapters
- Metadata
- Rotation

---

# 101. Phase 1：工程初始化

建立：

```text
videolite/
├── frontend/
├── backend/
├── tests/
├── build/
├── installer/
└── docs/
```

完成：

- Git
- Vue
- TS
- Go
- Wails
- Build
- Dev environment
- CI

---

# 102. Phase 2：基础 UI

完成：

- App Shell
- Sidebar
- Header
- Home
- Tasks
- History
- Settings
- About
- Theme
- i18n

验收：

```text
启动
+
页面切换
+
中英文
+
Light/Dark
```

---

# 103. Phase 3：Media Analyzer

完成：

```text
FFprobe
MediaInfo
Preflight
```

验收：

```text
H264
H265
10-bit
HDR
Audio
Subtitle
Chapter
Rotation
Metadata
```

---

# 104. Phase 4：Task Manager

完成：

- Queue
- State
- Process
- Progress
- Pause
- Resume
- Cancel
- Error

---

# 105. Phase 5：Simple Mode

完成：

```text
H.264
H.265
Low
Mid
High
MP4
MKV
AAC
```

并完成：

```text
Hardware
+
CPU fallback
```

---

# 106. Phase 6：Validation

必须完成：

```text
Encode
↓
FFprobe
↓
Validate
↓
History
```

---

# 107. Phase 7：Professional Mode

完成：

```text
Encoder
Codec
CBR
VBR
CQ
Bitrate
Max Bitrate
Quality
```

---

# 108. Phase 8：Media Preservation

完成：

- Multi Audio
- AAC
- Subtitle
- Chapters
- Metadata
- Rotation
- HDR
- 10-bit

---

# 109. Phase 9：Local Data

完成：

- SQLite
- History
- Settings
- Logs
- 7-day cleanup
- 500 MB cap
- 20 MB task cap
- ZIP Export

---

# 110. Phase 10：Account

完成：

```text
Register
Login
Email Verification
Device
License
Logout
Delete Account
Admin
```

---

# 111. Phase 11：Installer

完成：

- Installer
- Upgrade
- Uninstall
- WebView2 detection
- FFmpeg distribution
- Config migration

---

# 112. Phase 12：Release

完成：

- Code Signing
- Stable build
- QA
- Windows 10
- Windows 11
- Documentation
- License
- Privacy Policy
- Release Notes

---

# 113. QA 测试矩阵

## Resolution

```text
360p
480p
720p
1080p
1440p
4K
5K/6K/8K
```

## FPS

```text
24
25
29.97
30
50
59.94
60
120
```

## Codec

```text
H.264
H.265
```

## Bit Depth

```text
8-bit
10-bit
```

## Dynamic Range

```text
SDR
HDR
HLG
HDR10
```

---

# 114. Hardware QA

测试：

```text
NVIDIA
Intel
AMD
CPU Only
```

重点：

- Encoder detection
- Encoder failure
- CPU fallback
- HDR
- 10-bit
- H.265

---

# 115. Stability QA

连续：

```text
10 tasks
50 tasks
100 tasks
```

观察：

- RAM
- VRAM
- CPU
- GPU
- Handle
- Process
- Temp
- Log

---

# 116. Crash QA

测试：

- 强制关闭
- FFmpeg crash
- GPU driver failure
- Disk full
- Permission denied
- Input deleted
- Output deleted
- Network unavailable
- Server unavailable
- WebView2 unavailable

---

# 117. 数据安全 QA

必须验证：

```text
视频不会上传
视频路径不会上传
FFmpeg command 不上传
日志不会自动上传
```

断网：

```text
Local Compression
```

仍然可以正常执行。

Professional 已经授权：

```text
断网
↓
仍可以正常使用
```

---

# 118. Release 验收标准

V1.0 必须：

```text
Windows 10 Build 19041
Windows 11
```

正常运行。

必须能够：

```text
Import
Analyze
Encode
Progress
Validate
History
```

---

# 119. MVP

MVP 最小闭环：

```text
Wails
+
Vue
+
Go
+
FFmpeg
+
FFprobe

Import
+
Media Info
+
Simple Mode
+
H264/H265
+
Hardware
+
CPU fallback
+
AAC
+
MP4/MKV
+
Task Queue
+
Progress
+
Validation
+
History
```

---

# 120. MVP 不做

MVP 阶段暂不要求：

- Account
- License
- Admin
- Professional CQ
- Advanced Metadata UI

但核心架构必须预留接口。

---

# 121. V1 Beta

Beta 必须完整：

```text
Simple
+
Professional
+
CBR
+
VBR
+
CQ
+
Audio
+
Subtitle
+
Metadata
+
HDR
+
History
+
Logs
```

---

# 122. Release Candidate

RC 阶段：

> 不再增加核心功能。

只允许：

- Bug Fix
- Performance
- Compatibility
- Security
- UX polish
- Installer fixes

---

# 123. V1.0

V1.0 标准：

```text
稳定
+
可预测
+
输出可靠
+
不误删
+
不上传用户视频
+
硬件编码稳定
```

---

# 124. 后续 V1.x

可以增加：

- AV1
- Parallel Encoding
- Advanced Preview
- Before/After comparison
- Preset Import/Export
- More Hardware Detection
- More Statistics
- More Container
- More Codec

---

# 125. V2

未来：

```text
macOS
Linux
```

以及：

- 更专业的视频工作流
- 更丰富的媒体格式
- 高级预览
- 高级队列调度
- 更复杂的编码策略

---

# 126. Agent 执行须知

## 已经冻结，不得擅自修改

以下均属于产品决策：

```text
Windows 10 Build 19041
Wails 2.15.0
Installer Only
zh-CN + English
Light + Dark
MP4 + MKV
H.264 + H.265
AAC
Simple Low/Mid/High
Hardware First
CPU Fallback
Professional Manual Encoder
CBR
VBR
CQ
HDR preservation
10-bit preservation
Rotation preservation
Subtitle preservation
Multi-audio preservation
Chapter preservation
Metadata preservation
Local History
7-day Logs
500 MB Logs
20 MB Task Log
No checkpoint resume
No automatic task recovery
No video upload
No compression analytics
Stable Channel
Manual Update Check
Code Signing
```

---

# 127. Agent 可以自行决定的事项

Agent 可以根据工程实践决定：

- 代码文件如何划分
- Go package 结构
- Vue component 结构
- Pinia store 结构
- API 命名
- Event naming
- Error code naming
- 测试文件组织
- CI 实现
- Build script
- Installer script
- SQLite 表具体字段

但必须遵守产品行为。

---

# 128. Agent 不得自行改变的事项

禁止：

```text
把 Simple 改成 CRF
```

禁止：

```text
删除 H.265
```

禁止：

```text
删除 MKV
```

禁止：

```text
自动上传日志
```

禁止：

```text
强制联网验证
```

禁止：

```text
重启后自动恢复任务
```

禁止：

```text
默认删除源文件
```

禁止：

```text
把 Professional 变成付费后才能编码
```

禁止：

```text
偷偷取消 HDR/10bit 保留
```

禁止：

```text
因为某 Encoder 不支持而删除整个功能
```

---

# 129. 技术问题处理规则

如果：

```text
Feature
```

在某个硬件上不可用：

应该：

```text
Detect
↓
Disable
↓
Explain
```

而不是：

```text
Remove
```

---

# 130. 需求变更规则

如果 Agent 认为某项需求必须修改：

必须先产生：

```text
Requirement Change Proposal
```

格式：

```text
Original:
原需求

Problem:
技术问题

Impact:
影响

Option A:
方案 A

Option B:
方案 B

Recommendation:
技术建议

Risk:
风险
```

不能直接修改主计划。

---

# 131. 第一阶段 Agent 实际任务

第一批不要直接做完整 UI。

按照：

```text
TASK-001
项目初始化

TASK-002
Wails 2.15 Windows Build

TASK-003
WebView2 Detection

TASK-004
FFmpeg Distribution

TASK-005
FFprobe Test Harness

TASK-006
MediaInfo Parser

TASK-007
H264 CPU Test

TASK-008
H264 NVENC Test

TASK-009
H264 QSV Test

TASK-010
H264 AMF Test

TASK-011
H265 Test

TASK-012
AAC Test

TASK-013
MP4 Test

TASK-014
MKV Test

TASK-015
HDR/10bit Test

TASK-016
Subtitle/Multi-Audio Test

TASK-017
Metadata/Rotation Test
```

开始。

---

# 132. 第一阶段最终产物

必须得到：

```text
Technical Validation Matrix
```

例如：

| Feature | CPU | NVENC | QSV | AMF |
|---|---|---|---|---|
| H.264 | PASS | PASS | PASS | PASS/Partial |
| H.265 | PASS | PASS | PASS | PASS/Partial |
| 10-bit | Test | Test | Test | Test |
| HDR | Test | Test | Test | Test |
| AAC | PASS | PASS | PASS | PASS |
| Subtitle | PASS | PASS | PASS | PASS |
| Metadata | PASS | PASS | PASS | PASS |

实际结果必须来自测试。

不能提前填写假结果。

---

# 133. 第一条核心 Demo

开发初期必须尽快完成：

```text
Drag MP4
    ↓
FFprobe
    ↓
Display Media Info
    ↓
H.264
    ↓
Low
    ↓
Hardware Encoder
    ↓
FFmpeg
    ↓
Progress
    ↓
Output
    ↓
FFprobe
    ↓
Validation
    ↓
Completed
```

这是 VideoLite 的第一个真正技术里程碑。

---

# 134. 第二条核心 Demo

完成：

```text
Professional
    ↓
H.265
    ↓
NVENC
    ↓
CBR
    ↓
10 Mbps
    ↓
Encode
    ↓
Validation
```

然后：

```text
VBR
```

再：

```text
CQ / Quality
```

三条全部跑通。

---

# 135. 第三条核心 Demo

测试媒体保留：

```text
HDR
+
10-bit
+
Multiple Audio
+
Subtitle
+
Chapter
+
Rotation
+
Metadata
```

输入：

```text
Source
```

输出：

```text
Compressed
```

然后自动比较：

```text
Source MediaInfo
vs
Output MediaInfo
```

---

# 136. 产品成功标准

VideoLite V1.0 不以：

```text
功能数量
```

作为主要成功标准。

核心成功标准是：

```text
用户拖进去
        ↓
选择质量
        ↓
点击压缩
        ↓
稳定完成
        ↓
得到可靠视频
```

同时：

```text
不会误删源文件
不会偷偷上传视频
不会生成损坏文件
不会因为 GPU 问题无限报错
不会让普通用户必须理解 FFmpeg
```

---

# 137. 最终项目状态

```text
需求定义             100%
产品决策             100%
核心架构             90%
技术验证              0% → 当前正式开始
核心开发              0%
UI                    0%
QA                    0%
Release               0%
```

现在项目正式进入：

```text
PRODUCT DEFINED
        ↓
TECHNICAL VALIDATION
        ↓
IMPLEMENTATION
```

阶段。

---

# 138. 最终执行顺序

**Agent 不需要重新进行产品采访。**

直接执行：

```text
Phase 0
技术验证
        ↓
Phase 1
项目初始化
        ↓
Phase 2
UI 基础
        ↓
Phase 3
Media Analyzer
        ↓
Phase 4
Task Manager
        ↓
Phase 5
Simple Encoder
        ↓
Phase 6
Validation
        ↓
Phase 7
Professional Encoder
        ↓
Phase 8
Media Preservation
        ↓
Phase 9
History / Logs
        ↓
Phase 10
Account / License
        ↓
Phase 11
Installer
        ↓
Phase 12
QA
        ↓
Release Candidate
        ↓
V1.0 Stable
```

---

# 139. 当前开发起点

**从 Phase 0 开始。**

第一优先级不是设计漂亮的页面，而是验证：

```text
Wails 2.15.0
+
Windows 10 Build 19041
+
WebView2
+
FFmpeg
+
FFprobe
+
H.264
+
H.265
+
NVENC
+
QSV
+
AMF
+
AAC
+
MP4
+
MKV
```

然后建立完整的：

```text
Technical Validation Matrix
```

只有核心媒体链路验证通过后，再进入正式 UI 与任务系统开发。

---

# 140. 给下一位 Agent 的一句话

> **VideoLite 的产品需求已经冻结。不要重新设计产品，也不要重新采访需求；从 Phase 0 技术验证开始，用 Wails 2.15.0 + Vue 3 + TypeScript + Go + FFmpeg/FFprobe 建立第一个“导入 → 分析 → 编码 → 验证 → 历史”的完整闭环，然后逐阶段实现 Professional、媒体保留、账号授权、安装程序和 Release。**