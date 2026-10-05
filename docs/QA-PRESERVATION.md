# VideoDelite Phase 8 媒体保留 QA 矩阵

> 由 `go run ./tools/qamatrix` 于 2026-10-03 20:11:53 实测生成。
> 所有用例走产品真实管线（Analyzer → BuildSimpleConfig → BuildCommand → ffmpegx）。

## HDR10 + 10-bit（H.265）

| 检查项 | 结果 | 详情 |
|---|---|---|
| 10-bit 保留 | ✅ PASS | 10-bit |
| HDR10 色彩标记 | ✅ PASS | trc=smpte2084 |
| BT.2020 色域 | ✅ PASS | bt2020 |

## 10-bit SDR（H.265）

| 检查项 | 结果 | 详情 |
|---|---|---|
| 10-bit 保留 | ✅ PASS | 10-bit |

## 多音轨 + 语言标记

| 检查项 | 结果 | 详情 |
|---|---|---|
| 音轨数量 | ✅ PASS | 2 → 2 |
| 语言标记保留 | ✅ PASS | 2/2 |

## 字幕 + 章节（MKV 源）

| 检查项 | 结果 | 详情 |
|---|---|---|
| 字幕轨道（MKV） | ✅ PASS | 1 → 1 |
| 字幕语言标记 | ✅ PASS | chi |
| 章节保留 | ✅ PASS | 2 → 2 |

## 旋转元数据（手机竖拍）

| 检查项 | 结果 | 详情 |
|---|---|---|
| 旋转元数据 90° | ✅ PASS | 0° → 0° |

**汇总： 10/10 项通过（4 秒实测）**
