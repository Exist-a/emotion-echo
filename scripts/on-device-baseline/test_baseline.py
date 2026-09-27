"""云端基线跑分 TDD 测试（Lane O · 端侧化阶段一 · v0.3 §C.1 任务 5）。

跑法（仓库根）：
    python -m pytest scripts/on-device-baseline/ -v

设计：
- 不依赖真实网络 / 真实容器（fake_stub + tmp_path 隔离）
- 覆盖 factory + 3 impl + baseline_run 三层契约
- ≥ 10 用例（按任务 DoD 要求）
"""
from __future__ import annotations

import json
import sys
from pathlib import Path
from types import SimpleNamespace

import pytest  # noqa: F401

HERE = Path(__file__).resolve().parent
# 走 golden_bridge 跨目录 import（避开 pytest 路径解析）
sys.path.insert(0, str(HERE))

from model_fn_factory import ReplyResult, make_model_fn  # noqa: E402
from golden_bridge import load_cases  # noqa: E402

# run_baseline 顶层 import 已处理 sys.path
import run_baseline as _rb  # noqa: E402
baseline_run = _rb.baseline_run
render_report = _rb.render_report


# ============================================================
# 工厂契约
# ============================================================

def test_factory_rejects_unknown_impl():
    with pytest.raises(ValueError, match="unknown model_fn impl"):
        make_model_fn("not_a_real_impl")


def test_factory_returns_callable_for_cloud_grpc():
    fn = make_model_fn("cloud_grpc")
    assert callable(fn)


def test_factory_returns_callable_for_cloud_deepseek_without_key_at_construction():
    """构造时不抛；调用时才发现无 key（参数验证放 make 内部而非调用时）。"""
    fn = make_model_fn("cloud_deepseek", api_key="sk-fake")
    assert callable(fn)


# ============================================================
# cloud_grpc：fake_stub 注入（避免依赖真实容器）
# ============================================================

class _FakeChunk:
    """Duck-typed ChatChunk：满足 cloud_grpc 只读的 4 字段（delta_content/done/model/fallback_reason）。"""

    def __init__(self, *, delta_content="", done=False, model="", fallback_reason=""):
        self.delta_content = delta_content
        self.done = done
        self.model = model
        self.fallback_reason = fallback_reason


def _fake_stub(chunks):
    """生成一个 fake stub，ChatCompletion 返回可迭代 chunks。"""
    return SimpleNamespace(ChatCompletion=lambda req: iter(chunks))


def test_cloud_grpc_aggregates_streaming_chunks():
    fake = _fake_stub([
        _FakeChunk(delta_content="我在"),
        _FakeChunk(delta_content="听你说"),
        _FakeChunk(delta_content="。", model="deepseek-chat", done=True),
    ])
    fn = make_model_fn("cloud_grpc", fake_stub=fake)
    case = {"id": "t1", "layer": "daily", "input": "今天好累", "expect": {}}
    r = fn(case)
    assert isinstance(r, ReplyResult)
    assert r.text == "我在听你说。"
    assert r.model == "deepseek-chat"
    assert r.fallback_reason == ""


def test_cloud_grpc_captures_fallback_reason():
    fake = _fake_stub([
        _FakeChunk(delta_content="我在", model="mock", fallback_reason="mock_no_api_key"),
        _FakeChunk(delta_content="呢", done=True),
    ])
    fn = make_model_fn("cloud_grpc", fake_stub=fake)
    r = fn({"id": "t1", "layer": "daily", "input": "x", "expect": {}})
    assert r.model == "mock"
    assert r.fallback_reason == "mock_no_api_key"


def test_cloud_grpc_uses_first_model_when_multiple_chunks():
    """首帧带 model，后续帧 model 字段为空 —— 应保留首帧值。"""
    fake = _fake_stub([
        _FakeChunk(delta_content="hi", model="qwen-1.7b"),
        _FakeChunk(delta_content=" there", model=""),  # 后续不带
        _FakeChunk(delta_content=".", done=True),
    ])
    fn = make_model_fn("cloud_grpc", fake_stub=fake)
    r = fn({"id": "t", "layer": "daily", "input": "x", "expect": {}})
    assert r.model == "qwen-1.7b"
    assert r.text == "hi there."


def test_cloud_grpc_handles_stub_exception():
    """stub 内部抛异常 → ReplyResult.fallback_reason 非空 + text = 已聚合部分。"""

    class _BoomStub:
        def ChatCompletion(self, req):
            raise RuntimeError("boom")

    fn = make_model_fn("cloud_grpc", fake_stub=_BoomStub())
    r = fn({"id": "t", "layer": "daily", "input": "x", "expect": {}})
    assert "grpc_call_error" in r.fallback_reason
    assert "RuntimeError" in r.fallback_reason


# ============================================================
# cloud_deepseek：构造时强制 api_key
# ============================================================

