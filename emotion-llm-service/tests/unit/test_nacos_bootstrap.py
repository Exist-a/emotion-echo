"""emotion-llm-service · Nacos bootstrap 集成测试（lifespan 验证）

验证 main.py 的 lifespan 在启动时调用 NacosRuntime.start，
关闭时调用 close。
"""
from __future__ import annotations

import os
from unittest.mock import AsyncMock, MagicMock, patch

import pytest


@pytest.fixture
def mock_nacos_runtime_class():
    """patch NacosRuntime 类以捕获 start / close 调用。"""
    fake_instance = MagicMock()
    fake_instance.start = AsyncMock()
    fake_instance.close = AsyncMock()

    fake_class = MagicMock(return_value=fake_instance)
    return fake_class, fake_instance


@pytest.mark.asyncio
async def test_lifespan_starts_and_closes_nacos(monkeypatch, mock_nacos_runtime_class):
    fake_class, fake_instance = mock_nacos_runtime_class
    monkeypatch.setenv("NACOS_ENABLED", "true")
    monkeypatch.setenv("NACOS_ADDR", "fake:8848")
    monkeypatch.setenv("SVC_NAME", "emotion-llm-service")

    # 拦截 _create_nacos_client（避免真实 import）— lifespan start 内 await 之前已 patch class
    # 但 NacosRuntime.__init__ 不会 import；只有 .start 会
    # 我们这里用 fake_instance 直接替换整个 runtime
    with patch("nacos_client.NacosRuntime", fake_class), \
         patch("nacos_client.wait_for_nacos", AsyncMock(return_value=None)):
        # 重新 import main 确保 monkeypatch 生效
        import importlib
        import main as main_mod
        importlib.reload(main_mod)

        # 触发 lifespan
        async with main_mod.lifespan(main_mod.app):
            # 期间 start 应被调用
            pass

    assert fake_instance.start.call_count == 1
    assert fake_instance.close.call_count == 1

    # 清理 module-level 副作用
    importlib.reload(main_mod)


@pytest.mark.asyncio
async def test_lifespan_skips_nacos_when_disabled(monkeypatch, mock_nacos_runtime_class):
    fake_class, fake_instance = mock_nacos_runtime_class
    monkeypatch.setenv("NACOS_ENABLED", "false")

    with patch("nacos_client.NacosRuntime", fake_class):
        import importlib
        import main as main_mod
        importlib.reload(main_mod)
        async with main_mod.lifespan(main_mod.app):
            pass

    assert fake_instance.start.call_count == 0
    assert fake_instance.close.call_count == 0

    importlib.reload(main_mod)


@pytest.mark.asyncio
async def test_lifespan_continues_when_nacos_unreachable(monkeypatch, mock_nacos_runtime_class):
    """wait_for_nacos 失败时 lifespan 不抛出（dev 单机调试）。"""
    fake_class, fake_instance = mock_nacos_runtime_class
    monkeypatch.setenv("NACOS_ENABLED", "true")

    async def fake_wait(*args, **kwargs):
        raise RuntimeError("nacos unreachable")

    with patch("nacos_client.NacosRuntime", fake_class), \
         patch("nacos_client.wait_for_nacos", fake_wait):
        import importlib
        import main as main_mod
        importlib.reload(main_mod)
        async with main_mod.lifespan(main_mod.app):
            pass  # 不应抛

    assert fake_instance.start.call_count == 0  # 启动失败时不创建 runtime
    assert fake_instance.close.call_count == 0

    importlib.reload(main_mod)
