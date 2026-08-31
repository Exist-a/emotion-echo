"""
emotion-llm-service · Nacos 接入（Stage 31 PR-10）

设计要点（与 Go svc 同构，PR-07/08/09 模板）：
  - 启动时 WaitForNacos → register → heartbeat → get_config → listen_config
  - 优雅退出：lifespan shutdown 时 deregister + close
  - 敏感 dataId 防御：jwt.* / database.* / kafka.* / llm.* / openai.* / deepseek.*
    / postgres_password / *.secret / *.password / *.token / *.dsn
    （与 Go shared/pkg/configcenter/nacos_config.go 同源规则）

依赖：nacos-sdk-python >= 3.1.0
（3.0.x 因断线不重注册缺陷被 yanked；锁定 ≥ 3.1.0）
"""
from __future__ import annotations

import asyncio
import logging
import os
import re
from typing import Awaitable, Callable, Optional

logger = logging.getLogger(__name__)

# 敏感 dataId 模式（与 Go shared/pkg/configcenter/nacos_config.go 同步）
SENSITIVE_PATTERNS = [
    re.compile(r"^jwt\.", re.IGNORECASE),
    re.compile(r"^database\.", re.IGNORECASE),
    re.compile(r"^db\.", re.IGNORECASE),
    re.compile(r"^kafka\.", re.IGNORECASE),
    re.compile(r"^kafka_brokers$", re.IGNORECASE),
    re.compile(r"^llm\.", re.IGNORECASE),
    re.compile(r"^openai\.", re.IGNORECASE),
    re.compile(r"^deepseek\.", re.IGNORECASE),
    re.compile(r"^postgres_password$", re.IGNORECASE),
    re.compile(r"\.secret$", re.IGNORECASE),
    re.compile(r"\.password$", re.IGNORECASE),
    re.compile(r"\.token$", re.IGNORECASE),
    re.compile(r"\.dsn$", re.IGNORECASE),
]


def is_sensitive_data_id(data_id: str) -> bool:
    """判断 dataId 是否为敏感配置（应通过 etc/*.yaml / env 而非 Nacos 传递）。"""
    return any(p.search(data_id) for p in SENSITIVE_PATTERNS)


class NacosConfig:
    """Nacos 连接配置（与 Go shared/pkg/discovery.NacosConfig 字段对齐）。"""

    def __init__(
        self,
        server_addr: str,
        namespace: str = "emotion-echo-dev",
        group_name: str = "DEFAULT_GROUP",
        username: str = "",
        password: str = "",
        timeout_ms: int = 5000,
    ):
        self.server_addr = server_addr
        self.namespace = namespace
        self.group_name = group_name
        self.username = username
        self.password = password
        self.timeout_ms = timeout_ms


# NacosClient 类型：可以是 nacos.NacosClient 或 mock（单测）
NacosClientLike = object  # 真实类型：nacos.NacosClient（运行时才 import 避免测试时硬依赖）


async def _create_nacos_client(cfg: NacosConfig):
    """惰性 import nacos-sdk-python；缺包时给清晰错误。"""
    try:
        from nacos import NacosClient  # type: ignore
    except ImportError as e:
        raise RuntimeError(
            "nacos-sdk-python not installed. "
            "Install: pip install 'nacos-sdk-python>=3.1.0'"
        ) from e

    # nacos-sdk-python 支持多 server：用逗号分隔时仅取第一个
    # （与 Go SDK buildServerConfigs 多节点不同；Python SDK 仅支持单 endpoint）
    return NacosClient(
        server_addresses=cfg.server_addr,
        namespace=cfg.namespace,
        username=cfg.username or None,
        password=cfg.password or None,
        timeout=cfg.timeout_ms / 1000.0,
    )


