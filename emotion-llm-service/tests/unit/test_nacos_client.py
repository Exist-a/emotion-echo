"""emotion-llm-service · Nacos 客户端单测（Stage 31 PR-10 建立 / Stage 88 重写为 v3 SDK 契约）

背景（Stage 88）：nacos_client.py 自 Stage 31 起用的是旧同步 SDK 的导入路径
（`from nacos import NacosClient`），而 requirements 锁 `nacos-sdk-python>=3.1.0`
（v3 = `v2.nacos` 命名空间，异步 gRPC）——容器内 import 必然失败，
启动降级为无 Nacos 运行（WARNING 被吞），直到 Stage 88 才被发现。

Stage 88 契约（v3 / gRPC）：
  - _import_v3_sdk 从 v2.nacos 导入（AST 源码契约 + 真实导入双锁）
  - 注册走 register_instance(RegisterInstanceParam(..., ephemeral=True))——
    gRPC 长连接保活 + SDK redo 自动重注册，**无手工心跳 task**（liveness 委托 SDK）
  - SVC_HOST=0.0.0.0 时自动探测本机真实 IP（注册 0.0.0.0 是无效地址）
  - metadata 必须带 grpc_port（BFF resolveGRPCAddr WithPortHint 消费，Stage 75 模式）
  - close = deregister → shutdown（二次 close no-op）
"""
from __future__ import annotations

import ast
import asyncio
from pathlib import Path
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

import nacos_client
from nacos_client import (
    NacosConfig,
    NacosRuntime,
    is_sensitive_data_id,
    wait_for_nacos,
)

SERVICE_DIR = Path(__file__).resolve().parents[2]


# -----------------------------------------------------------------------------
# is_sensitive_data_id（Stage 31 原有，不动）
# -----------------------------------------------------------------------------

@pytest.mark.parametrize(
    "data_id,want",
    [
        ("jwt.secret", True), ("JWT.SECRET", True), ("database.dsn", True),
        ("db.password", True), ("kafka.brokers", True), ("llm.api_key", True),
        ("openai.key", True), ("deepseek.token", True), ("postgres_password", True),
        ("anything.secret", True), ("my.PASSWORD", True), ("auth.token", True),
        ("primary.dsn", True),
        ("emotion-llm-service.ops.yaml", False), ("feature_flags", False),
        ("rate_limit", False), ("model_router", False),
    ],
)
def test_is_sensitive_data_id(data_id, want):
    assert is_sensitive_data_id(data_id) is want


# -----------------------------------------------------------------------------
# SDK 导入契约（Stage 88 核心 RED：import 路径必须与 requirements 锁定版本匹配）
# -----------------------------------------------------------------------------

def _module_imports(tree: ast.AST) -> list[str]:
    """收集模块全部 import 语句（Import / ImportFrom）的模块名。"""
    names = []
    for node in ast.walk(tree):
        if isinstance(node, ast.ImportFrom) and node.module:
            names.append(node.module)
        elif isinstance(node, ast.Import):
            names += [a.name for a in node.names]
    return names


class TestSDKImportContract:
    def test_no_legacy_sync_import(self):
        """禁止旧同步 SDK 导入路径：requirements 锁 >=3.1.0，`import nacos` 必然失败"""
        tree = ast.parse((SERVICE_DIR / "nacos_client.py").read_text(encoding="utf-8"))
        mods = _module_imports(tree)
        assert "nacos" not in mods, (
            "nacos_client.py 还在用旧同步 SDK 的 `from nacos import ...`——"
            "v3 SDK（>=3.1.0）的导入命名空间是 v2.nacos，这条路径在容器里必然 ImportError"
        )

    def test_uses_v3_import_path(self):
        tree = ast.parse((SERVICE_DIR / "nacos_client.py").read_text(encoding="utf-8"))
        assert "v2.nacos" in _module_imports(tree)

    def test_requirements_pins_v3_sdk(self):
        req = (SERVICE_DIR / "requirements.txt").read_text(encoding="utf-8")
        assert "nacos-sdk-python>=3.1.0" in req


