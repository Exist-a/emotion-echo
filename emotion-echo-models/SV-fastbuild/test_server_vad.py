"""E2E-F-106 回归钉：SenseVoice 服务端两处修复。

覆盖点：
1. **VAD 模型走本地目录**（`FUNASR_VAD_DIR`，默认 `/app/model/vad`）—— 原实现用
   `vad_model="fsmn-vad"` + `hub="ms"`，把 VAD 下载放在**请求路径**里，首次
   /analyze 要先下载再推理，必然超过 ai-svc 的 30s 超时。
2. **启动时预热**（`@app.on_event("startup")`）—— 原为纯懒加载，server.py 自述
   "first request 30-60s"，而 ai-svc 客户端超时 30s ⇒ 冷启动第一个请求必失败。

运行：
    cd emotion-echo-models/SV-fastbuild && python -m pytest test_server_vad.py -q

设计说明：`server.py` 在模块级 import fastapi / uvicorn / logging_setup / metrics_setup，
本测试用 sys.modules 桩把这些替掉，从而能在**不装 funasr/torch/fastapi** 的机器上
直接验证被测逻辑（避免"测试需要 4GB 依赖才能跑"）。
"""
import importlib.util
import os
import sys
import types
from pathlib import Path
from unittest import mock

HERE = Path(__file__).resolve().parent


def _stub(name: str, **attrs):
    mod = types.ModuleType(name)
    for k, v in attrs.items():
        setattr(mod, k, v)
    sys.modules[name] = mod
    return mod


def load_server():
    """把 server.py 作为模块加载（第三方依赖用桩替换）。"""
    # fastapi 桩：FastAPI 类需支持 add_middleware / get / post / on_event
    class _App:
        def __init__(self, *a, **k):
            self.startup_handlers = []
            self.routes = {}

        def add_middleware(self, *a, **k):
            pass

        def get(self, path, **k):
            def deco(fn):
                self.routes[("GET", path)] = fn
                return fn
            return deco

        def post(self, path, **k):
            def deco(fn):
                self.routes[("POST", path)] = fn
                return fn
            return deco

        def on_event(self, event):
            def deco(fn):
                if event == "startup":
                    self.startup_handlers.append(fn)
                return fn
            return deco

    _stub("fastapi",
          FastAPI=_App,
          File=lambda *a, **k: None,
          HTTPException=type("HTTPException", (Exception,), {"__init__": lambda self, **k: None}),
          UploadFile=object)
    _stub("fastapi.middleware")
    _stub("fastapi.middleware.cors", CORSMiddleware=type("CORSMiddleware", (), {}))
    _stub("fastapi.responses", JSONResponse=lambda *a, **k: {"__json__": True, "args": a})
    _stub("uvicorn", run=lambda *a, **k: None)
    _stub("logging_setup", setup_logging=lambda name: __import__("logging").getLogger(name))
    _stub("metrics_setup",
          ANALYZE_TOTAL=mock.MagicMock(),
          MODEL_INFERENCE_DURATION=mock.MagicMock(),
          MetricsMiddleware=type("MetricsMiddleware", (), {}),
          metrics_endpoint=lambda: None)

    spec = importlib.util.spec_from_file_location("sv_server", HERE / "server.py")
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def test_load_model_uses_local_vad_dir_when_present(tmp_path, monkeypatch):
    """VAD 本地目录存在 ⇒ 传本地路径（离线可用），且不再让 funasr 去 ModelScope 拉。"""
    vad_dir = tmp_path / "vad"
    vad_dir.mkdir()
    monkeypatch.setenv("FUNASR_MODEL_DIR", "/app/model")
    monkeypatch.setenv("FUNASR_VAD_DIR", str(vad_dir))

    captured = {}

    class FakeAutoModel:
        def __init__(self, **kwargs):
            captured.update(kwargs)

    fake_funasr = types.ModuleType("funasr")
    fake_funasr.AutoModel = FakeAutoModel
    sys.modules["funasr"] = fake_funasr

    mod = load_server()
    mod._load_model_sync()

    assert captured["vad_model"] == str(vad_dir), (
        "本地 VAD 目录存在时必须直接用它（否则 funasr 会在请求路径里下载 fsmn-vad）"
    )


def test_load_model_falls_back_to_hub_name_when_no_local_vad(tmp_path, monkeypatch):
    """本地 VAD 不存在 ⇒ 回退 ModelScope 名称（兼容未重建的旧镜像，不硬崩）。"""
    monkeypatch.setenv("FUNASR_VAD_DIR", str(tmp_path / "missing"))
    captured = {}

    class FakeAutoModel:
        def __init__(self, **kwargs):
            captured.update(kwargs)

    fake_funasr = types.ModuleType("funasr")
    fake_funasr.AutoModel = FakeAutoModel
    sys.modules["funasr"] = fake_funasr

    mod = load_server()
    mod._load_model_sync()

    assert captured["vad_model"] == "fsmn-vad"
    assert captured["hub"] == "ms"


def test_startup_warmup_handler_registered():
    """启动必须注册预热回调 —— 否则冷启动 30-60s > ai-svc 超时 30s，首个请求必失败。"""
    mod = load_server()
    handlers = getattr(mod.app, "startup_handlers", [])
    assert handlers, "server.py 必须注册 startup 预热（@app.on_event('startup')）"
    assert any(getattr(h, "__name__", "") == "_warmup_model" for h in handlers), (
        f"startup handler 应包含 _warmup_model，实际：{[getattr(h,'__name__','?') for h in handlers]}"
    )


def test_health_reports_model_loaded_flag():
    """健康检查必须暴露 model_loaded —— Dockerfile healthcheck 依此判定真实就绪。"""
    import asyncio

    mod = load_server()
    mod._MODEL = None
    res = asyncio.get_event_loop().run_until_complete(mod.health()) if False else None
    # health 是 async 函数；用 asyncio.run 跑
    res = asyncio.run(mod.health())
    assert res["args"][0]["model_loaded"] is False
    assert res["args"][0]["status"] == "loading"

    mod._MODEL = object()
    res2 = asyncio.run(mod.health())
    assert res2["args"][0]["model_loaded"] is True
    assert res2["args"][0]["status"] == "ok"
