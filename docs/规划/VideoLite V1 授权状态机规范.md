
# VideoLite V1 授权状态机规范

## 1. 授权状态机目标

VideoLite 的授权系统只负责判断：

> **当前用户是否有权限使用某项功能。**

授权系统不参与视频压缩本身。

必须严格遵循：

```text
Account / License
       ↓
Authorization State
       ↓
Application Feature Gate
```

而视频处理完全独立：

```text
TaskManager
    ↓
MediaAnalyzer / FFprobe
    ↓
EncoderSelector
    ↓
FFmpeg
    ↓
Output Validation
```

**FFmpeg、FFprobe、TaskManager 不得依赖 Account Service。**

---

# 2. 核心概念

V1 将以下概念严格区分：

| 概念 | 含义 |
|---|---|
| 未登录 | 当前没有有效账户会话 |
| 已登录 | 当前账户身份认证成功 |
| 设备已激活 | 当前安装实例已经与账户建立授权关系 |
| 授权有效 | 当前安装实例具备 Professional 使用权 |
| 授权过期 | 本地授权状态已经超过允许使用期限 |
| 授权撤销 | 服务端明确撤销当前授权 |
| 网络不可用 | 当前无法访问授权服务器 |
| 本地授权有效 | 虽然离线，但之前获得的本地授权仍有效 |
| 锁定 | Professional 功能不可使用 |
| 任务运行中 | FFmpeg 已经开始实际处理 |

其中：

**“登录状态” ≠ “Professional 授权状态” ≠ “网络状态”。**

---

# 3. 授权状态

V1 推荐采用以下主状态：

```text
UNAUTHENTICATED
AUTHENTICATED
ACTIVATING
AUTHORIZED
OFFLINE_AUTHORIZED
AUTHORIZATION_EXPIRED
REVOKED
ACCOUNT_DISABLED
```

另外将网络状态、FFmpeg任务状态作为独立状态，不与授权状态强行合并。

---

# 4. 状态定义

## 4.1 UNAUTHENTICATED

含义：

```text
当前没有有效登录会话
```

允许：

- Simple Mode
- 本地视频读取
- FFprobe
- FFmpeg
- H.264
- H.265
- 硬件编码
- CPU 编码
- AAC
- MP4/MKV
- Task Queue
- History
- Settings
- Logs

不允许：

- Professional Mode

UI：

```text
未登录
[登录]
```

---

# 5. AUTHENTICATED

表示：

```text
账户身份已经通过认证
```

但：

> **AUTHENTICATED 不代表已经拥有 Professional 授权。**

登录成功后，需要继续检查：

```text
账户状态
    ↓
设备状态
    ↓
License
    ↓
Authorization
```

如果账户具备 Professional 权限，则进入：

```text
AUTHORIZED
```

否则保持：

```text
AUTHENTICATED
```

---

# 6. ACTIVATING

首次 Professional 激活时进入：

```text
AUTHENTICATED
      ↓
ACTIVATING
```

执行：

```text
获取 Installation ID
        ↓
提交设备激活请求
        ↓
服务器验证账户
        ↓
服务器验证 License
        ↓
创建/确认 Device
        ↓
生成授权结果
        ↓
保存本地 Authorization State
```

成功：

```text
ACTIVATING
    ↓
AUTHORIZED
```

失败：

```text
ACTIVATING
    ↓
AUTHENTICATED
```

不能因为激活失败而影响 Simple Mode。

---

# 7. AUTHORIZED

这是正常的 Professional 授权状态。

表示：

```text
账户有效
+
设备已激活
+
Professional 授权有效
+
本地授权状态有效
```

允许：

- Simple Mode
- Professional Mode
- H.264
- H.265
- CPU
- NVENC
- QSV
- AMF/VCN
- CBR
- VBR
- CQ/Quality
- Professional 音频设置
- 专业媒体保留功能

最重要的一点：

> **AUTHORIZED 状态下，日常视频压缩不需要访问服务器。**

例如：

```text
用户点击开始
      ↓
读取本地 Authorization State
      ↓
确认 Professional 可用
      ↓
TaskManager
      ↓
FFmpeg
```

不能变成：

```text
点击开始
 ↓
HTTP 请求服务器
 ↓
等待授权
 ↓
FFmpeg
```

---

# 8. OFFLINE_AUTHORIZED

当用户已经激活过 Professional，并且本地授权仍然有效，但当前没有网络时：

```text
AUTHORIZED
     ↓
网络断开
     ↓
OFFLINE_AUTHORIZED
```

允许：

- Professional Mode
- 本地 FFmpeg
- 所有已经授权的 Professional 功能
- 新建任务
- 执行任务
- 暂停
- 恢复
- 取消
- 压缩完成
- 输出验证
- 写入 History

不允许：