# -----------------------------------------------------------------------------
# NacosRuntime.start 流程（v3 契约，_import_v3_sdk 边界 mock——本地无 SDK 依赖）
# -----------------------------------------------------------------------------

class _FakeRegisterParam:
    """v3 RegisterInstanceParam 的测试替身（记录 kwargs 供断言）"""

    def __init__(self, **kwargs):
        self.__dict__.update(kwargs)


class _FakeDeregisterParam:
    def __init__(self, **kwargs):
        self.__dict__.update(kwargs)


class _FakeConfigParam:
    def __init__(self, **kwargs):
        self.__dict__.update(kwargs)


class _FakeSDK:
    """_import_v3_sdk() 返回值的测试替身"""

    RegisterInstanceParam = _FakeRegisterParam
    DeregisterInstanceParam = _FakeDeregisterParam
    ConfigParam = _FakeConfigParam


def _make_mock_naming():
    mock = MagicMock()
    mock.register_instance = AsyncMock(return_value=True)
    mock.deregister_instance = AsyncMock(return_value=True)
    mock.shutdown = AsyncMock()
    return mock


def _make_mock_config():
    mock = MagicMock()
    mock.get_config = AsyncMock(return_value="feature_flags:\n  x: true\n")
    mock.add_listener = MagicMock()
    mock.shutdown = AsyncMock()
    return mock


@pytest.fixture
def v3_runtime(monkeypatch):
    """装配：SDK 替身 + naming/config 双 mock，返回 (runtime, naming, config)"""
    monkeypatch.setattr(nacos_client, "_import_v3_sdk", lambda: _FakeSDK)
    naming, config = _make_mock_naming(), _make_mock_config()
    monkeypatch.setattr(
        nacos_client, "_create_naming_service", AsyncMock(return_value=naming)
    )
    monkeypatch.setattr(
        nacos_client, "_create_config_service", AsyncMock(return_value=config)
    )
    return NacosRuntime(NacosConfig(server_addr="fake:8848", namespace="emotion-echo-dev")), naming, config


@pytest.mark.asyncio
async def test_start_registers_instance_v3(v3_runtime):
    """注册走 register_instance + RegisterInstanceParam，ephemeral=True（liveness 委托 SDK）"""
    runtime, naming, _ = v3_runtime
    await runtime.start(
        svc_name="emotion-llm-service",
        host="172.18.0.9",
        port=8000,
        metadata={"grpc_port": "50051"},
    )

    assert naming.register_instance.call_count == 1
    param = naming.register_instance.call_args.args[0]
    assert param.service_name == "emotion-llm-service"
    assert param.ip == "172.18.0.9"
    assert param.port == 8000
    assert param.ephemeral is True
    assert param.metadata["grpc_port"] == "50051"
    assert param.metadata["stage"] == "emotion-echo-dev"
    assert not hasattr(runtime, "_heartbeat_task"), (
        "v3 gRPC 长连接 + redo 由 SDK 保活，手工心跳 task 必须移除"
    )
    await runtime.close()


@pytest.mark.asyncio
async def test_start_rewrites_0_0_0_0_to_advertised_ip(v3_runtime, monkeypatch):
    """SVC_HOST=0.0.0.0（compose 默认）→ 自动探测真实容器 IP；注册 0.0.0.0 是无效地址"""
    runtime, naming, _ = v3_runtime
    monkeypatch.setattr(nacos_client, "_advertise_ip", lambda addr: "172.18.0.9")
    await runtime.start("emotion-llm-service", "0.0.0.0", 8000)

    param = naming.register_instance.call_args.args[0]
    assert param.ip == "172.18.0.9", "注册 0.0.0.0 会让 Discover 拿到不可拨号地址"
    await runtime.close()


@pytest.mark.asyncio
async def test_start_explicit_host_passes_through(v3_runtime):
    """显式配置的真实 IP 不被覆盖"""
    runtime, naming, _ = v3_runtime
    await runtime.start("emotion-llm-service", "172.18.0.42", 8000)
    param = naming.register_instance.call_args.args[0]
    assert param.ip == "172.18.0.42"
    await runtime.close()


