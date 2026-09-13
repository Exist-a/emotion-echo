"""emotion-llm-service · Nacos 接入（Stage 31 PR-10 建立 / Stage 88 移植 v3 SDK）

设计要点（与 Go svc 同构）：
  - 启动时 WaitForNacos → register → get_config → listen_config
  - 优雅退出：deregister + shutdown
  - 敏感 dataId 防御：jwt.* / database.* / kafka.* / llm.* / openai.* / deepseek.*
    / postgres_password / *.secret / *.password / *.token / *.dsn
    （与 Go shared/pkg/configcenter/nacos_config.go 同源规则）

Stage 88 · SDK v3（nacos-sdk-python >= 3.1.0，导入命名空间 v2.nacos）：
  - gRPC 长连接协议，与 Go SDK v2 同协议；ephemeral 实例 liveness 由连接维持，
    SDK redo 模块断线自动重注册——**无手工心跳**（3.0.x 的断线不重注册缺陷
    已在 3.1+ 修复，requirements 因此锁定 >=3.1.0）
  - SVC_HOST=0.0.0.0（compose 默认）时自动探测本机真实 IP；注册 0.0.0.0
    会让 Discover 拿到不可拨号地址
  - metadata.grpc_port 由 main.py 注入，供 BFF resolveGRPCAddr（WithPortHint）消费
"""
from __future__ import annotations

import asyncio
import logging
import os
import re
import socket
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


def _import_v3_sdk():
    """惰性 import nacos-sdk-python v3（v2.nacos 命名空间）；缺包时给清晰错误。

    单测在 _create_naming_service / _create_config_service 边界 mock，本地无需装 SDK；
    本函数的导入路径由 test_nacos_client.py 的 AST 源码契约锁定。
    """
    try:
        from v2.nacos import (
            ClientConfig,
            ConfigParam,
            DeregisterInstanceParam,
            NacosConfigService,
            NacosNamingService,
            RegisterInstanceParam,
        )
    except ImportError as e:
        raise RuntimeError(
            "nacos-sdk-python not installed. "
            "Install: pip install 'nacos-sdk-python>=3.1.0'"
        ) from e

    class _SDK:
        pass

    _SDK.ClientConfig = ClientConfig
    _SDK.ConfigParam = ConfigParam
    _SDK.DeregisterInstanceParam = DeregisterInstanceParam
    _SDK.NacosConfigService = NacosConfigService
    _SDK.NacosNamingService = NacosNamingService
    _SDK.RegisterInstanceParam = RegisterInstanceParam
    return _SDK


async def _create_naming_service(cfg: NacosConfig):
    """构建 v3 Naming 服务（异步 gRPC 连接）"""
    sdk = _import_v3_sdk()
    client_config = _client_config(sdk, cfg)
    return await sdk.NacosNamingService.create_naming_service(client_config)


async def _create_config_service(cfg: NacosConfig):
    """构建 v3 Config 服务"""
    sdk = _import_v3_sdk()
    client_config = _client_config(sdk, cfg)
    return await sdk.NacosConfigService.create_config_service(client_config)


def _client_config(sdk, cfg: NacosConfig):
    """公共 ClientConfig：非 root 容器里 SDK 的日志/磁盘缓存目录必须可写
    （默认相对 cwd → /app 只读；Stage 88 e2e 实测 Permission denied）"""
    client_config = sdk.ClientConfig(
        server_addresses=cfg.server_addr,
        namespace_id=cfg.namespace,
        username=cfg.username or None,
        password=cfg.password or None,
        log_dir=_ensure_writable_dir("NACOS_LOG_DIR", "/tmp/nacos-logs"),
    )
    client_config.set_cache_dir(_ensure_writable_dir("NACOS_CACHE_DIR", "/tmp/nacos-cache"))
    return client_config


def _ensure_writable_dir(env_key: str, default: str) -> str:
    d = os.getenv(env_key, default)
    os.makedirs(d, exist_ok=True)
    return d


def _advertise_ip(server_addr: str) -> str:
    """探测本机在 Nacos 可达网络上的真实 IP（UDP connect 不发包，仅查路由）。

    compose 注入的 SVC_HOST=0.0.0.0 不是可拨号地址；UDP connect 到 Nacos
    拿 sockname 即本机在该网络的 IP。探测失败退回 hostname 解析。
    """
    first = server_addr.split(",")[0].strip()
    host, _, port_str = first.rpartition(":")
    try:
        with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as s:
            s.connect((host or first, int(port_str) if port_str else 8848))
            return s.getsockname()[0]
    except OSError:
        pass
    return socket.gethostbyname(socket.gethostname())


