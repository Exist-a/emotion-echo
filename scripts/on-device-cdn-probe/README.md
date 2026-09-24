# scripts/on-device-cdn-probe/

CDN 候选可达性探测（Lane O · T2#2）。

## 文件

- `probe.sh` —— bash 探测脚本（5 候选 CDN HEAD + CORS + wasm magic byte）
- `test_probe.sh` —— bash 契约测试（**TDD 节奏**：先 RED 后 GREEN，16/16 PASS）
- `probe_report.md` —— 实跑产出（自动生成；本次运行 PASS=3 FAIL=0 SKIP=8）

## 跑法

```bash
# 实跑 + 写报告
bash scripts/on-device-cdn-probe/probe.sh

# CI 确定性兜底（无网络时仍 0 退出）
bash scripts/on-device-cdn-probe/probe.sh --dry-run

# 单 host 探测
bash scripts/on-device-cdn-probe/probe.sh --host aliyuncs.com

# 自定义输出文件
bash scripts/on-device-cdn-probe/probe.sh --output /tmp/report.md

# 契约测试
bash scripts/on-device-cdn-probe/test_probe.sh
```

## 退出码契约

- `0` = 全部 PASS 或 SKIP（无 FAIL）
- `1` = 任意 FAIL

## SKIP 纪律（不静默）

- 探测任意步骤 timeout / conn-refused → 显式 SKIP
- `--dry-run` 模式下所有探测 SKIP
- 与 `check_required_checks.py:24-26` 纪律一致

## 设计依据

- 调研：见 `docs/plans/on-device-cdn-probe-report-2026-09-24.md`
- 设计层：见 `docs/plans/on-device-compile-cdn-2026-09-24.md` §三
- 退出码模式：与 `scripts/smoke_data_layer.py` 一致
- HEAD 探测模板：`scripts/smoke_upload_minio.sh:71`
- skip pattern：`scripts/smoke_data_layer.py:48-50`