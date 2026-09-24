"""性能基线测量骨架（Lane O · 端侧化阶段一 · T1）。

本骨架只提供：
- PerfMeasurement dataclass（schema 契约 + 字段校验）
- aggregate_measurements（mean / median / p95 / min / max）
- check_thresholds（v0.2 §八 8.1 + §8.3 派生阈值）
- compare_local_vs_cloud（p95 对比 + ratio + verdict）
- render_report（markdown 报告，含阈值断言 + 对比表）

**真机测量**（TTFT / tokens/sec / vram_required / model_load_ms）由 T2/T3 在
容器 web 服务内嵌 `/demo/local-llm` 路由 + IAB 实测后注入 PerfMeasurement 列表，
**不在本骨架内**（协议 §四 T1 零 dev mode）。

阈值来源：
- v0.2 §八 8.1 性能预期（桌面端 200-400ms，移动端 800-1500ms）
- v0.2 §八 8.3 核心业务指标（首 token ≤500ms）
- v0.2 §九 风险（首载等待 < 2 分钟）

跨平台统计：p95 用最简实现（按值排序后取 ceil(0.95*n) - 1 位置）。
真实场景建议用 numpy.percentile，本骨架零依赖。
"""
from __future__ import annotations

import math
from dataclasses import dataclass, field
from typing import Any, Iterable

# ---------- 常量 ----------

DEVICE_CLASSES = ("desktop_dgpu", "desktop_igpu", "mobile")

METRIC_LOAD_MS = "model_load_ms"
METRIC_TTFT_MS = "ttft_ms"
METRIC_TOKENS_PER_SEC = "tokens_per_sec"
METRIC_VRAM_MB = "vram_mb"

_METRICS = {METRIC_LOAD_MS, METRIC_TTFT_MS, METRIC_TOKENS_PER_SEC, METRIC_VRAM_MB}

# ---------- schema 契约 ----------


@dataclass(frozen=True)
class PerfMeasurement:
    model_id: str
    metric: str
    value: float
    device_class: str
    timestamp_iso: str
    note: str = ""

    def __post_init__(self) -> None:
        if self.value < 0:
            raise ValueError(f"value must be >= 0, got {self.value}")
        if self.device_class not in DEVICE_CLASSES:
            raise ValueError(
                f"device_class must be one of {DEVICE_CLASSES}, got {self.device_class!r}"
            )
        if self.metric not in _METRICS:
            raise ValueError(f"metric must be one of {sorted(_METRICS)}, got {self.metric!r}")


# ---------- 聚合函数 ----------


def _percentile(values: list[float], p: float) -> float:
    """零依赖 p95：排序后取 ceil(p*n) - 1 位置。n=0 返回 0.0。"""
    n = len(values)
    if n == 0:
        return 0.0
    if n == 1:
        return values[0]
    s = sorted(values)
    idx = max(0, min(n - 1, math.ceil(p * n) - 1))
    return s[idx]


def aggregate_measurements(
    records: Iterable[PerfMeasurement], metric: str
) -> dict[str, float]:
    """按 metric 过滤后聚合：n / mean / median / p95 / min / max。"""
    values = [r.value for r in records if r.metric == metric]
    if not values:
        return {"n": 0, "mean": 0.0, "median": 0.0, "p95": 0.0, "min": 0.0, "max": 0.0}
    return {
        "n": len(values),
        "mean": round(sum(values) / len(values), 4),
        "median": _percentile(values, 0.5),
        "p95": _percentile(values, 0.95),
        "min": min(values),
        "max": max(values),
    }


# ---------- 阈值断言 ----------


# 桌面端独显/集显 + 移动端的分层阈值（v0.2 §八 8.1 + §8.3）
_THRESHOLDS: dict[str, dict[str, dict[str, float]]] = {
    METRIC_TTFT_MS: {
        "desktop_dgpu": {"max": 500.0},
        "desktop_igpu": {"max": 800.0},
        "mobile": {"max": 1500.0},
    },
    METRIC_TOKENS_PER_SEC: {
        "desktop_dgpu": {"min": 12.0},
        "desktop_igpu": {"min": 12.0},
        "mobile": {"min": 8.0},
    },
    METRIC_VRAM_MB: {
        "desktop_dgpu": {"max": 12000.0},
        "desktop_igpu": {"max": 6000.0},
        "mobile": {"max": 4000.0},
    },
    METRIC_LOAD_MS: {
        "desktop_dgpu": {"max": 120_000.0},
        "desktop_igpu": {"max": 120_000.0},
        "mobile": {"max": 180_000.0},
    },
}


def check_thresholds(record: PerfMeasurement) -> list[str]:
    """返回违规项列表；空列表 = 通过。"""
    violations: list[str] = []
    rules = _THRESHOLDS.get(record.metric, {}).get(record.device_class, {})
    if "max" in rules and record.value > rules["max"]:
        violations.append(
            f"{record.metric}_over_max:{record.value}>{rules['max']} ({record.device_class})"
        )
    if "min" in rules and record.value < rules["min"]:
        violations.append(
            f"{record.metric}_below_min:{record.value}<{rules['min']} ({record.device_class})"
        )
    return violations