def test_cloud_deepseek_raises_when_no_api_key():
    from model_fns.cloud_deepseek import ConfigurationError
    with pytest.raises(ConfigurationError, match="requires api_key"):
        make_model_fn("cloud_deepseek")  # 不传 api_key，env 也无 → 构造失败


# ============================================================
# record：回放 + 落盘
# ============================================================

def test_record_writes_replay_file_on_miss(tmp_path):
    def inner(case):
        return ReplyResult(text=f"reply-{case['id']}", model="mock", fallback_reason="", latency_ms=10)

    fn = make_model_fn("record", replay_dir=tmp_path, inner_fn=inner)
    case = {"id": "x1", "input": "hello", "expect": {}}
    r = fn(case)
    assert r.text == "reply-x1"
    replay = tmp_path / "x1.json"
    assert replay.exists()
    data = json.loads(replay.read_text(encoding="utf-8"))
    assert data["text"] == "reply-x1"
    assert data["model"] == "mock"
    assert data["latency_ms"] == 10


def test_record_hits_existing_replay_without_calling_inner(tmp_path):
    """命中回放 → 不调 inner_fn（用计数器断言）。"""
    calls = []

    def inner(case):
        calls.append(case["id"])
        return ReplyResult(text="should_not_be_used", model="")

    # 先写入一个回放
    (tmp_path / "x1.json").write_text(
        json.dumps({
            "case_id": "x1",
            "input": "hello",
            "text": "cached-text",
            "model": "cached-model",
            "fallback_reason": "",
            "latency_ms": 5,
        }, ensure_ascii=False),
        encoding="utf-8",
    )
    fn = make_model_fn("record", replay_dir=tmp_path, inner_fn=inner)
    r = fn({"id": "x1", "input": "hello", "expect": {}})
    assert r.text == "cached-text"
    assert r.model == "cached-model"
    assert r.extras.get("replay_hit") is True
    assert calls == []  # 未调 inner


def test_record_read_only_miss_returns_specific_fallback_reason(tmp_path):
    """read_only 模式 + 无回放 → 返回 reply_miss_in_read_only_mode。"""
    fn = make_model_fn(
        "record",
        replay_dir=tmp_path,
        inner_fn=lambda c: ReplyResult(text="ignored"),
        mode="read_only",
    )
    r = fn({"id": "nope", "input": "x", "expect": {}})
    assert r.text == ""
    assert r.fallback_reason == "replay_miss_in_read_only_mode"


# ============================================================
# baseline_run：与 golden 评分串联
# ============================================================

def test_baseline_run_measures_pass_length_guardrail_rates():
    """mock fallback 路径下 length 必挂；护栏可全过。"""
    def mock_fn(case):
        if case["layer"] == "high_risk":
            return ReplyResult(
                text="我很在意你的安全，请拨打心理援助热线 400-161-9995。" + "陪" * 80,
                model="mock",
                fallback_reason="mock_no_api_key",
                latency_ms=1,
            )
        return ReplyResult(
            text="我理解你的感受。" + "嗯" * 30,  # ~40 字，远不到 100
            model="mock",
            fallback_reason="mock_no_api_key",
            latency_ms=1,
        )

    cases = load_cases()
    report = baseline_run(mock_fn, cases)
    assert report["n"] == len(cases)
    assert 0.0 <= report["pass_rate"] <= 1.0
    # mock 输出过短 → length_pass_rate 必 < 1
    assert report["length_pass_rate"] < 1.0
    # mock 输出避开了护栏词 → guardrail_pass_rate 应为 1.0
    assert report["guardrail_pass_rate"] == 1.0


def test_baseline_run_rows_include_layer_and_meta():
    def stub(case):
        return ReplyResult(text="x" * 200, model="stub", fallback_reason="", latency_ms=42)

    cases = load_cases()
    report = baseline_run(stub, cases)
    layers = {r["layer"] for r in report["rows"]}
    assert layers == {"daily", "high_risk", "long_input", "personality", "emotion"}
    for r in report["rows"]:
        assert r["latency_ms"] >= 0
        assert r["model"] == "stub"
        assert r["fallback_reason"] == ""
        assert isinstance(r["ok"], bool)


def test_render_report_emits_three_rates_and_layer_table():
    stub = lambda c: ReplyResult(text="x" * 200, model="m", fallback_reason="", latency_ms=1)
    cases = load_cases()
    report = baseline_run(stub, cases)
    md = render_report(
        report,
        {
            "impl": "stub",
            "generated_at": "2026-09-24T00:00:00Z",
            "command": "pytest",
            "scope": "test",
            "warnings": ["x"],
        },
    )
    assert "pass_rate" in md
    assert "length_pass_rate" in md
    assert "guardrail_pass_rate" in md
    assert "| 层 |" in md or "| 层 |" in md.replace(" ", "|")
    assert "`daily`" in md
    assert "`high_risk`" in md