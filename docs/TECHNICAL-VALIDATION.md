# VideoDelite Phase 0 技术验证矩阵

> 本文档由 `go run ./tools/valmatrix` 于 2026-10-03 11:34:54 实测生成，结果来自真实编码运行。

- FFmpeg: `D:\scoop\shims\ffmpeg.exe`
- FFprobe: `D:\scoop\shims\ffprobe.exe`
- 测试源: lavfi testsrc2 1920x1080@30 4s + sine 音频

| Feature | CPU | NVENC | QSV | AMF |
|---|---|---|---|---|
| H264 MP4+MKV | PASS | PASS | FAIL — Input #0, lavfi, from 'testsrc2=size=1920x1080:rate=30:duration=4': | FAIL — Input #0, lavfi, from 'testsrc2=size=1920x1080:rate=30:duration=4': |
| H265 MP4+MKV | PASS | PASS | FAIL — Input #0, lavfi, from 'testsrc2=size=1920x1080:rate=30:duration=4': | FAIL — Input #0, lavfi, from 'testsrc2=size=1920x1080:rate=30:duration=4': |
| 10-bit HEVC | PASS | PASS | FAIL — Input #0, lavfi, from 'testsrc2=size=1920x1080:rate=30:duration=4': | FAIL — Input #0, lavfi, from 'testsrc2=size=1920x1080:rate=30:duration=4': |
| HDR10 signaling | PARTIAL — HDR 信号未写入输出 | PARTIAL — HDR 信号未写入输出 | FAIL — Input #0, lavfi, from 'testsrc2=size=1920x1080:rate=30:duration=4': | FAIL — Input #0, lavfi, from 'testsrc2=size=1920x1080:rate=30:duration=4': |
| AAC 128k | PASS | PASS | PASS | PASS |

## 说明

- PASS = 编码成功且 FFprobe 验证通过；PARTIAL = 编码成功但部分特性丢失（已注明）；FAIL = 编码失败（真实错误信息）。
- 本机无该厂商 GPU 时 FAIL/错误信息属预期结果（计划书 §97：Detect + Disable + Reason）。
- HDR10 signaling 仅验证 smpte2084 基础标记写入；动态元数据（SMPTE ST 2086）保留为 V1 已知限制，输出验证阶段会给出警告。