- 强制要求重新登录
- 强制重新激活
- 因网络断开停止 FFmpeg

UI 可以提示：

```text
已离线
Professional 授权仍有效
```

但不能弹窗阻塞用户正常压缩。

---

# 9. 网络恢复

当：

```text
OFFLINE_AUTHORIZED
        ↓
网络恢复
```

不需要立即打断用户。

可以在后台进行授权状态同步：

```text
OFFLINE_AUTHORIZED
        ↓
网络恢复
        ↓
后台检查授权
        ↓
AUTHORIZED
```

如果同步失败：

```text
OFFLINE_AUTHORIZED
        ↓
同步失败
        ↓
继续 OFFLINE_AUTHORIZED
```

不得因为一次 HTTP 请求失败就锁死 Professional。

---

# 10. AUTHORIZATION_EXPIRED

如果本地授权明确存在有效期，并且已经超过有效期：

```text
AUTHORIZED / OFFLINE_AUTHORIZED
              ↓
        授权有效期结束
              ↓
AUTHORIZATION_EXPIRED
```

此时：

允许：

- Simple Mode
- 已完成任务查看
- History
- Settings
- Logs
- 本地数据访问

禁止：

- 新建 Professional 编码任务

已有正在运行的 FFmpeg：

> **默认继续执行到完成。**

不能：

```text
授权刚过期
    ↓
kill FFmpeg
```

正确：

```text
授权过期
    ↓
锁定新的 Professional 任务
    ↓
已有任务继续
    ↓
任务完成
    ↓
更新授权状态
```

---

# 11. REVOKED

服务端明确撤销授权：

```text
AUTHORIZED
    ↓
服务器确认授权已撤销
    ↓
REVOKED
```

例如：

- 管理员撤销 License
- 设备授权被撤销
- 账户授权被取消

行为：

### 新任务

Professional：

```text
禁止启动
```

Simple：

```text
正常使用
```

### 正在运行的任务

必须：

```text
继续运行
```

不得主动：

```text
kill FFmpeg
```

任务完成后：

```text
History 正常记录
```

之后 Professional 功能进入锁定状态。

---

# 12. ACCOUNT_DISABLED

账户被服务端禁用：

```text
正常账户
   ↓
ACCOUNT_DISABLED
```

行为：

Professional：

```text
禁止新的 Professional 任务
```

Simple：

```text
继续可用
```

正在运行：

```text
允许当前任务完成
```

不能通过账户禁用事件直接杀掉 FFmpeg。

---

# 13. 授权状态与网络状态必须分离

这是 V1 的重要架构要求。

不能设计成：

```text
Online = Authorized
Offline = Unauthorized
```

这是错误的。

应该是两个独立维度：

### Authorization

```text
UNAUTHENTICATED
AUTHENTICATED
AUTHORIZED
EXPIRED
REVOKED
DISABLED
```

### Network

```text
ONLINE
OFFLINE
UNKNOWN
```

因此可以存在：

```text
AUTHORIZED + ONLINE
AUTHORIZED + OFFLINE
EXPIRED + ONLINE
EXPIRED + OFFLINE
REVOKED + ONLINE
REVOKED + OFFLINE
```

---

# 14. Professional 功能门控

Professional 功能是否可用，只判断：

```text
Authorization State
```

而不是直接判断：

```text
Network State
```

推荐：

```text
canUseProfessional =
    authorizationState == AUTHORIZED
    ||
    authorizationState == OFFLINE_AUTHORIZED
```

Simple Mode：

```text
canUseSimple = true
```

---

# 15. FFmpeg 与授权状态的边界

FFmpeg 启动前：

```text
UI
 ↓
Application Service
 ↓
检查 Feature Authorization
 ↓
通过
 ↓
TaskManager
 ↓
FFmpeg
```

FFmpeg 启动以后：

> **授权状态变化不能直接控制 FFmpeg 生命周期。**

也就是说：

```text
FFmpeg Running
      │
      ├── 网络断开       → 不影响
      ├── Token 过期     → 不影响
      ├── Session 过期   → 不影响
      ├── License 撤销   → 不影响当前任务
      └── Server 故障    → 不影响
```

当前任务只受：

```text
用户取消
FFmpeg 自身错误
输入文件错误
输出文件错误
磁盘空间
系统错误
```

等实际任务条件影响。

---

# 16. Token 生命周期

Token 只用于：

```text
Account API
License API
Device API
Security API
```

Token：

> **不是 FFmpeg 的依赖。**

例如：

```text
Token expired
```

不能导致：

```text
Running FFmpeg
      ↓
kill
```

正确：

```text
Token expired
      ↓
后台重新认证 / 要求重新登录
      ↓
影响未来的 Account API
      ↓
当前 FFmpeg 继续
```

---

# 17. 安全凭据存储

