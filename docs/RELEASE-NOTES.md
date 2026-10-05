# VideoDelite 更新说明

## v1.0.0-mvp（2026-10-03）

首个 MVP 版本。简单、快速、本地优先的视频压缩工具。

### 新功能
- **Simple 模式**：H.264 / H.265 × 低/中/高三档画质 × MP4 / MKV，硬件编码优先（NVIDIA NVENC 实测），失败自动回退 CPU
- **Professional 模式**：手动选择编码器（NVENC/QSV/AMF/CPU）、CBR / VBR / 质量模式（0–51）、AAC 64–512 kbps、音轨/字幕/章节/Metadata 保留开关
- **任务系统**：单并发队列、实时进度（速度/码率/剩余时间）、线程级暂停/恢复、取消、重试、文件冲突策略（自动重命名/覆盖/跳过）
- **输出验证**：每个输出强制通过 FFprobe 校验（容器/编码/分辨率/时长/音轨），退出码 0 不等于成功
- **任务历史**：本地 SQLite 持久化，压缩率/耗时/状态，重启保留
- **账号系统**（可选）：注册（Resend 邮箱验证码）/ 登录 / 设备激活 / Professional 授权 / 离线宽限
- **本地日志**：7 天保留 / 500MB 上限 / 单任务 20MB 上限，ZIP 导出（路径脱敏）
- **中英双语**（首启跟随 Windows）、浅色/深色主题（跟随系统可覆盖）
- **安装程序**：WebView2 自动检测、FFmpeg/FFprobe 分发、升级覆盖安装、卸载默认保留用户数据

### 已知限制
- HDR10 动态元数据保留为尽力而为（基础色彩标记已验证，输出验证会给警告）
- 未做 Authenticode 正式签名（测试自签名），首次运行 SmartScreen 会提示
- QSV / AMF 需要对应厂商 GPU，不可用时界面自动禁用并显示原因

### 隐私
所有视频处理完全在本机完成。视频文件、文件名、路径与压缩参数永不上传。

---

## 发布检查清单（内部）
- [ ] 更新 wails.json productVersion 与 package.ps1 $version
- [ ] `powershell -File build\package.ps1`
- [ ] 正式证书签名：`build\sign.ps1`（替换为 OV/EV 证书 + signtool 时间戳）
- [ ] Windows 10 (19041) / Windows 11 实机安装验证
- [ ] 更新本文件
