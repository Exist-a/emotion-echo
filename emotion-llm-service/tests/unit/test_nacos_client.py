"""emotion-llm-service · Nacos 客户端单测（Stage 31 PR-10）

覆盖：
  - is_sensitive_data_id 防御性默认（拒绝 jwt./database./llm.* 等前缀）
  - NacosRuntime.start 5 步流程（mock NacosClient）
  - NacosRuntime.close 优雅退出（心跳取消 + shutdown）
  - wait_for_nacos 失败语义（不可达）
"""
from __future__ import annotations

import asyncio
import socket
from unittest.mock import MagicMock, patch

import pytest

from nacos_client import (
    NacosConfig,
    NacosRuntime,
    is_sensitive_data_id,
    wait_for_nacos,
)


# -----------------------------------------------------------------------------
# is_sensitive_data_id
# -----------------------------------------------------------------------------

@pytest.mark.parametrize(
    "data_id,want",
    [
        # 敏感前缀
        ("jwt.secret", True),
        ("JWT.SECRET", True),
        ("database.dsn", True),
        ("db.password", True),
        ("kafka.brokers", True),
        ("llm.api_key", True),
        ("openai.key", True),
        ("deepseek.token", True),
        ("postgres_password", True),
        # 敏感后缀
        ("anything.secret", True),
        ("my.PASSWORD", True),
        ("auth.token", True),
        ("primary.dsn", True),
        # 正常运营参数
        ("emotion-llm-service.ops.yaml", False),
        ("feature_flags", False),
        ("rate_limit", False),
        ("model_router", False),
    ],
)
def test_is_sensitive_data_id(data_id, want):
    assert is_sensitive_data_id(data_id) is want


# -----------------------------------------------------------------------------
# NacosRuntime.start 流程
# -----------------------------------------------------------------------------

def _make_mock_nacos_client():
    """构造一个最小可用的 nacos-sdk-python mock。

    nacos_client.NacosRuntime.start 通过 _create_nacos_client 动态 import，
    单测用 patch 拦截。
    """
    mock = MagicMock()
    mock.add_naming_instance = MagicMock(return_value=True)
    mock.get_config = MagicMock(return_value="feature_flags:\n  x: true\n")
    mock.add_config_watcher = MagicMock()
    mock.shutdown = MagicMock()
    mock.close = MagicMock()
    return mock


@pytest.mark.asyncio
async def test_start_registers_and_loads_config():
    """5 步流程：register → heartbeat → get_config → listen_config 全部触发"""
    mock_client = _make_mock_nacos_client()
    cfg = NacosConfig(server_addr="fake:8848", namespace="emotion-echo-dev")

    runtime = NacosRuntime(cfg)
    with patch("nacos_client._create_nacos_client", AsyncMock(return_value=mock_client)):
        await runtime.start(
            svc_name="emotion-llm-service",
            host="0.0.0.0",
            port=8000,
            metadata={"stage": "emotion-echo-dev"},
        )

    # Assert: register 被调用 1 次
    assert mock_client.add_naming_instance.call_count == 1
    call_kwargs = mock_client.add_naming_instance.call_args.kwargs
    assert call_kwargs["service_name"] == "emotion-llm-service"
    assert call_kwargs["port"] == 8000
    assert call_kwargs["metadata"]["stage"] == "emotion-echo-dev"

    # Assert: get_config 被调用 1 次（dataId = svc-name + ".ops.yaml"）
    assert mock_client.get_config.call_count == 1
    assert mock_client.get_config.call_args.args[0] == "emotion-llm-service.ops.yaml"
    assert mock_client.get_config.call_args.args[1] == "DEFAULT_GROUP"

    # Assert: 心跳 task 已启动
    assert runtime._heartbeat_task is not None
    assert not runtime._heartbeat_task.done()

    await runtime.close()


@pytest.mark.asyncio
async def test_close_cancels_heartbeat_and_shuts_down():
    mock_client = _make_mock_nacos_client()
    cfg = NacosConfig(server_addr="fake:8848")

    runtime = NacosRuntime(cfg)
    with patch("nacos_client._create_nacos_client", AsyncMock(return_value=mock_client)):
        await runtime.start("emotion-llm-service", "0.0.0.0", 8000)

    heartbeat_task = runtime._heartbeat_task
    await runtime.close()

    # Assert: 心跳 task 已取消
    assert heartbeat_task.cancelled() or heartbeat_task.done()
    # Assert: client.shutdown / close 被调用（优先 shutdown）
    assert mock_client.shutdown.called or mock_client.close.called
    # Assert: 二次 close 是 no-op（不报错）
    await runtime.close()


@pytest.mark.asyncio
async def test_get_config_failure_is_logged_not_fatal():
    """GetConfig 失败时 start 不抛出（首次启动 Nacos 控制台尚未 bootstrap）"""
    mock_client = _make_mock_nacos_client()
    mock_client.get_config = MagicMock(side_effect=Exception("rpc timeout"))

    cfg = NacosConfig(server_addr="fake:8848")
    runtime = NacosRuntime(cfg)
    with patch("nacos_client._create_nacos_client", AsyncMock(return_value=mock_client)):
        # 不应抛出
        await runtime.start("emotion-llm-service", "0.0.0.0", 8000)

    await runtime.close()


@pytest.mark.asyncio
async def test_listen_config_failure_is_logged_not_fatal():
    mock_client = _make_mock_nacos_client()
    mock_client.add_config_watcher = MagicMock(side_effect=Exception("subscribe failed"))

    cfg = NacosConfig(server_addr="fake:8848")
    runtime = NacosRuntime(cfg)
    with patch("nacos_client._create_nacos_client", AsyncMock(return_value=mock_client)):
        # 提供 on_config_change 触发 listen_config 路径
        async def cb(d, g, c):
            pass
        await runtime.start("emotion-llm-service", "0.0.0.0", 8000, on_config_change=cb)

    await runtime.close()


# -----------------------------------------------------------------------------
# wait_for_nacos 失败语义
# -----------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_wait_for_nacos_unreachable_raises():
    """不可达时应在 max_wait 后抛出 RuntimeError（不无限等待）。"""
    # 127.0.0.1:1 几乎肯定不被监听
    with pytest.raises(RuntimeError, match="not reachable"):
        await wait_for_nacos("127.0.0.1:1", max_wait=1.5, interval=0.2)


# -----------------------------------------------------------------------------
# 测试辅助：把 MagicMock 包成 awaitable
# -----------------------------------------------------------------------------

class AsyncMock:
    """简易 AsyncMock（pytest-mock 提供完整版，但 emotion-llm-service 当前没装）。"""
    def __init__(self, return_value=None):
        self.return_value = return_value
        self.call_count = 0

    def __call__(self, *args, **kwargs):
        self.call_count += 1
        async def _coro():
            return self.return_value
        return _coro()