Windows 本地：

- Access Token
- Refresh Token
- License Secret
- 其他敏感凭据

不得明文保存。

优先：

```text
Windows Credential Manager
```

或：

```text
Windows DPAPI
```

SQLite 可以保存：

- 非敏感授权状态
- 授权时间
- 授权类型
- Installation ID
- 最后同步时间

但敏感 Token 不直接明文写入 SQLite。

---

# 18. Installation ID

首次安装生成随机：

```text
Installation ID
```

例如：

```text
UUID / cryptographically random identifier
```

禁止使用：

- CPU ID
- GPU ID
- MAC 地址
- 硬盘序列号
- 主板序列号
- Windows Product ID
- 硬件指纹组合

作为设备唯一标识。

---

# 19. 本地授权缓存

Professional 激活成功后，本地保存：

```text
AuthorizationState
InstallationID
AccountID
LicenseType
AuthorizedAt
LastAuthorizedAt
ExpiresAt（如果存在）
LastSyncAt
```

本地授权缓存用于：

```text
离线启动
离线 Professional 编码
网络断开情况下继续使用
```

服务器不可用：

```text
只要本地授权仍然有效
→ 不阻塞 Professional
```

---

# 20. 服务端授权状态同步

建议使用：

```text
Login
Device Activation
App Startup
User Manually Refresh
Network Recovery
```

等事件触发授权同步。

但：

> **授权同步属于后台服务行为，不得进入 FFmpeg 编码链路。**

错误架构：

```text
TaskManager
 ↓
Authorization API
 ↓
HTTP
 ↓
FFmpeg
```

正确架构：

```text
                 ┌── Account API
                 │
AccountService ──┼── License API
                 │
                 └── Device API
                       ↓
                Authorization State
                       ↓
                  Application
                       ↓
                 Feature Gate


TaskManager ──→ FFmpeg
```

两个系统相互独立。

---

# 21. 启动时状态判断

应用启动：

```text
启动
 ↓
读取本地授权状态
 ↓
读取本地登录状态
 ↓
检查本地授权有效性
```

### 情况 A

```text
没有登录
```

进入：

```text
UNAUTHENTICATED
```

Simple 正常。

---

### 情况 B

```text
已登录
+
本地授权有效
+
网络可用
```

进入：

```text
AUTHORIZED
```

---

### 情况 C

```text
已登录
+
本地授权有效
+
网络不可用
```

进入：

```text
OFFLINE_AUTHORIZED
```

Professional 正常可用。

---

### 情况 D

```text
已登录
+
本地授权已过期
```

进入：

```text
AUTHORIZATION_EXPIRED
```

Professional 锁定。

---

### 情况 E

```text
授权已撤销
```

进入：

```text
REVOKED
```

Professional 锁定。

---

# 22. 退出登录

用户点击：

```text
退出登录
```

行为：

```text
Authenticated Session
        ↓
Logout
        ↓
UNAUTHENTICATED
```

Professional：

```text
锁定
```

Simple：

```text
继续正常使用
```

注意：

> 退出登录不删除本地视频、输出文件、History、Settings。

也不删除正在运行的 FFmpeg。

如果用户退出登录时存在任务：

```text
Running Task
    ↓
继续运行
```

---

# 23. 账号删除

用户删除账户：

服务端删除：

- Account
- Email
- 账户个人数据
- 可删除的设备关联
- License 个人关联

保留必要的最小安全记录时，应遵循适用的数据保留要求。

本地：

**绝对不能自动删除：**

- 原视频
- 输出视频
- History
- Settings
- Logs

除非用户明确选择：

```text
同时清除本地数据
```

---

# 24. 卸载

正常卸载：

```text
删除程序文件
```

保留：

```text
用户视频
输出视频
History
Settings
Logs
```

可以提供：

```text
卸载时删除本地用户数据
```

但必须明确告知用户。

---

# 25. 状态转换图

核心状态机：

```text
                  ┌──────────────────┐
                  │ UNAUTHENTICATED  │
                  └────────┬─────────┘
                           │ Login
                           ▼
                  ┌──────────────────┐
                  │ AUTHENTICATED    │
                  └────────┬─────────┘
                           │ Activate
                           ▼
                  ┌──────────────────┐
                  │   ACTIVATING     │
                  └───────┬──────────┘
                          │
                 Success  │  Failure
                          │
                          ▼
                 ┌──────────────────┐
                 │    AUTHORIZED    │
                 └───────┬──────────┘
                         │
              Network    │
              unavailable│
                         ▼
                 ┌──────────────────┐
                 │ OFFLINE_AUTHORIZED│
                 └───────┬──────────┘
                         │
                  Network│recovery
                         ▼
                 ┌──────────────────┐
                 │    AUTHORIZED    │
                 └──────────────────┘


AUTHORIZED / OFFLINE_AUTHORIZED
              │
              │ expiry
              ▼
     ┌─────────────────────┐
     │ AUTHORIZATION_EXPIRED│
     └─────────────────────┘


AUTHORIZED / OFFLINE_AUTHORIZED
              │
              │ server revoke
              ▼
        ┌──────────┐
        │ REVOKED  │
        └──────────┘


AUTHORIZED / OFFLINE_AUTHORIZED
              │
              │ account disabled
              ▼
     ┌──────────────────┐
     │ ACCOUNT_DISABLED │
     └──────────────────┘
```

