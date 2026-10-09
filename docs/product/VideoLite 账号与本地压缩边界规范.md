# VideoLite 账号与本地压缩边界规范

## 1. 核心原则

VideoLite 必须严格遵循：

> **账号负责“身份、授权和服务端管理”；本地负责“视频处理和用户数据”。**

两者必须解耦。

最核心的边界：

```text
                    VideoLite
                        │
          ┌─────────────┴─────────────┐
          │                           │
      本地处理域                  账号服务域
          │                           │
   视频 / FFmpeg / FFprobe       登录 / 授权 / 设备
   编码 / 转码 / 验证             注册 / 邮箱 / 安全
   任务 / 历史 / 日志             账号管理 / 管理后台
          │                           │
          └─────────────┬─────────────┘
                        │
                   最小必要通信
```

---

# 2. 本地压缩完全属于本地

以下全部必须在用户电脑本地完成：

- 视频读取
- 视频分析
- FFprobe
- FFmpeg
- 编码
- 解码
- 硬件编码
- CPU 编码
- GPU 编码
- 音频转换
- 字幕处理
- Metadata 处理
- HDR / 10-bit 处理
- Rotation 处理
- 输出验证
- 压缩进度
- Task Queue
- Task History
- 本地日志
- 临时文件处理

服务器**不参与实际视频压缩过程**。

---

# 3. 账号服务器不能接触视频

服务器禁止接收：

```text
视频文件
视频二进制
视频截图
视频缩略图
视频文件内容
视频音频内容
视频字幕内容
```

也禁止上传：

```text
用户视频文件名
用户视频完整路径
视频 Metadata
视频 GPS
相机型号
视频创建时间
FFmpeg Command
FFprobe 完整输出
压缩参数
压缩结果
GPU 编码日志
```

除非未来经过明确的独立产品需求变更，否则永久保持这一边界。

---

# 4. 登录不等于压缩权限

账号系统不能成为：

```text
登录
 ↓
服务器批准
 ↓
服务器允许压缩
```

这种模式。

正确模式：

```text
用户启动 VideoLite
        │
        ├── 未登录
        │      ↓
        │   Simple Mode
        │      ↓
        │   本地压缩
        │
        └── 已登录
               ↓
          检查本地授权
               ↓
          Professional Mode
               ↓
            本地压缩
```

---

# 5. Simple Mode 与账号

Simple Mode：

> **不依赖账号即可使用。**

用户可以在完全不登录的情况下：

- 导入视频
- FFprobe 分析
- H.264 压缩
- H.265 压缩
- 使用 Low / Mid / High
- 使用硬件编码
- CPU fallback
- 输出 MP4 / MKV
- AAC
- 查看任务
- 查看历史
- 使用本地日志

---

# 6. Professional Mode 与账号

当前产品规则：

> **Professional Mode 需要登录并拥有有效授权。**

但必须明确：

**登录只是授权条件，不代表视频处理转移到服务器。**

正确关系：

```text
账号
 ↓
证明用户身份
 ↓
确认 Professional 授权
 ↓
生成/确认本地授权状态
 ↓
Professional 功能在本地解锁
 ↓
FFmpeg 在本地运行
```

---

# 7. 授权成功后的本地运行

授权成功后：

```text
Server
 ↓
Authorization Result
 ↓
Local License / Authorization
 ↓
VideoLite
 ↓
Local FFmpeg
```

后续实际压缩：

```text
Input Video
 ↓
Local FFprobe
 ↓
Local FFmpeg
 ↓
Local Output
```

服务器不参与。

---

# 8. 离线使用

如果用户已经成功获得 Professional 本地授权：

```text
网络断开
       ↓
仍然可以进行本地压缩
```

包括：

- Simple
- Professional
- H.264
- H.265
- CPU
- GPU
- AAC
- MP4
- MKV

均不应该因为服务器暂时不可访问而导致正在进行的本地任务停止。

---

# 9. 正在编码时网络断开

特别规定：

```text
Professional Task
        ↓
Encoding
        ↓
网络断开
```

结果：

> **不得终止正在运行的 FFmpeg。**

本地 Task Manager 不应该因为：

```text
API Timeout
Server Offline
License Server Unreachable
```

而杀掉本地 FFmpeg。

---

# 10. 启动时服务器不可用

如果用户：

```text
已经拥有本地有效授权
+
服务器暂时无法访问
```

VideoLite：

