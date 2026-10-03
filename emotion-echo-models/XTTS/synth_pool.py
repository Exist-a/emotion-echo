"""synth_pool — XTTS 推理卸载线程池（E2E-28 C4 / D-43 / F-136 落地）。

根因（E2E-28 #11 实测 + server.py 回读，2026-10-03）：
    async def handler 在事件循环里直调 tts_model.synthesize()（CPU 推理
    13~30s）⇒ 整个事件循环锁死 ⇒ 并发请求严格串行（排队 ≈ 前序请求时长）。
    账本 F-136 原记"uvicorn 单 worker"是表象，事件循环阻塞才是根。

修法取舍（ADR-2026-10 决策 39）：
    - 线程池卸载（本模块）：torch CPU 算子释放 GIL，两个推理可真并行；
      **零额外内存**。
    - 进程级多 worker：每 worker ~2.5GiB 模型常驻，.wslconfig memory=8GB
      + 19 容器稳态 ~6G，扩容即击穿（2026-09-22 Docker 冻结三连实案），
      否决。

用法（server.py）：
    outputs = await run_synth(tts_model.synthesize, text, cfg, ...)

池容量：env XTTS_SYNTH_WORKERS（默认 2）。上限语义 = 同时进行的推理数；
设 1 退化为串行（与修前等价，可用于回滚）。
"""

from __future__ import annotations

import asyncio
import os
from concurrent.futures import ThreadPoolExecutor
from functools import partial
from typing import Any, Callable, TypeVar

T = TypeVar("T")

_DEFAULT_WORKERS = 2
_pools: dict[int, ThreadPoolExecutor] = {}


def parse_workers(raw: str | None) -> int:
    """解析池容量 env；非法/缺省一律回退默认 2（不许 0/负数炸池）。"""
    if raw is None:
        return _DEFAULT_WORKERS
    try:
        n = int(str(raw).strip())
    except (TypeError, ValueError):
        return _DEFAULT_WORKERS
    return n if n >= 1 else _DEFAULT_WORKERS


def _get_pool(workers: int) -> ThreadPoolExecutor:
    pool = _pools.get(workers)
    if pool is None:
        pool = ThreadPoolExecutor(max_workers=workers, thread_name_prefix="xtts-synth")
        _pools[workers] = pool
    return pool


async def run_synth(fn: Callable[..., T], *args: Any,
                    workers: int | None = None, **kwargs: Any) -> T:
    """把阻塞推理丢进线程池执行，await 返回结果；异常原样透传。

    workers=None → 按 env XTTS_SYNTH_WORKERS（默认 2）取共享池；
    指定 workers → 取对应容量的池（测试用 1 验证串行语义）。
    """
    size = parse_workers(os.environ.get("XTTS_SYNTH_WORKERS")) if workers is None else workers
    loop = asyncio.get_running_loop()
    return await loop.run_in_executor(_get_pool(size), partial(fn, *args, **kwargs))
