"""record 实现：先查 replay 文件；命中即回放，未命中转发到 inner_fn 落盘。

用途：
- baseline 报告复跑（不依赖容器 / 不消耗上游 token）
- 调试：把某次真实跑分落盘后慢慢分析
- CI 加速：录播到位时 0 网络

回放文件格式（按 case.id 分文件，UTF-8 JSON）：
  {"case_id": "daily-01", "input": "...", "text": "...", "model": "...",
   "fallback_reason": "", "latency_ms": 234, "recorded_at": "2026-09-24T..."}
"""
from __future__ import annotations

import json
import logging
import os
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from model_fn_factory import BaselineModelFn, ReplyResult

logger = logging.getLogger(__name__)


def _replay_path(replay_dir: Path, case_id: str) -> Path:
    safe = case_id.replace("/", "_").replace("..", "_")
    return replay_dir / f"{safe}.json"


def make(
    *,
    replay_dir: str | Path,
    inner_fn: BaselineModelFn,
    mode: str = "read_or_write",
) -> BaselineModelFn:
    """构造一个 record model_fn。

    mode：
      - "read_or_write"（默认）：命中即回放，未命中调 inner_fn 并落盘
      - "write_only"：永远调 inner_fn 并落盘（强制刷新）
      - "read_only"：命中即回放，未命中返回 fallback_reply（**测试用**，生产不该用）
    """
    replay_path = Path(replay_dir)
    replay_path.mkdir(parents=True, exist_ok=True)

    fallback_reply = ReplyResult(
        text="",
        model="",
        fallback_reason="replay_miss_in_read_only_mode",
        latency_ms=0,
    )

    def model_fn(case: dict[str, Any]) -> ReplyResult:
        path = _replay_path(replay_path, case["id"])
        if mode != "write_only" and path.exists():
            try:
                data = json.loads(path.read_text(encoding="utf-8"))
                return ReplyResult(
                    text=data.get("text", ""),
                    model=data.get("model", ""),
                    fallback_reason=data.get("fallback_reason", ""),
                    latency_ms=int(data.get("latency_ms", 0)),
                    extras={"replay_hit": True},
                )
            except (json.JSONDecodeError, KeyError, ValueError) as e:
                logger.warning("replay file %s unreadable (%s); falling through", path, e)

        if mode == "read_only":
            return fallback_reply

        result = inner_fn(case)
        # 落盘（即使失败也写，记 fallback_reason）
        try:
            payload = {
                "case_id": case["id"],
                "input": case.get("input", ""),
                "text": result.text,
                "model": result.model,
                "fallback_reason": result.fallback_reason,
                "latency_ms": result.latency_ms,
                "recorded_at": datetime.now(timezone.utc).isoformat(),
            }
            path.write_text(json.dumps(payload, ensure_ascii=False, indent=2), encoding="utf-8")
            logger.info("recorded case=%s -> %s", case["id"], path)
        except Exception as e:
            logger.warning("record failed to persist %s: %s", path, e)

        return ReplyResult(
            text=result.text,
            model=result.model,
            fallback_reason=result.fallback_reason,
            latency_ms=result.latency_ms,
            extras={**result.extras, "replay_hit": False},
        )

    return model_fn