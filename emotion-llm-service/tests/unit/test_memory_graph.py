"""CL-08c Phase 4：memory_graph 门面 + servicer 的单元测试（假 graphiti，不打网络）。

T10 决议：LLM/embedder 均为可继承接口 ⇒ 单测注入确定性假实现。
Graphiti 真链路（falkordblite + LLM 摄取）在 dev 栈上验证（T-13 运行时）。
"""

import asyncio
import sys
import os
from datetime import datetime, timezone
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[2]))

import pytest

from memory_graph import MemoryGraph, AsyncBridge


class FakeGraphiti:
    def __init__(self, fail=False):
        self.episodes = []
        self.searches = []
        self.fail = fail

    async def add_episode(self, name, episode_body, source_description,
                          reference_time, source=None, group_id=None, **kw):
        if self.fail:
            raise RuntimeError("graph backend down")
        self.episodes.append({
            "name": name, "body": episode_body, "desc": source_description,
            "ref": reference_time, "group_id": group_id,
        })
        return object()

    async def search(self, query, group_ids=None, **kw):
        if self.fail:
            raise RuntimeError("graph backend down")
        self.searches.append({"q": query, "groups": group_ids, **kw})
        return ["hit-1", "hit-2"]


def run(coro):
    return asyncio.run(coro)


class TestUpsertEpisode:
    def test_ok_passes_group_id_and_reference_time(self):
        g = FakeGraphiti()
        mg = MemoryGraph(g)
        ok, msg = run(mg.upsert_episode(
            user_id=42, name="msg-100", content="我在学萨克斯",
            valid_at_unix_ms=1790000000000,
        ))
        assert ok and msg == ""
        assert len(g.episodes) == 1
        ep = g.episodes[0]
        assert ep["group_id"] == "42", "group_id 必须等于 user_id（T-17 用户隔离边界）"
        assert ep["body"] == "我在学萨克斯"
        assert ep["ref"] == datetime.fromtimestamp(1790000000, tz=timezone.utc)
        assert ep["name"] == "msg-100"

    def test_empty_name_falls_back_to_content_prefix(self):
        g = FakeGraphiti()
        mg = MemoryGraph(g)
        ok, _ = run(mg.upsert_episode(user_id=1, name="", content="短", valid_at_unix_ms=0))
        assert ok
        assert g.episodes[0]["name"] == "短"

    def test_invalid_inputs_rejected_without_touching_graph(self):
        g = FakeGraphiti()
        mg = MemoryGraph(g)
        ok, msg = run(mg.upsert_episode(user_id=0, name="n", content="c", valid_at_unix_ms=1))
        assert not ok and "user_id" in msg
        ok, msg = run(mg.upsert_episode(user_id=1, name="n", content="  ", valid_at_unix_ms=1))
        assert not ok and "content" in msg
        assert g.episodes == []

    def test_backend_failure_folds_to_false_not_raise(self):
        mg = MemoryGraph(FakeGraphiti(fail=True))
        ok, msg = run(mg.upsert_episode(user_id=1, name="n", content="c", valid_at_unix_ms=1))
        assert not ok and "graphiti" in msg


class TestSearch:
    def test_group_filter_is_hard_boundary(self):
        g = FakeGraphiti()
        mg = MemoryGraph(g)
        hits = run(mg.search("最近在学什么", user_id=42))
        assert hits == ["hit-1", "hit-2"]
        assert g.searches[0]["groups"] == ["42"], "检索必须带 group_ids 过滤（T-17）"

    def test_invalid_query_returns_empty(self):
        mg = MemoryGraph(FakeGraphiti())
        assert run(mg.search("  ", user_id=1)) == []
        assert run(mg.search("q", user_id=0)) == []

    def test_backend_failure_returns_empty_not_raise(self):
        mg = MemoryGraph(FakeGraphiti(fail=True))
        assert run(mg.search("q", user_id=1)) == []


class TestAsyncBridge:
    def test_runs_coroutine_on_dedicated_loop(self):
        bridge = AsyncBridge()
        bridge.start()
        try:
            async def work(v):
                await asyncio.sleep(0.01)
                return v * 2
            assert bridge.call(work(21), timeout=5) == 42
        finally:
            pass  # daemon 线程随进程退出

    def test_timeout_raises_and_cancels(self):
        import concurrent.futures
        bridge = AsyncBridge()
        bridge.start()

        async def hang():
            await asyncio.sleep(10)

        with pytest.raises(TimeoutError):
            bridge.call(hang(), timeout=0.2)


class TestServicerDegraded:
    def test_degraded_mode_returns_false_not_crash(self):
        from memory_graph_servicer import MemoryGraphServicer
        import memory_graph_pb2

        class Ctx:
            def code(self, *a, **k):
                pass

        svc = MemoryGraphServicer(None, None)
        req = memory_graph_pb2.UpsertEpisodeRequest(
            user_id=1, name="n", content="c", valid_at_unix_ms=1,
            source_kind="message", source_id=100,
        )
        resp = svc.UpsertEpisode(req, Ctx())
        assert resp.ok is False
        assert "degraded" in resp.message

    def test_real_graph_roundtrip_through_bridge(self):
        from memory_graph_servicer import MemoryGraphServicer
        import memory_graph_pb2

        g = FakeGraphiti()
        bridge = AsyncBridge()
        bridge.start()
        svc = MemoryGraphServicer(MemoryGraph(g), bridge)
        req = memory_graph_pb2.UpsertEpisodeRequest(
            user_id=7, name="msg-9", content="内容", valid_at_unix_ms=1790000000000,
        )
        resp = svc.UpsertEpisode(req, None)
        assert resp.ok is True
        assert g.episodes[0]["group_id"] == "7"