---

# 26. “正在编码”是独立状态机

授权状态不能直接和任务状态混合。

例如：

```text
Authorization:
AUTHORIZED

Task:
ENCODING

Network:
OFFLINE
```

这是完全合法的状态。

也可以：

```text
Authorization:
REVOKED

Task:
ENCODING
```

这也是允许短暂存在的状态：

```text
当前任务继续
↓
完成
↓
新的 Professional 任务禁止
```

因此：

```text
AuthorizationState
TaskState
NetworkState
```

必须是三个独立维度。

---

# 27. 关键业务规则

V1 必须冻结以下规则：

### Rule 01

```text
Simple Mode 不需要登录。
```

### Rule 02

```text
Professional Mode 需要有效授权。
```

### Rule 03

```text
Professional 压缩始终在本地执行。
```

### Rule 04

```text
首次 Professional 激活需要网络。
```

### Rule 05

```text
已经获得有效本地授权后，可以离线压缩。
```

### Rule 06

```text
网络断开不能停止正在运行的 FFmpeg。
```

### Rule 07

```text
服务器故障不能阻塞已经有效授权的本地压缩。
```

### Rule 08

```text
Token 过期不能停止正在运行的 FFmpeg。
```

### Rule 09

```text
License 撤销不能强制杀死正在运行的 FFmpeg。
```

### Rule 10

```text
授权变化只影响未来能够启动的新 Professional 任务。
```

### Rule 11

```text
账户、设备、License 数据可以在服务器管理。
```

### Rule 12

```text
视频及视频处理过程永远留在本地。
```

### Rule 13

```text
FFmpeg 模块不得依赖 Account Service。
```

### Rule 14

```text
授权检查不得成为 FFmpeg 运行时依赖。
```

---

# 28. V1 验收测试

Agent 必须至少测试以下情况：

| 测试 | 预期 |
|---|---|
| 未登录启动 | Simple 可用 |
| 未登录 Professional | 锁定 |
| 登录成功 | 显示账户状态 |
| 首次激活 | 网络激活成功 |
| 激活成功后断网 | Professional 仍可用 |
| 断网后新建任务 | 可以编码 |
| 编码过程中断网 | FFmpeg 正常完成 |
| 编码过程中 Token 过期 | FFmpeg 正常完成 |
| 编码过程中服务器宕机 | FFmpeg 正常完成 |
| 服务器撤销授权 | 当前任务完成，新任务禁止 |
| 授权过期 | 新 Professional 任务禁止 |
| Simple 在授权过期后 | 正常可用 |
| 退出登录 | Professional 锁定，Simple 正常 |
| 退出登录时编码 | 当前任务继续 |
| 删除账户 | 不删除本地视频/History |
| 卸载软件 | 不删除用户视频/输出 |
| 重启电脑 | 本地授权状态正确恢复 |
| 离线启动 | 有效本地授权可以进入 Professional |
| FFmpeg 运行期间无 HTTP 请求 | 必须通过测试 |

---

# 29. 最终架构红线

VideoLite V1 的授权系统最终必须满足：

```text
                    SERVER
                       │
        ┌──────────────┼──────────────┐
        │              │              │
     Account        Device         License
        │              │              │
        └──────────────┼──────────────┘
                       ↓
               Authorization
                       ↓
                Feature Gate
                       │
             ┌─────────┴─────────┐
             │                   │
        Simple Mode       Professional Mode
             │                   │
             └─────────┬─────────┘
                       ↓
                  LOCAL ONLY
                       ↓
               TaskManager
                       ↓
                  FFmpeg
                       ↓
                Output File
                       ↓
                 FFprobe
                       ↓
                 Validation
```

**服务器负责“证明用户是谁、设备是谁、有没有权限”。**

**本机负责“读取视频、处理视频、生成视频、验证视频”。**

两条链路必须保持解耦。

尤其禁止：

```text
FFmpeg → Account API
FFmpeg → License API
FFmpeg → HTTP
FFmpeg → Token
FFmpeg → Server
```

V1 的最终原则：

> **授权可以决定“能不能开始一个新的 Professional 任务”，但绝不能决定“已经开始的本地 FFmpeg 任务能不能继续运行”。**