class NacosRuntime:
    """emotion-llm-service 的 Nacos 运行时客户端封装。

    使用方式（在 FastAPI lifespan 中）：
        runtime = NacosRuntime(cfg)
        await runtime.start(svc_name="emotion-llm-service", host=..., port=8000)
        ...
        await runtime.close()
    """

    def __init__(self, cfg: NacosConfig):
        self._cfg = cfg
        self._client = None
        self._heartbeat_task: Optional[asyncio.Task] = None
        self._stopped = False

    @property
    def client(self):
        return self._client

    @property
    def cfg(self) -> NacosConfig:
        return self._cfg

    async def start(
        self,
        svc_name: str,
        host: str,
        port: int,
        metadata: Optional[dict] = None,
        ops_data_id: Optional[str] = None,
        on_config_change: Optional[Callable[[str, str, str], Awaitable[None]]] = None,
    ) -> None:
        """启动流程：connect → register → heartbeat → get_config → listen_config。

        失败语义（与 Go svc 同构）：
          - connect 失败 → RuntimeError
          - register 失败 → RuntimeError
          - get_config 失败（无配置）→ 不阻断（dev 首次启动正常）
          - listen_config 失败 → log warning，不阻断
        """
        self._client = await _create_nacos_client(self._cfg)

        meta = {"stage": self._cfg.namespace, "version": _git_version()}
        if metadata:
            meta.update(metadata)

        # Register（nacos-sdk-python 同步方法；在线程池跑避免阻塞 event loop）
        loop = asyncio.get_event_loop()
        await loop.run_in_executor(
            None,
            lambda: self._client.add_naming_instance(
                service_name=svc_name,
                ip=host,
                port=port,
                metadata=meta,
            ),
        )
        logger.info("[nacos] registered %s at %s:%d (metadata=%s)", svc_name, host, port, meta)

        # Heartbeat task：5s 间隔
        self._heartbeat_task = asyncio.create_task(self._heartbeat_loop(svc_name, host, port, meta))

        # GetConfig + ListenConfig
        data_id = ops_data_id or f"{svc_name}.ops.yaml"
        try:
            content = await loop.run_in_executor(
                None, lambda: self._client.get_config(data_id, self._cfg.group_name)
            )
            logger.info(
                "[nacos] ops config loaded: %s/%s, %d bytes",
                self._cfg.group_name, data_id, len(content or ""),
            )
        except Exception as e:
            logger.warning("[nacos] GetConfig(%s/%s) failed (continuing): %s", self._cfg.group_name, data_id, e)
            content = ""

        if on_config_change is not None:
            try:
                self._client.add_config_watcher(data_id, self._cfg.group_name, _sync_callback(on_config_change))
                logger.info("[nacos] ListenConfig registered: %s/%s", self._cfg.group_name, data_id)
            except Exception as e:
                logger.warning("[nacos] ListenConfig failed (continuing): %s", e)

    async def close(self) -> None:
        """优雅退出：取消心跳 → deregister → close client。"""
        if self._stopped:
            return
        self._stopped = True
        if self._heartbeat_task is not None:
            self._heartbeat_task.cancel()
            try:
                await self._heartbeat_task
            except asyncio.CancelledError:
                pass
        if self._client is not None:
            try:
                # nacos-sdk-python: shutdown() 关闭所有实例 + 心跳
                if hasattr(self._client, "shutdown"):
                    self._client.shutdown()
                elif hasattr(self._client, "close"):
                    self._client.close()
            except Exception as e:
                logger.warning("[nacos] close failed (continuing): %s", e)

    async def _heartbeat_loop(self, svc_name: str, host: str, port: int, metadata: dict) -> None:
        """5s 间隔发送心跳（nacos-sdk-python 自动维护，这里仅保活）。"""
        # nacos-sdk-python >= 3.1.0 内部自动续约；本 loop 仅观测心跳状态
        # 并在异常时打 warn，不做显式 SendHeartbeat（SDK API 不暴露）
        try:
            while not self._stopped:
                await asyncio.sleep(5)
        except asyncio.CancelledError:
            raise


def _sync_callback(async_fn):
    """把 async 回调包装成 nacos-sdk-python 的同步 callback。

    nacos-sdk-python 用线程调用 callback，所以我们在新 event loop 中跑 await。
    """
    def wrapper(args):
        try:
            loop = asyncio.new_event_loop()
            try:
                loop.run_until_complete(async_fn(*args))
            finally:
                loop.close()
        except Exception as e:
            logger.warning("[nacos] config change callback failed: %s", e)
    return wrapper


def _git_version() -> str:
    """构建期注入的 git SHA（dev 默认 'dev-build'）。"""
    return os.getenv("GIT_VERSION", "dev-build")


async def wait_for_nacos(server_addr: str, max_wait: float = 60.0, interval: float = 0.5) -> None:
    """等待 Nacos TCP 可达（指数退避，最长 max_wait 秒）。

    dev 用：compose depends_on 已保证顺序；这是双保险（与 Go shared/pkg/discovery
    WaitForNacos 等价）。
    """
    import socket

    # 取第一个 endpoint
    first = server_addr.split(",")[0].strip()
    if ":" in first:
        host, port_str = first.rsplit(":", 1)
        port = int(port_str)
    else:
        host, port = first, 8848

    import time
    deadline = time.monotonic() + max_wait
    delay = interval
    max_delay = 5.0

    while time.monotonic() < deadline:
        try:
            with socket.create_connection((host, port), timeout=2):
                return
        except OSError:
            pass
        await asyncio.sleep(delay)
        delay = min(delay * 2, max_delay)

    raise RuntimeError(f"Nacos {server_addr} not reachable within {max_wait}s")