# ---------- 对比函数 ----------


def compare_local_vs_cloud(
    local: Iterable[PerfMeasurement],
    cloud: Iterable[PerfMeasurement],
    metric: str,
) -> dict[str, Any]:
    """local vs cloud p95 对比 + ratio + verdict。

    ratio_local_over_cloud 含义：
    - 越小越好（ttft / load_ms / vram）
    - 越大越好（tokens_per_sec）

    verdict：
    - "local_faster" / "cloud_faster" / "tie" / "N/A"
    """
    local_summary = aggregate_measurements(local, metric)
    cloud_summary = aggregate_measurements(cloud, metric)
    if local_summary["n"] == 0 or cloud_summary["n"] == 0:
        return {
            "metric": metric,
            "local_p95": local_summary["p95"],
            "cloud_p95": cloud_summary["p95"],
            "ratio_local_over_cloud": 0.0,
            "verdict": "N/A",
        }
    ratio = round(local_summary["p95"] / cloud_summary["p95"], 4)
    # tokens/sec 反向：cloud/local 表示"云端需要多少倍算力才能追上本地"
    if metric == METRIC_TOKENS_PER_SEC:
        ratio = round(cloud_summary["p95"] / local_summary["p95"], 4)
        verdict = "local_faster" if local_summary["p95"] > cloud_summary["p95"] else (
            "cloud_faster" if cloud_summary["p95"] > local_summary["p95"] else "tie"
        )
    else:
        verdict = (
            "local_faster" if local_summary["p95"] < cloud_summary["p95"] else (
                "cloud_faster" if cloud_summary["p95"] < local_summary["p95"] else "tie"
            )
        )
    return {
        "metric": metric,
        "local_p95": local_summary["p95"],
        "cloud_p95": cloud_summary["p95"],
        "ratio_local_over_cloud": ratio,
        "verdict": verdict,
    }


# ---------- 报告渲染 ----------


def render_report(
    records: Iterable[PerfMeasurement],
    comparisons: dict[str, dict[str, Any]] | None = None,
) -> str:
    """渲染 markdown 性能基线报告。

    sections:
    - 概览（按 metric × device 分组聚合）
    - 阈值断言（违规项列出）
    - 对比（如传入 comparisons）

    真机测量记录由 T2/T3 注入（协议 §四）；本骨架仅纯函数契约。
    """
    records = list(records)
    lines: list[str] = ["# Performance Baseline Report", ""]

    # 概览
    lines.append("## 概览（按 metric × device 聚合）")
    lines.append("")
    lines.append("| metric | device | n | mean | median | p95 | min | max |")
    lines.append("|--------|--------|---|------|--------|-----|-----|-----|")
    grouped: dict[tuple[str, str], list[PerfMeasurement]] = {}
    for r in records:
        grouped.setdefault((r.metric, r.device_class), []).append(r)
    if not grouped:
        lines.append("| (无记录) | | 0 | | | | | |")
    else:
        for (metric, device), group in sorted(grouped.items()):
            s = aggregate_measurements(group, metric)
            lines.append(
                f"| {metric} | {device} | {s['n']} | {s['mean']} | {s['median']} | {s['p95']} | {s['min']} | {s['max']} |"
            )
    lines.append("")

    # 阈值断言
    lines.append("## 阈值断言（v0.2 §八 8.1 + §8.3 派生）")
    lines.append("")
    any_violation = False
    for r in records:
        violations = check_thresholds(r)
        if violations:
            any_violation = True
            lines.append(
                f"- ❌ {r.model_id} {r.metric}={r.value} ({r.device_class}) → {'; '.join(violations)}"
            )
    if not any_violation:
        lines.append("- ✅ 全部记录通过阈值")
    lines.append("")

    # 对比
    if comparisons:
        lines.append("## 对比（local vs cloud）")
        lines.append("")
        lines.append("| metric | local_p95 | cloud_p95 | ratio_local_over_cloud | verdict |")
        lines.append("|--------|-----------|-----------|------------------------|---------|")
        for cmp_key, cmp in comparisons.items():
            if cmp_key.startswith("rich_"):
                continue
            lines.append(
                f"| {cmp.get('metric', cmp_key)} | {cmp.get('local_p95', 0)} | {cmp.get('cloud_p95', 0)} | {cmp.get('ratio_local_over_cloud', 0)} | {cmp.get('verdict', 'N/A')} |"
            )
        lines.append("")

    lines.append("---")
    lines.append("")
    lines.append("**骨架完成于 Lane O T1；真机测量记录由 T2/T3 注入（协议 §四）。**")

    return "\n".join(lines)