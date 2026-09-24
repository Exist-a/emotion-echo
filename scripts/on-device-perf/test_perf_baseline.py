"""性能基线测量脚本骨架 TDD 测试（Lane O · 端侧化阶段一）。

本会话（T1）目标 = 纯函数契约测试 + 真机测量留 T2/T3。
- 测量记录 schema（PerfMeasurement）
- 聚合函数（mean / median / p95 / 阈值断言）
- 对比函数（local vs cloud p95 / ratio）
- 报告渲染（markdown 输出）

**真机测量**（TTFT / tokens/sec / vram_required）由 T2/T3 在容器 web 服务内嵌
`/demo/local-llm` 路由 + IAB 实测后注入 PerfMeasurement 列表，**不在本骨架内**。

跑法（repo 根）：python -m pytest scripts/on-device-perf/
"""
from __future__ import annotations

import re

from perf_baseline import (
    DEVICE_CLASSES,
    METRIC_LOAD_MS,
    METRIC_TOKENS_PER_SEC,
    METRIC_TTFT_MS,
    METRIC_VRAM_MB,
    PerfMeasurement,
    aggregate_measurements,
    check_thresholds,
    compare_local_vs_cloud,
    render_report,
)


def _m(metric: str, value: float, device: str = "desktop_dgpu", model: str = "Qwen3-1.7B-q4f16_1-MLC") -> PerfMeasurement:
    """测试夹具：构造 PerfMeasurement"""
    return PerfMeasurement(
        model_id=model,
        metric=metric,
        value=value,
        device_class=device,
        timestamp_iso="2026-09-24T00:00:00Z",
        note="",
    )


# ---------- schema 契约 ----------


def test_device_classes_are_canonical():
    assert DEVICE_CLASSES == {"desktop_dgpu", "desktop_igpu", "mobile"} or set(DEVICE_CLASSES) == {
        "desktop_dgpu",
        "desktop_igpu",
        "mobile",
    }


def test_metric_constants_are_distinct_strings():
    metrics = {METRIC_LOAD_MS, METRIC_TTFT_MS, METRIC_TOKENS_PER_SEC, METRIC_VRAM_MB}
    assert len(metrics) == 4


def test_perf_measurement_required_fields():
    m = _m(METRIC_TTFT_MS, 350.0)
    assert m.model_id == "Qwen3-1.7B-q4f16_1-MLC"
    assert m.metric == METRIC_TTFT_MS
    assert m.value == 350.0
    assert m.device_class == "desktop_dgpu"


def test_perf_measurement_rejects_negative_value():
    import pytest

    with pytest.raises(ValueError):
        _m(METRIC_TTFT_MS, -1.0)


def test_perf_measurement_rejects_unknown_device_class():
    import pytest

    with pytest.raises(ValueError):
        _m(METRIC_TTFT_MS, 100.0, device="unknown_device")


# ---------- 聚合函数 ----------


def test_aggregate_measurements_basic_stats():
    records = [_m(METRIC_TTFT_MS, v) for v in [100.0, 200.0, 300.0, 400.0, 500.0]]
    summary = aggregate_measurements(records, METRIC_TTFT_MS)
    assert summary["n"] == 5
    assert summary["mean"] == 300.0
    assert summary["median"] == 300.0
    assert summary["min"] == 100.0
    assert summary["max"] == 500.0
    assert summary["p95"] >= 400.0  # p95 of [100..500] ≈ 480


def test_aggregate_measurements_filters_by_metric():
    records = [
        _m(METRIC_TTFT_MS, 100.0),
        _m(METRIC_TOKENS_PER_SEC, 30.0),
        _m(METRIC_TTFT_MS, 200.0),
    ]
    summary = aggregate_measurements(records, METRIC_TTFT_MS)
    assert summary["n"] == 2  # only TTFT_MS counted


def test_aggregate_measurements_empty_returns_zero():
    summary = aggregate_measurements([], METRIC_TTFT_MS)
    assert summary["n"] == 0
    assert summary["mean"] == 0.0


def test_aggregate_measurements_single_value_p95_equals_value():
    summary = aggregate_measurements([_m(METRIC_TTFT_MS, 250.0)], METRIC_TTFT_MS)
    assert summary["n"] == 1
    assert summary["p95"] == 250.0


# ---------- 阈值断言（v0.2 §八 8.1 + §8.3 派生）----------


def test_check_thresholds_ttft_pass():
    violations = check_thresholds(_m(METRIC_TTFT_MS, 350.0))  # ≤ 500ms 桌面端 pass
    assert violations == []


def test_check_thresholds_ttft_fail_on_desktop_dgpu():
    violations = check_thresholds(_m(METRIC_TTFT_MS, 800.0))
    assert any("ttft" in v for v in violations)