```text
启动
 ↓
读取本地授权
 ↓
进入正常使用
```

不得强制：

```text
联网
 ↓
服务器确认
 ↓
才能使用
```

---

# 11. 首次 Professional 激活

首次使用 Professional 时：

```text
Login
 ↓
Server Authentication
 ↓
Device Activation
 ↓
Authorization
 ↓
保存本地授权
 ↓
Professional Enabled
```

此过程需要网络。

但激活完成后：

> 日常视频压缩不需要服务器。

---

# 12. Device Activation

设备使用：

```text
Random Installation ID
```

而不是：

- CPU ID
- GPU ID
- MAC 地址
- 硬盘序列号
- 主板序列号
- Windows Hardware Fingerprint

服务器只知道：

```text
Account ID
Installation ID
Activation Status
Created At
Last Seen
App Version
```

不应该通过硬件指纹追踪用户。

---

# 13. 账号服务器可以知道什么

服务器允许保存：

### Account

```text
Account ID
Username
Email
Password Hash
Registration Time
Account Status
```

### Device

```text
Installation ID
Account ID
Activation Time
Last Authorization Time
App Version
Device Status
```

### Security

```text
Login Attempt
Password Reset
Email Verification
Device Activation
Account Ban
Security Event
```

### License

```text
Authorization Type
Authorization Status
Created Time
Revoked Time
```

---

# 14. 账号服务器不能知道什么

默认禁止：

```text
用户有哪些视频
用户视频叫什么
视频存在哪里
视频多大
视频是什么内容
视频是什么编码
视频压缩成了多大
用户每天压缩多少视频
用户用了什么压缩参数
用户使用什么 GPU 压缩
用户用了 NVENC 还是 QSV
用户压缩成功率
用户压缩失败原因
用户 FFmpeg Command
```

这些数据属于本地处理域。

---

# 15. 特别禁止“匿名遥测”绕过边界

不能通过：

```text
Telemetry
Analytics
Crash Report
Usage Statistics
```

偷偷上传：

```text
视频信息
压缩参数
文件路径
文件名
FFmpeg 输出
```

例如：

```text
User compressed 4K H.265
NVENC
20 Mbps
File size 8.2 GB → 2.1 GB
```

即使没有用户名：

> 也不能默认上传。

---

# 16. 崩溃日志

默认：

```text
Local Only
```

不自动上传。

用户主动点击：

```text
Export Logs
```

以后，可以获得 ZIP。

如果未来增加：

```text
Send Diagnostic Report
```

必须：

1. 明确告知用户；
2. 用户主动点击；
3. 上传前显示内容；
4. 自动隐藏敏感信息；
5. 用户确认后才能上传。

V1 不实现自动诊断上传。

---

# 17. FFmpeg Command

FFmpeg Command 属于：

> **本地技术数据。**

例如：

```text
ffmpeg -i xxx.mp4 ...
```

只能：

```text
本地执行
本地日志
本地 Debug
```

不得：

```text
自动上传服务器
```

---

# 18. FFprobe 数据

FFprobe 得到：

```text
Codec
Resolution
FPS
Bitrate
HDR
Color
Audio
Subtitle
Metadata
```

全部属于：

> 本地媒体分析数据。

不上传。

---

# 19. 本地 History

History：

```text
本地 SQLite
```

服务器不保存。

例如：

```text
Input:
D:\Video\Japan.mp4

Output:
D:\Compressed\Japan.mp4

Codec:
H.265

Encoder:
NVENC

Duration:
00:15:20

Size:
8.2 GB → 2.1 GB
```

这些信息：

> 全部只存在本地。

---

# 20. 本地 Settings

包括：

- 默认编码器
- 默认 Codec
- Quality
- Output Folder
- Conflict Strategy
- Language
- Theme
- Professional 参数

均为本地设置。

服务器不需要同步。

---

# 21. 本地账号凭据

客户端不能明文保存：

```text
Password
Refresh Token
License Secret
```

建议：

```text
Windows Credential Manager
/
DPAPI
```

保护本地敏感凭据。

---

# 22. Token

Token 只用于：

```text
Authentication
Authorization
Account API
```

不能作为：

```text
FFmpeg
Task
Video Processing
```

的输入依赖。

也就是说：

```text
Token 失效
```

不应该直接导致：

```text
正在进行的本地 FFmpeg 被杀死
```

---

# 23. License 失效