class NacosRuntime:
    """emotion-llm-service 的 Nacos 运行时客户端封装（v3 gRPC）。

    使用方式（在 FastAPI lifespan 中）：
        runtime = NacosRuntime(cfg)
        await runtime.start(svc_name="emotion-llm-service", host=..., port=8000)
        ...
        await runtime.close()
    """

    def __init__(self, cfg: NacosConfig):
        self._cfg = cfg
        self._naming = None
        self._config = None
        self._stopped = False
        self._last_svc_name = ""
        self._last_ip = ""
        self._last_port = 0

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
        """启动流程：connect → register → get_config → listen_config。

        失败语义（与 Go svc 同构）：
          - connect / register 失败 → RuntimeError（lifespan 层决定降级）
          - get_config 失败（无配置）→ 不阻断（dev 首次启动正常）
          - listen_config 失败 → log warning，不阻断
        """
        self._naming = await _create_naming_service(self._cfg)
        self._config = await _create_config_service(self._cfg)
        sdk = _import_v3_sdk()

        meta = {"stage": self._cfg.namespace, "version": _git_version()}
        if metadata:
            meta.update(metadata)

        advertise_ip = host if host not in ("", "0.0.0.0", "::") else _advertise_ip(
            self._cfg.server_addr
        )
        ok = await self._naming.register_instance(
            sdk.RegisterInstanceParam(
                service_name=svc_name,
                ip=advertise_ip,
                port=port,
                metadata={k: str(v) for k, v in meta.items()},
                ephemeral=True,
            )
        )
        if not ok:
            raise RuntimeError(f"register_instance returned false for {svc_name}")
        logger.info(
            "[nacos] registered %s at %s:%d (metadata=%s)",
            svc_name, advertise_ip, port, meta,
        )
        self._last_svc_name = svc_name
        self._last_ip = advertise_ip
        self._last_port = port

        # GetConfig + ListenConfig（gRPC 连接保活 + redo 由 SDK 负责，无手工心跳）
        data_id = ops_data_id or f"{svc_name}.ops.yaml"
        try:
            content = await self._config.get_config(
                sdk.ConfigParam(data_id=data_id, group=self._cfg.group_name)
            )
            logger.info(
                "[nacos] ops config loaded: %s/%s, %d bytes",
                self._cfg.group_name, data_id, len(content or ""),
            )
        except Exception as e:
            logger.warning(
                "[nacos] GetConfig(%s/%s) failed (continuing): %s",
                self._cfg.group_name, data_id, e,
            )
            content = ""

        if on_config_change is not None:
            try:
                self._config.add_listener(
                    data_id, self._cfg.group_name, _sync_callback(on_config_change)
                )
                logger.info("[nacos] ListenConfig registered: %s/%s", self._cfg.group_name, data_id)
            except Exception as e:
                logger.warning("[nacos] ListenConfig failed (continuing): %s", e)

    async def close(self) -> None:
        """优雅退出：deregister → shutdown（naming + config）。二次 close no-op。"""
        if self._stopped:
            return
        self._stopped = True
        sdk = _import_v3_sdk()
        if self._naming is not None:
            try:
                await self._naming.deregister_instance(
                    sdk.DeregisterInstanceParam(
                        service_name=self._last_svc_name,
                        ip=self._last_ip,
                        port=self._last_port,
                        ephemeral=True,
                    )
                )
            except Exception as e:
                logger.warning("[nacos] deregister failed (continuing): %s", e)
            try:
                await self._naming.shutdown()
            except Exception as e:
                logger.warning("[nacos] naming shutdown failed (continuing): %s", e)
        if self._config is not None:
            try:
                await self._config.shutdown()
            except Exception as e:
                logger.warning("[nacos] config shutdown failed (continuing): %s", e)

    async def _heartbeat_loop(self, svc_name, host, port, metadata) -> None:
        """已废弃（Stage 88）：v3 gRPC 长连接 + redo 由 SDK 保活。保留签名防外部引用。"""
        raise NotImplementedError("heartbeat moved into SDK (v3 gRPC connection liveness)")


def _sync_callback(async_fn):
    """把 async 回调包装成 SDK listener 的同步 callback。

    SDK 在自己的线程里调 listener，这里开新 event loop 跑 await。
    兼容两种回调形状：3 参 (data_id, group, content) 与 1 参 (content)。
    """
    def wrapper(*args):
        try:
            loop = asyncio.new_event_loop()
            try:
                if len(args) >= 3:
                    loop.run_until_complete(async_fn(args[0], args[1], args[2]))
                elif len(args) == 1:
                    loop.run_until_complete(async_fn("", "", str(args[0])))
                else:
                    logger.warning("[nacos] config change callback unexpected args: %r", args)
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
