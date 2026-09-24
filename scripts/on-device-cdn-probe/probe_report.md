# CDN 候选可达性探测报告

**生成时间**: 2026-09-24T03:30:39Z
**协议依据**: on-device-compile-cdn-2026-09-24.md §三（5 候选 CDN） + AGENTS.md §〇.6 文档功课
**探测工具**: `probe.sh` (本目录)
**退出码契约**: 0 = 全部 PASS 或 SKIP；1 = 任意 FAIL（与 smoke_data_layer.py 一致）

## 总览

| ID | 候选 | Host | HEAD | CORS preflight | 备注 |
|----|------|------|------|----------------|------|
| `RAW` | raw.githubusercontent.com (v0.2 §4.1 wasm 默认) | `raw.githubusercontent.com` | [SKIP-DRY-RUN] N/A | [SKIP-DRY-RUN] | https://raw.githubusercontent.com/mlc-ai/binary-mlc-llm-libs/main/web-llm-models/README.md |
| `A` | Aliyun OSS (推荐) | `oss-cn-hangzhou.aliyuncs.com` | [SKIP-DRY-RUN] N/A | [SKIP-DRY-RUN] | https://oss-cn-hangzhou.aliyuncs.com/README |
| `B` | Tencent Cloud COS | `cos.ap-shanghai.myqcloud.com` | [SKIP-DRY-RUN] N/A | [SKIP-DRY-RUN] | https://cos.ap-shanghai.myqcloud.com/README |
| `E` | HuggingFace mirror (hf-mirror) | `hf-mirror.com` | [SKIP-DRY-RUN] N/A | [SKIP-DRY-RUN] | https://hf-mirror.com/README |
| `F` | ModelScope | `www.modelscope.cn` | [SKIP-DRY-RUN] N/A | [SKIP-DRY-RUN] | https://www.modelscope.cn/api/v1/models/X-D-Lab/MindChat-Qwen2-0_5B |

## 汇总

- **PASS**: 0
- **FAIL**: 0
- **SKIP**: 0

## wasm magic byte 探测（仅 Aliyun OSS 推荐 bucket）

- [SKIP-DRY-RUN] wasm magic byte 探测未执行

## 探测结论

（根据本次实测填入 —— 用于 §十二 决策 2/3 的可达性证据）

**观察**（待人工补充）:

## SKIP 纪律

- 探测任意步骤 timeout / conn-refused → 显式 SKIP（不静默；与 check_required_checks.py:24-26 一致）
- `--dry-run` 模式下所有探测 SKIP，CI 确定性兜底（退出码 0）