如果服务器确认：

```text
Account Banned
License Revoked
```

客户端下一次能够与服务器正常通信时：

```text
同步授权状态
```

但对于已经正在执行的本地 Task：

> 默认让当前 Task 完成。

之后：

```text
Professional
```

进入未授权状态。

这样可以避免：

```text
服务器状态变化
 ↓
用户正在压缩的视频突然损坏
```

---

# 24. 账号删除

删除账号：

```text
Server
 ↓
删除个人账号数据
```

不会删除用户电脑上的：

- 视频
- 压缩结果
- History
- FFmpeg
- Settings

但本地授权数据可以根据产品安全策略失效。

---

# 25. 卸载

卸载 VideoLite：

```text
Program Files
```

由 Installer 删除。

但默认不删除：

```text
用户视频
压缩输出
```

对于本地：

```text
History
Logs
Settings
License
```

可以提供：

```text
Remove user data
```

选项。

---

# 26. 本地压缩架构边界

推荐严格保持：

```text
┌─────────────────────────────────────┐
│             VideoLite               │
│                                     │
│  ┌───────────────────────────────┐  │
│  │       Local Processing        │  │
│  │                               │  │
│  │ FFprobe                       │  │
│  │ Media Analyzer                │  │
│  │ Task Manager                  │  │
│  │ FFmpeg                        │  │
│  │ Encoder                       │  │
│  │ Validation                    │  │
│  │ History                       │  │
│  │ Logs                          │  │
│  └───────────────────────────────┘  │
│                 │                   │
│                 │ Auth Only         │
│                 ▼                   │
│  ┌───────────────────────────────┐  │
│  │       Account Service         │  │
│  │                               │  │
│  │ Login                         │  │
│  │ Register                      │  │
│  │ Device                        │  │
│  │ License                       │  │
│  │ Security                      │  │
│  └───────────────────────────────┘  │
└─────────────────────────────────────┘
```

---

# 27. 最重要的架构原则

任何开发 Agent 在写代码时都必须遵守：

> **FFmpeg 模块不能依赖 Account Service。**

错误：

```text
FFmpegService
    ↓
LicenseService
    ↓
HTTP
    ↓
Server
```

正确：

```text
AccountService
    ↓
Authorization State
    ↓
Application State
```

以及：

```text
TaskManager
    ↓
FFmpegService
```

两者通过：

```text
Application / License State
```

进行功能权限判断，而不是让 FFmpeg 本身联网。

---

# 28. 推荐模块边界

```text
backend/
├── account/
├── license/
├── device/
│
├── task/
├── ffmpeg/
├── ffprobe/
├── encoder/
├── media/
├── validation/
├── history/
├── logging/
├── settings/
└── system/
```

其中：

```text
account/
license/
device/
```

属于：

> 账号 / 授权域。

而：

```text
task/
ffmpeg/
ffprobe/
encoder/
media/
validation/
history/
logging/
```

属于：

> 本地处理域。

---

# 29. 网络依赖原则

本地压缩路径：

```text
Input
 ↓
FFprobe
 ↓
Config
 ↓
Encoder
 ↓
FFmpeg
 ↓
Output
 ↓
FFprobe
 ↓
Validation
```

**整个路径不允许经过 HTTP。**

---

# 30. 断网验收测试

必须进行：

### 测试 A

```text
已登录
已授权
断网
 ↓
Simple Compression
```

必须成功。

### 测试 B

```text
已登录
已授权
断网
 ↓
Professional Compression
```

必须成功。

### 测试 C

```text
Professional Encoding
 ↓
网络突然断开
```

FFmpeg：

> 必须继续运行。

### 测试 D

```text
服务器完全不可用
 ↓
启动客户端
```

已有有效本地授权：

> 必须可以继续本地工作。

---

# 31. 最终一句话边界

VideoLite 的账号系统本质上是：

> **“证明你是谁，以及你有没有资格使用某些功能。”**

而不是：

> **“替你处理视频。”**

VideoLite 的视频压缩本质上是：

> **“你的电脑读取你的文件，你的电脑运行 FFmpeg，你的电脑生成输出，你的电脑验证输出。”**

服务器只负责：

```text
Identity
Authorization
Device
Security
Account
Version
```

不负责：

```text
Video Processing
Video Storage
Compression
Media Analysis
Compression Analytics
```

**这条边界作为 V1.0 架构红线冻结。**