@pytest.mark.asyncio
async def test_start_loads_ops_config_with_v3_param(v3_runtime):
    """get_config 走 ConfigParam(data_id, group)（v3 签名）"""
    runtime, _, config = v3_runtime
    await runtime.start("emotion-llm-service", "172.18.0.9", 8000)
    assert config.get_config.call_count == 1
    param = config.get_config.call_args.args[0]
    assert param.data_id == "emotion-llm-service.ops.yaml"
    assert param.group == "DEFAULT_GROUP"
    await runtime.close()


@pytest.mark.asyncio
async def test_get_config_failure_is_logged_not_fatal(v3_runtime):
    runtime, _, config = v3_runtime
    config.get_config = AsyncMock(side_effect=Exception("rpc timeout"))
    await runtime.start("emotion-llm-service", "172.18.0.9", 8000)  # 不应抛
    await runtime.close()


@pytest.mark.asyncio
async def test_listen_config_failure_is_logged_not_fatal(v3_runtime):
    runtime, _, config = v3_runtime
    config.add_listener = MagicMock(side_effect=Exception("subscribe failed"))

    async def cb(d, g, c):
        pass

    await runtime.start("emotion-llm-service", "172.18.0.9", 8000, on_config_change=cb)
    await runtime.close()


@pytest.mark.asyncio
async def test_close_deregisters_and_shuts_down(v3_runtime):
    runtime, naming, config = v3_runtime
    await runtime.start("emotion-llm-service", "172.18.0.9", 8000)
    await runtime.close()

    assert naming.deregister_instance.call_count == 1
    param = naming.deregister_instance.call_args.args[0]
    assert param.service_name == "emotion-llm-service"
    assert param.ip == "172.18.0.9"
    assert param.port == 8000
    assert naming.shutdown.call_count == 1
    assert config.shutdown.call_count == 1
    # 二次 close no-op
    await runtime.close()
    assert naming.deregister_instance.call_count == 1


@pytest.mark.asyncio
async def test_register_failure_raises(v3_runtime):
    """register 失败 → RuntimeError（与 Go svc fail-fast 语义同构；lifespan 层负责降级）"""
    runtime, naming, _ = v3_runtime
    naming.register_instance = AsyncMock(side_effect=Exception("server unavailable"))
    with pytest.raises(Exception):
        await runtime.start("emotion-llm-service", "172.18.0.9", 8000)


# -----------------------------------------------------------------------------
# _advertise_ip 探测
# -----------------------------------------------------------------------------

class TestAdvertiseIP:
    def test_0_0_0_0_resolved_via_udp_connect(self, monkeypatch):
        monkeypatch.setenv("NACOS_ADDR", "emotion-echo-nacos:8848")

        class _FakeSock:
            def connect(self, addr):
                assert addr[0] == "emotion-echo-nacos" and addr[1] == 8848

            def getsockname(self):
                return ("172.18.0.9", 0)

            def close(self):
                pass

            def __enter__(self):
                return self

            def __exit__(self, *exc):
                return False

        monkeypatch.setattr(
            nacos_client.socket, "socket", lambda *a, **k: _FakeSock()
        )
        assert nacos_client._advertise_ip("emotion-echo-nacos:8848") == "172.18.0.9"

    def test_udp_failure_falls_back_to_hostname(self, monkeypatch):
        monkeypatch.setenv("NACOS_ADDR", "emotion-echo-nacos:8848")

        class _DeadSock:
            def connect(self, addr):
                raise OSError("no route")

            def close(self):
                pass

            def __enter__(self):
                return self

            def __exit__(self, *exc):
                return False

        monkeypatch.setattr(nacos_client.socket, "socket", lambda *a, **k: _DeadSock())
        monkeypatch.setattr(
            nacos_client.socket, "gethostbyname", lambda h: "172.18.0.42"
        )
        assert nacos_client._advertise_ip("emotion-echo-nacos:8848") == "172.18.0.42"


# -----------------------------------------------------------------------------
# wait_for_nacos 失败语义（Stage 31 原有，不动）
# -----------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_wait_for_nacos_unreachable_raises():
    with pytest.raises(RuntimeError, match="not reachable"):
        await wait_for_nacos("127.0.0.1:1", max_wait=1.5, interval=0.2)
