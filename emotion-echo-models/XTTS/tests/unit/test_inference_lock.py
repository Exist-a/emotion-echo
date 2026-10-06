"""test_inference_lock.py · XTTS 模型级推理互斥锁单元测试（F-135 实测引爆的并发缺陷）

背景（2026-10-06 F-135 双端点实测，证据链三件）：
  stream+phonemes 并发实测**双双截断**（phonemes dur=1.109s / stream 62464B，
  对照串行基线 2.133s）；spec 期错误簇 = stream「index out of range in self」
  + phonemes「tensor a (84) != b (83)」。根因（vendored 代码回读）：
    gpt.py:570  compute_embeddings → store_prefix_emb(emb) 写**共享单例**
                gpt_inference.cached_prefix_emb（gpt_inference.py:22）
    gpt.py:557/640  inference() 与 inference_stream() 两条路径共用同一
                GPT2InferenceModel 实例 ⇒ 并发时后到者覆盖 prefix，先生成者的
                下一步 forward 读错 prefix_len ⇒ get_fixed_embedding 越界 /
                tensor 错位 / 提前 EOS（截断，无异常的坏数据更隐蔽）。
  2×phonemes 并发"成功"是时序运气（两个 store 几乎同时完成，generate 慢启动
  错开窗口）；stream 生成 2~13s 中途被 synthesize 的 store 命中是大概率事件。

⇒ **F-136 契约 #1（"两推理真并行"）前提有误**：torch 算子释放 GIL ≠ 模型
  无共享状态。修法 = 模型级推理互斥（INFERENCE_LOCK），事件循环不阻塞的
  F-136 原始意图仍保住（卸载线程池结构不变）。

stream 侧用生产者线程 + 有界队列（locked_stream）：整段生成持锁，但消费者
读慢/断开时锁必须能释放（防止 phonemes 被饿死到请求超时）。

契约：
  1. INFERENCE_LOCK 存在且为 threading.Lock（跨路径共享同一把）
  2. run_synth 执行体在锁内：并发两任务执行区间**不重叠**（总耗时 ≥ 两段之和）
     ——本条是对 test_synth_pool.py 契约 #1 的**有意反转**（缺陷修正，见上）
  3. locked_stream 保序完整转发 + 结束信号
  4. locked_stream 异常透传给消费者
  5. 消费者提前关闭（GeneratorExit）→ 生产者退出 + 锁释放（join 有界）
  6. locked_stream 与 run_synth 互斥（同锁，区间不重叠）
  7. server.py 接线静态契约：stream_audio_generator 经 locked_stream 消费推理
     （不 import server —— torch 2GB 加载链，仓内既有约定）
"""
from __future__ import annotations

import asyncio
import sys
import threading
import time
from pathlib import Path

import pytest

SERVICE_DIR = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(SERVICE_DIR))

from synth_pool import INFERENCE_LOCK, locked_stream, run_synth  # noqa: E402


def _sleep_task(seconds: float) -> str:
    time.sleep(seconds)
    return "ok"


def _intervals_overlap(a: tuple[float, float], b: tuple[float, float]) -> bool:
    return a[0] < b[1] and b[0] < a[1]


class TestInferenceLockExists:
    def test_lock_is_threading_lock(self):
        assert isinstance(INFERENCE_LOCK, type(threading.Lock()))


class TestRunSynthMutualExclusion:
    def test_concurrent_tasks_do_not_overlap(self):
        """并发两任务的执行区间不得重叠（模型共享状态 ⇒ 必须互斥）。

        注：不用 Barrier 强制同时到达 —— 互斥下第二个任务在锁外排队，
        barrier 永远凑不齐（BrokenBarrier）。两任务同时提交进 2-worker 池，
        区间取**任务体内部**计时（B 的等待时间在锁上，不计入区间）。
        """
        intervals: dict[str, tuple[float, float]] = {}

        def task(tag: str) -> str:
            t0 = time.perf_counter()
            time.sleep(0.3)
            t1 = time.perf_counter()
            intervals[tag] = (t0, t1)
            return tag

        async def scenario():
            return await asyncio.gather(
                run_synth(task, "A"),
                run_synth(task, "B"),
            )

        results = asyncio.run(scenario())
        assert sorted(results) == ["A", "B"]
        assert not _intervals_overlap(intervals["A"], intervals["B"]), (
            f"run_synth 两任务执行区间重叠 {intervals} —— 模型级推理未互斥，"
            "cached_prefix_emb 数据竞争（F-135 实测截断/崩溃根因）未修"
        )

    def test_task_runs_holding_lock(self):
        """执行体必须真的持锁（否则互斥是假象）。"""
        seen = {}

        def task() -> str:
            seen["locked"] = INFERENCE_LOCK.locked()
            return "ok"

        asyncio.run(run_synth(task))
        assert seen.get("locked") is True, "run_synth 执行体未持 INFERENCE_LOCK"


