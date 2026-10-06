"""
test_synth_pool.py · XTTS 推理卸载线程池单元测试（E2E-28 C4 / D-43 落地）

背景（E2E-28 #11 实测 + server.py 代码回读）：
  两个并发 /tts_with_phonemes 实测严格串行（[12.2s, 23.3s]，后者≈两者之和，
  排队等待 ≈12.2s）。账本 F-136 记的根因是"uvicorn 单 worker"，本轮更深定位：
  **async def handler 在事件循环里直调 tts_model.synthesize()（阻塞 13~30s）
  ⇒ 整个事件循环锁死 ⇒ 第二个请求连被 accept 处理都要等第一个完成**。
  进程级多 worker 能解但每 worker ~2.5GiB（.wslconfig 8GB 顶、19 容器稳态
  ~6G）有击穿风险 ⇒ 修法取**线程池卸载**（torch CPU 算子释放 GIL，零内存成本）。

契约：
  1. 【2026-10-06 反转】run_synth 模型级互斥：两个 0.4s 任务串行（≥0.8s）——
     原"并行"契约有误（模型共享 cached_prefix_emb，并发=数据竞争，
     F-135 实测引爆）；F-136 原始意图（事件循环不阻塞）仍由本文件验证
  2. 池容量上限：max_workers=1 时两个任务串行（≥0.8s）——防"无限开线程"
  3. 异常透传：任务抛错 → await 方抛同异常（不静默）
  4. env 配置：XTTS_SYNTH_WORKERS 解析（非法值回退默认）
  5. server.py 接线静态契约（不 import server——torch 2GB 加载链，仓内
     既有测试约定）：tts_with_phonemes / tts 走 run_synth 卸载 +
     stream_audio_generator 为同步 def（Starlette 对同步迭代器自动走线程池）

RED（2026-10-03）：synth_pool 模块不存在 → ImportError。
"""
from __future__ import annotations

import asyncio
import sys
import time
from pathlib import Path

import pytest

SERVICE_DIR = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(SERVICE_DIR))

from synth_pool import run_synth, parse_workers  # noqa: E402


def _sleep_task(seconds: float) -> str:
    time.sleep(seconds)
    return "ok"


class TestRunSynthConcurrency:
    def test_two_tasks_overlap(self):
        """【2026-10-06 有意反转】并发任务模型级互斥：两个 0.4s 任务 ≥0.8s。

        F-135 实测推翻原契约"两推理真并行"：inference/inference_stream 共享
        gpt_inference.cached_prefix_emb（gpt.py:570 覆盖写）⇒ 并发推理 =
        数据竞争（stream+phonemes 并发双双截断 1.1s vs 基线 2.1s + index
        out of range / tensor 错位错误簇）。修法 = INFERENCE_LOCK（见
        test_inference_lock.py）。本测试只验证**事件循环不阻塞**（两任务
        都能在池里被受理完成，而非第二个请求连 accept 都进不来）。
        """
        start = time.perf_counter()
        results = asyncio.run(_two_runs(0.4))
        elapsed = time.perf_counter() - start
        assert results == ["ok", "ok"]
        assert elapsed >= 0.8, (
            f"两个 0.4s 任务耗时 {elapsed:.3f}s —— 未互斥！模型并发 = "
            "cached_prefix_emb 数据竞争（坏数据比崩溃更隐蔽）"
        )

    def test_pool_capacity_one_serializes(self):
        """workers=1 → 必须串行（≥0.8s）：证明池容量真的生效。"""
        start = time.perf_counter()
        results = asyncio.run(_two_runs(0.4, workers=1))
        elapsed = time.perf_counter() - start
        assert results == ["ok", "ok"]
        assert elapsed >= 0.8, (
            f"workers=1 却 {elapsed:.3f}s 完成 —— 池容量没生效（无限并发）"
        )

    def test_exception_propagates(self):
        """任务异常必须透传到 await 点，不许吞。"""
        async def scenario():
            await run_synth(_boom)
        with pytest.raises(ValueError, match="boom"):
            asyncio.run(scenario())


def _boom() -> None:
    raise ValueError("boom")


