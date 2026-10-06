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
import queue
import threading
from concurrent.futures import ThreadPoolExecutor
from functools import partial
from typing import Any, Callable, TypeVar

T = TypeVar("T")

_DEFAULT_WORKERS = 2
_pools: dict[int, ThreadPoolExecutor] = {}

# F-135（2026-10-06）：模型级推理互斥。
# 根因（vendored 代码回读 + 并发实测）：inference() 与 inference_stream() 共享
# 同一个 GPT2InferenceModel 实例，且每次推理前 gpt.py:570
# store_prefix_emb(emb) 覆盖写 gpt_inference.cached_prefix_emb（gpt_inference.py:22，
# forward 每步读）⇒ 跨路径并发推理 = 数据竞争：先生成者的下一步 forward 读到
# 后到者的 prefix ⇒ get_fixed_embedding 越界（index out of range）/ tensor 尺寸
# 错位 / 提前 EOS（截断坏数据）。实测：stream+phonemes 并发双双截断
# （1.1s vs 串行基线 2.1s）+ spec 期错误簇两型齐现。
# ⇒ ADR-2026-10 决策 39 的"两个推理可真并行"前提有误（算子释放 GIL ≠ 模型
# 无共享状态）；本锁有意反转该行为。事件循环不阻塞的 F-136 原始意图不受影响
# （池结构保留，锁只保证模型访问互斥）。
INFERENCE_LOCK = threading.Lock()


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


def _locked_call(fn: Callable[..., T], *args: Any, **kwargs: Any) -> T:
    """执行体包装：模型推理持 INFERENCE_LOCK（跨 synthesize/stream 互斥）。"""
    with INFERENCE_LOCK:
        return fn(*args, **kwargs)


async def run_synth(fn: Callable[..., T], *args: Any,
                    workers: int | None = None, **kwargs: Any) -> T:
    """把阻塞推理丢进线程池执行，await 返回结果；异常原样透传。

    workers=None → 按 env XTTS_SYNTH_WORKERS（默认 2）取共享池；
    指定 workers → 取对应容量的池（测试用 1 验证串行语义）。
    F-135（2026-10-06）：执行体持 INFERENCE_LOCK —— 模型共享状态
    （cached_prefix_emb）使并发推理成为数据竞争，模型访问必须互斥。
    """
    size = parse_workers(os.environ.get("XTTS_SYNTH_WORKERS")) if workers is None else workers
    loop = asyncio.get_running_loop()
    return await loop.run_in_executor(
        _get_pool(size), partial(_locked_call, fn, *args, **kwargs)
    )


_DONE = object()


def locked_stream(gen_fn: Callable[[], Any], *, maxsize: int = 8,
                  poll_seconds: float = 0.1) -> Any:
    """整段生成持 INFERENCE_LOCK 的转发生成器（生产者线程 + 有界队列）。

    为什么生产者线程：stream 端点必须边生成边吐（F-135 尽早出声），但整段
    生成期间必须持锁（prefix 覆盖发生在生成中途的任意时刻）。直接在
    `with lock: for chunk in streamer: yield` 里 yield 会把锁占死在
    消费者暂停/断开上 ⇒ phonemes 请求饿死到请求超时。

    有界队列语义：消费者读慢 → 生产者在 put 上节流（每 poll_seconds 检查一次
    停止）；消费者断开（GeneratorExit）→ stop_event 置位 → 生产者在下一个
    chunk 边界退出放锁（最多浪费一个 chunk 的推理）。

    异常语义：gen_fn() 抛错 → 消费者侧原样抛出（不静默吞成截断）。
    """
    q: queue.Queue = queue.Queue(maxsize=maxsize)
    stop_event = threading.Event()

    def producer() -> None:
        try:
            with INFERENCE_LOCK:
                for chunk in gen_fn():
                    if stop_event.is_set():
                        return
                    while True:
                        if stop_event.is_set():
                            return
                        try:
                            q.put(chunk, timeout=poll_seconds)
                            break
                        except queue.Full:
                            continue
        except BaseException as e:  # noqa: BLE001 —— 异常必须送达消费者侧
            try:
                q.put(e, timeout=1.0)
            except queue.Full:
                pass
        finally:
            try:
                q.put(_DONE, timeout=1.0)
            except queue.Full:
                pass

    threading.Thread(target=producer, name="xtts-stream-inference", daemon=True).start()
    try:
        while True:
            item = q.get()
            if item is _DONE:
                break
            if isinstance(item, BaseException):
                raise item
            yield item
    finally:
        stop_event.set()