class TestLockedStream:
    def test_forwards_chunks_in_order_and_completes(self):
        """保序完整转发 + 正常收尾。"""
        chunks = [b"c1", b"c2", b"c3"]

        def gen_fn():
            yield from chunks

        got = list(locked_stream(gen_fn))
        assert got == chunks

    def test_exception_propagates_to_consumer(self):
        """生产者异常 → 消费者侧原样抛出（不静默吞成截断）。"""
        boom = RuntimeError("inference exploded")

        def gen_fn():
            yield b"ok-chunk"
            raise boom

        with pytest.raises(RuntimeError) as ei:
            list(locked_stream(gen_fn))
        assert ei.value is boom

    def test_consumer_abort_releases_lock(self):
        """消费者提前关闭 → 生产者退出 + 锁释放（防 phonemes 饿死）。"""
        release_gate = threading.Event()

        def gen_fn():
            for i in range(1000):  # 远多于消费者消费量
                release_gate.wait(timeout=5)  # 模拟慢推理，消费侧先行退出
                if release_gate.is_set():
                    release_gate.clear()
                yield f"chunk-{i}".encode()

        stream = locked_stream(gen_fn)
        first = next(stream)  # 消费 1 块
        assert first == b"chunk-0"
        release_gate.set()  # 让生产者推进
        stream.close()  # 消费者断开

        # 生产者必须在有界时间内退出并放锁
        deadline = time.perf_counter() + 5
        while INFERENCE_LOCK.locked() and time.perf_counter() < deadline:
            time.sleep(0.05)
        assert not INFERENCE_LOCK.locked(), "消费者断开后锁未被释放（生产者未退出）"

    def test_mutex_with_run_synth(self):
        """locked_stream 生成期间，run_synth 任务不得重叠执行（同一把锁）。"""
        intervals: dict[str, tuple[float, float]] = {}

        def synth_task() -> str:
            t0 = time.perf_counter()
            time.sleep(0.3)
            intervals["synth"] = (t0, time.perf_counter())
            return "ok"

        def gen_fn():
            for i in range(3):
                time.sleep(0.15)
                t0 = time.perf_counter()
                intervals.setdefault("stream", (t0, 0))
                intervals["stream"] = (min(intervals["stream"][0], t0), time.perf_counter())
                yield b"x"

        async def scenario():
            stream = locked_stream(gen_fn)

            async def consume():
                for _ in stream:
                    await asyncio.sleep(0)

            await asyncio.gather(consume(), run_synth(synth_task))

        asyncio.run(scenario())
        assert "synth" in intervals
        # stream 持锁生成期间（含 chunk 间隔），synth 必须等锁
        assert not _intervals_overlap(intervals["stream"], intervals["synth"]), (
            f"stream 与 run_synth 执行区间重叠 {intervals} —— 未共用同一把推理锁"
        )


class TestServerWiring:
    def test_stream_generator_uses_locked_stream(self):
        """静态接线契约：server.py 的 stream_audio_generator 必须经 locked_stream
        消费 inference_stream（文本扫描，不 import server）。"""
        src = (SERVICE_DIR / "server.py").read_text(encoding="utf-8")
        assert "locked_stream" in src, "server.py 未接线 locked_stream"
        # 推理迭代必须交给 locked_stream（不得裸 for chunk in streamer 直吐）
        assert "for chunk in streamer:" not in src, (
            "stream_audio_generator 仍裸迭代 inference_stream —— 并发污染未修"
        )