async def _two_runs(seconds: float, workers: int | None = None) -> list[str]:
    r1 = asyncio.create_task(run_synth(_sleep_task, seconds, workers=workers))
    r2 = asyncio.create_task(run_synth(_sleep_task, seconds, workers=workers))
    return await asyncio.gather(r1, r2)


class TestParseWorkers:
    def test_valid_value(self):
        assert parse_workers("4") == 4

    def test_invalid_falls_back_to_default(self):
        assert parse_workers("not-a-number") == 2
        assert parse_workers("0") == 2
        assert parse_workers("-1") == 2

    def test_default_when_unset(self):
        assert parse_workers(None) == 2


class TestServerWiringStaticContract:
    """server.py 接线静态契约（仓内约定：不 import server.py 的 torch 链）。"""

    @staticmethod
    def _server_src() -> str:
        return (SERVICE_DIR / "server.py").read_text(encoding="utf-8")

    def test_handlers_offload_via_run_synth(self):
        src = self._server_src()
        assert "run_synth(" in src, "server.py 未使用 run_synth 卸载"
        # 两个同步推理 handler 的函数体内必须出现 run_synth
        for fn in ("async def tts_with_phonemes", "async def text_to_speech"):
            assert fn in src, f"缺 {fn}"
            i = src.index(fn)
            body = src[i:i + 2500]
            assert "run_synth(" in body, (
                f"{fn} 函数体 2500 字符内无 run_synth —— 阻塞推理仍在事件循环里"
            )

    def test_stream_generator_is_sync(self):
        """stream_audio_generator 必须是同步 def —— Starlette 对同步迭代器
        自动 run_in_threadpool；async 生成器会回到事件循环里阻塞。"""
        src = self._server_src()
        assert "async def stream_audio_generator" not in src, (
            "stream_audio_generator 仍是 async —— 流式块生成会锁死事件循环"
        )
        assert "def stream_audio_generator" in src


class TestGreenWiringContracts:
    """GREEN 侧配套契约（与实现同批落地的真实断言）。"""

    def test_synth_pool_module_never_imports_torch(self):
        """synth_pool 必须保持零 torch 依赖 —— 它是卸载层，若被拖进 torch
        导入链，池本身变重且测试无法脱离模型环境运行。"""
        src = (SERVICE_DIR / "synth_pool.py").read_text(encoding="utf-8")
        for line in src.splitlines():
            stripped = line.strip()
            if stripped.startswith(("import ", "from ")):
                assert "torch" not in stripped, (
                    f"synth_pool 顶层引入 torch: {stripped}"
                )

    def test_compose_declares_synth_workers_env(self):
        """compose 必须显式声明 XTTS_SYNTH_WORKERS —— 容量是调优面，
        隐式默认会让部署侧看不见可调参数（同 F-132 cpus 隐式教训）。"""
        repo = SERVICE_DIR.parents[1]
        compose = (repo / "deploy" / "docker-compose.apps.yml").read_text(
            encoding="utf-8")
        # 锚点必须是 2 空格缩进的服务键 —— 裸 "emotion-echo-xtts:" 会先撞
        # XTTS_BASE_URL 里的 "emotion-echo-xtts:8003" 子串（首轮实测误命中）。
        i = compose.index("\n  emotion-echo-xtts:\n")
        block = compose[i:i + 3000]
        assert "XTTS_SYNTH_WORKERS" in block, (
            "compose xtts 段未声明 XTTS_SYNTH_WORKERS"
        )

    def test_dockerfile_copies_synth_pool(self):
        """Dockerfile 必须 COPY synth_pool.py —— server.py 顶层
        `from synth_pool import run_synth`，漏 COPY ⇒ 容器启动即
        ModuleNotFoundError（2026-10-03 首轮构建前自查抓到的真缺口，
        与 Stage 36-B5 pcm_chunk_shape 漏 COPY 同型）。"""
        dockerfile = (SERVICE_DIR / "Dockerfile").read_text(encoding="utf-8")
        copy_lines = [ln for ln in dockerfile.splitlines()
                      if ln.startswith("COPY") and "server.py" in ln]
        assert copy_lines, "Dockerfile 无 server.py COPY 行"
        assert any("synth_pool.py" in ln for ln in copy_lines), (
            "server.py 的 COPY 行未随行 synth_pool.py —— 镜像启动必炸"
        )