def test_check_thresholds_tokens_per_sec_pass_desktop_dgpu():
    violations = check_thresholds(_m(METRIC_TOKENS_PER_SEC, 30.0, "desktop_dgpu"))
    assert violations == []


def test_check_thresholds_tokens_per_sec_fail_mobile():
    # 移动端 < 8 tokens/s → 体验不可用
    violations = check_thresholds(_m(METRIC_TOKENS_PER_SEC, 5.0, "mobile"))
    assert any("tokens_per_sec" in v for v in violations)


def test_check_thresholds_vram_warn_mobile():
    # 移动端 vram > 4000 MB → 警告（即便 q4f16_1 low_resource 标 true，移动端仍可能爆）
    violations = check_thresholds(_m(METRIC_VRAM_MB, 4500.0, "mobile"))
    assert any("vram" in v.lower() for v in violations)


def test_check_thresholds_load_ms_pass_under_60s():
    violations = check_thresholds(_m(METRIC_LOAD_MS, 30_000.0))  # 30s OK
    assert violations == []


def test_check_thresholds_load_ms_fail_over_120s():
    """首次加载 > 2 分钟 → 用户流失（v0.2 §九 风险：首载等待）"""
    violations = check_thresholds(_m(METRIC_LOAD_MS, 150_000.0))
    assert any("load" in v.lower() for v in violations)


# ---------- 对比函数 local vs cloud ----------


def test_compare_local_vs_cloud_ratio_smaller_better():
    """TTFT: 本地 350ms vs 云端 800ms → ratio 0.4375（本地更快）"""
    local = [_m(METRIC_TTFT_MS, 350.0)]
    cloud = [_m(METRIC_TTFT_MS, 800.0)]
    cmp = compare_local_vs_cloud(local, cloud, METRIC_TTFT_MS)
    assert cmp["local_p95"] == 350.0
    assert cmp["cloud_p95"] == 800.0
    assert abs(cmp["ratio_local_over_cloud"] - 0.4375) < 0.001
    assert cmp["verdict"] == "local_faster"


def test_compare_local_vs_cloud_tokens_per_sec_larger_better():
    local = [_m(METRIC_TOKENS_PER_SEC, 30.0)]
    cloud = [_m(METRIC_TOKENS_PER_SEC, 25.0)]
    cmp = compare_local_vs_cloud(local, cloud, METRIC_TOKENS_PER_SEC)
    assert cmp["verdict"] == "local_faster"  # tokens/sec: 高 = 快


def test_compare_local_vs_cloud_empty_side_returns_na():
    cmp = compare_local_vs_cloud([], [_m(METRIC_TTFT_MS, 800.0)], METRIC_TTFT_MS)
    assert cmp["verdict"] == "N/A"


def test_compare_local_vs_cloud_uses_p95_not_mean():
    local = [_m(METRIC_TTFT_MS, v) for v in [100, 200, 1000]]  # one outlier
    cloud = [_m(METRIC_TTFT_MS, v) for v in [800, 850, 900]]
    cmp = compare_local_vs_cloud(local, cloud, METRIC_TTFT_MS)
    # p95 of [100,200,1000] ≈ 1000 (max)
    # p95 of [800,850,900] ≈ 900
    assert cmp["local_p95"] >= 950
    assert cmp["cloud_p95"] >= 850


# ---------- 报告渲染 ----------


def test_render_report_includes_threshold_section():
    records = [_m(METRIC_TTFT_MS, 350.0), _m(METRIC_TOKENS_PER_SEC, 30.0)]
    md = render_report(records)
    assert "Threshold" in md or "阈值" in md
    assert METRIC_TTFT_MS in md


def test_render_report_includes_comparison_when_provided():
    records = [_m(METRIC_TTFT_MS, 350.0)]
    cmp = {"rich_": compare_local_vs_cloud(records, [_m(METRIC_TTFT_MS, 800.0)], METRIC_TTFT_MS)}
    md = render_report(records, comparisons=cmp)
    assert "Comparison" in md or "对比" in md
    assert "local_faster" in md or "ratio" in md.lower()


def test_render_report_handles_empty_records():
    md = render_report([])
    assert md  # 至少返回非空字符串（说明骨架完整）


def test_render_report_no_python_repr_leaked():
    """防 'PerfMeasurement(...)' 字面量泄到报告（AGENTS §〇 文档功课）"""
    records = [_m(METRIC_TTFT_MS, 350.0)]
    md = render_report(records)
    assert "PerfMeasurement(" not in md
    assert "at 0x" not in md
    assert "object at" not in md.lower()